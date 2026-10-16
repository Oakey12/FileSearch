package onec

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Section — часть файла, которую можно прочитать (модуль, описание, справка).
type Section struct {
	ID       string `json:"id"`
	Group    string `json:"group"` // «Модули», «Формы», «Макеты», «Справка», «Описание»
	Title    string `json:"title"`
	Kind     string `json:"kind"`           // module — код 1С, text — прочий текст
	Text     string `json:"text,omitempty"` // у больших файлов текст запрашивается отдельно
	Lines    int    `json:"lines"`
	Editable bool   `json:"editable"`
	Note     string `json:"note,omitempty"`
}

// Document — разобранная обработка или отчёт.
type Document struct {
	Path     string    `json:"path"`
	Name     string    `json:"name"`
	Synonym  string    `json:"synonym,omitempty"`
	Kind     string    `json:"kind"`   // «Внешняя обработка», «Внешний отчёт»
	Format   string    `json:"format"` // «1С:Предприятие 8», «1С:Предприятие 7.7»
	Sections []Section `json:"sections"`
	CanSave  bool      `json:"canSave"` // можно сохранить правки обратно в файл этого формата
}

// File — открытый файл: документ для показа и всё, что нужно для сохранения правок.
type File struct {
	Doc     Document
	v8      *container
	setters map[string]func(string) // id раздела → запись нового текста

	// 1С 7.7: исходный файл и новые тексты модулей (путь потока → текст).
	ole77Path  string
	ole77Edits map[string]string
}

const maxFileSize = 512 << 20

// Open читает .epf/.erf (1С 8) или .ert (1С 7.7).
func Open(path string) (*File, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Size() > maxFileSize {
		return nil, errors.New("файл слишком большой")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var file *File
	switch {
	case IsV8(data):
		file, err = openV8(path, data)
	case IsOLE(data):
		file, err = open77(path)
	default:
		return nil, errors.New("это не файл 1С (ожидается .epf, .erf, .cf, .cfe, .ert или .md)")
	}
	if err != nil {
		return nil, err
	}
	file.Doc.Path = path
	for i := range file.Doc.Sections {
		if t := file.Doc.Sections[i].Text; t != "" {
			file.Doc.Sections[i].Lines = strings.Count(t, "\n") + 1
		}
	}
	return file, nil
}

// Outline — документ без текстов разделов: для конфигураций с тысячами модулей
// интерфейс получает список, а текст модуля запрашивает при выборе (SectionText).
func (f *File) Outline() Document {
	doc := f.Doc
	doc.Sections = make([]Section, len(f.Doc.Sections))
	for i, s := range f.Doc.Sections {
		s.Text = ""
		doc.Sections[i] = s
	}
	return doc
}

// SectionText — текст раздела по id.
func (f *File) SectionText(id string) (string, bool) {
	for _, s := range f.Doc.Sections {
		if s.ID == id {
			return s.Text, true
		}
	}
	return "", false
}

// ExtractText — все модули файла одним текстом (для поиска по содержимому).
func ExtractText(path string) (string, error) {
	f, err := Open(path)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, s := range f.Doc.Sections {
		if s.Kind == "module" && s.Text != "" {
			fmt.Fprintf(&b, "// ===== %s =====\n%s\n\n", s.Title, s.Text)
		}
	}
	return b.String(), nil
}

func baseName(path string) string {
	return strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
}

func normalizeEOL(s string) string { return strings.ReplaceAll(s, "\r\n", "\n") }
func windowsEOL(s string) string   { return strings.ReplaceAll(normalizeEOL(s), "\n", "\r\n") }

const bom = "\uFEFF"

// --- 1С 8 --------------------------------------------------------------------

var (
	base64Re    = regexp.MustCompile(`#base64:([A-Za-z0-9+/=\s]+)`)
	moxcelStrRe = regexp.MustCompile(`"((?:[^"]|""){2,})"`)
)

// addTextElem — раздел из элемента-текста (модуль объекта, модуль обычной формы).
func (f *File) addTextElem(sec Section, e *v8Elem) {
	hasBOM := strings.HasPrefix(string(e.payload), bom)
	sec.Text = normalizeEOL(strings.TrimPrefix(string(e.payload), bom))
	f.Doc.Sections = append(f.Doc.Sections, sec)
	f.setters[sec.ID] = func(text string) {
		out := windowsEOL(text)
		if hasBOM {
			out = bom + out
		}
		e.payload = []byte(out)
		e.dirty = true
	}
}

// addManagedForm — управляемая форма: модуль хранится строкой верхнего уровня
// в скобочном описании формы.
func (f *File) addManagedForm(id, group, title string, e *v8Elem) {
	form := string(e.payload)
	var span [2]int
	found := false
	for _, s := range stringsAtDepth(form, 1) {
		if looksLikeCode(form[s[0]:s[1]]) {
			span, found = s, true
			break
		}
	}
	sec := Section{ID: id, Group: group, Title: title, Kind: "module"}
	if !found {
		sec.Note = "У этой формы нет модуля."
		f.Doc.Sections = append(f.Doc.Sections, sec)
		return
	}
	sec.Text = normalizeEOL(unquote(form[span[0]:span[1]]))
	sec.Editable = true
	f.Doc.Sections = append(f.Doc.Sections, sec)
	f.setters[sec.ID] = func(text string) {
		current := string(e.payload)
		e.payload = []byte(current[:span[0]] + quote(windowsEOL(text)) + current[span[1]:])
		e.dirty = true
	}
}

// looksLikeCode — строка похожа на модуль 1С (а не на заголовок или имя).
func looksLikeCode(s string) bool {
	if strings.TrimSpace(s) == "" {
		return true // пустой модуль формы
	}
	lower := strings.ToLower(s)
	for _, kw := range []string{"процедура", "функция", "&наклиенте", "&насервере", "#область", "перем ", "//", "procedure", "function"} {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// moxcelText — строки из табличного документа (текст ячеек в кавычках).
func moxcelText(b []byte) string {
	s := string(b)
	if i := strings.Index(s, "{"); i >= 0 {
		s = s[i:]
	}
	var lines []string
	seen := map[string]bool{}
	for _, m := range moxcelStrRe.FindAllStringSubmatch(s, -1) {
		t := strings.TrimSpace(unquote(m[1]))
		if t == "" || t == "ru" || t == "en" || seen[t] || strings.HasPrefix(t, "#") {
			continue
		}
		seen[t] = true
		lines = append(lines, t)
	}
	return strings.Join(lines, "\n")
}

// --- Сохранение -------------------------------------------------------------------

// SaveTo применяет правки и записывает файл в исходном формате (.epf/.erf или .ert).
// Запись атомарная: сначала во временный файл рядом, затем переименование, —
// так прерванное сохранение не испортит существующий файл. dst может совпадать
// с исходным файлом.
func (f *File) SaveTo(dst string, edits map[string]string) error {
	tmp := dst + ".tmp-filesearch"
	defer os.Remove(tmp)

	if f.v8 != nil {
		data, err := f.Save(edits)
		if err != nil {
			return err
		}
		if err := os.WriteFile(tmp, data, 0o644); err != nil {
			return err
		}
	} else {
		if err := f.applyEdits(edits); err != nil {
			return err
		}
		if len(f.ole77Edits) == 0 {
			// Модули не меняли — копия исходного файла.
			data, err := os.ReadFile(f.ole77Path)
			if err != nil {
				return err
			}
			if err := os.WriteFile(tmp, data, 0o644); err != nil {
				return err
			}
		} else {
			streams := map[string][]byte{}
			for path, text := range f.ole77Edits {
				packed, err := deflate(encode1251(windowsEOL(text)))
				if err != nil {
					return err
				}
				streams[path] = packed
			}
			if err := writeOLE(f.ole77Path, tmp, streams); err != nil {
				return err
			}
		}
	}
	return os.Rename(tmp, dst)
}

func (f *File) applyEdits(edits map[string]string) error {
	for id, text := range edits {
		set, ok := f.setters[id]
		if !ok {
			return fmt.Errorf("раздел %q нельзя изменить", id)
		}
		set(text)
	}
	return nil
}

// Save применяет правки к контейнеру 1С 8 и возвращает новый файл.
// Неизменённые элементы переносятся побайтно.
func (f *File) Save(edits map[string]string) ([]byte, error) {
	if f.v8 == nil {
		return nil, errors.New("это не контейнер 1С 8")
	}
	for id, text := range edits {
		set, ok := f.setters[id]
		if !ok {
			return nil, fmt.Errorf("раздел %q нельзя изменить", id)
		}
		set(text)
	}
	return f.v8.bytes()
}
