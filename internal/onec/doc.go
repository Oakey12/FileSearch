package onec

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Section — часть файла, которую можно прочитать (модуль, описание, справка).
type Section struct {
	ID       string `json:"id"`
	Group    string `json:"group"` // «Модули», «Формы», «Макеты», «Справка», «Описание»
	Title    string `json:"title"`
	Kind     string `json:"kind"` // module — код 1С, text — прочий текст
	Text     string `json:"text"`
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
		file, err = openERT(path)
	default:
		return nil, errors.New("это не обработка или отчёт 1С (ожидается .epf, .erf или .ert)")
	}
	if err != nil {
		return nil, err
	}
	file.Doc.Path = path
	return file, nil
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

func openV8(path string, data []byte) (*File, error) {
	root, err := parseContainer(data, 0)
	if err != nil {
		return nil, err
	}
	file := &File{v8: root, setters: map[string]func(string){}}
	doc := &file.Doc
	doc.Format = "1С:Предприятие 8"
	doc.Kind = "Внешняя обработка"
	if strings.EqualFold(filepath.Ext(path), ".erf") {
		doc.Kind = "Внешний отчёт"
	}
	doc.Name = baseName(path)
	doc.CanSave = true

	// root → «{2,<guid главного объекта>,}».
	mainGUID := ""
	if e := root.find("root"); e != nil {
		mainGUID = guidRe.FindString(string(e.payload))
	}
	objectID := ""
	if e := root.find(mainGUID); e != nil {
		if names := objectNames(string(e.payload)); len(names) > 0 {
			objectID, doc.Name, doc.Synonym = names[0].id, names[0].name, names[0].synonym
		}
	}

	// Модуль объекта — «<id>.0», вложенный контейнер с элементом text.
	for _, id := range []string{objectID, mainGUID} {
		if id == "" {
			continue
		}
		if e := root.find(id + ".0"); e != nil && e.child != nil {
			if text := e.child.find("text"); text != nil {
				file.addTextElem(Section{ID: "object", Group: "Модули", Title: "Модуль объекта", Kind: "module", Editable: true}, text)
				break
			}
		}
	}

	// Формы и макеты: элементы с именем-GUID, у которых есть «<guid>.0».
	type child struct {
		guid, name, synonym string
	}
	var children []child
	for _, e := range root.elems {
		if !guidRe.MatchString(e.Name) || len(e.Name) != 36 || e.Name == mainGUID {
			continue
		}
		name := e.Name
		synonym := ""
		for _, n := range objectNames(string(e.payload)) {
			if n.id == e.Name {
				name, synonym = n.name, n.synonym
				break
			}
		}
		children = append(children, child{e.Name, name, synonym})
	}
	sort.SliceStable(children, func(i, j int) bool { return children[i].name < children[j].name })

	for _, c := range children {
		e := root.find(c.guid + ".0")
		if e == nil {
			continue
		}
		title := c.name
		switch {
		case e.child != nil && e.child.find("module") != nil:
			// Обычная форма: контейнер с элементами form и module.
			file.addTextElem(Section{ID: "form:" + c.guid, Group: "Формы", Title: title, Kind: "module", Editable: true}, e.child.find("module"))
		case strings.HasPrefix(string(e.payload), "MOXCEL"):
			doc.Sections = append(doc.Sections, Section{ID: "tpl:" + c.guid, Group: "Макеты", Title: title, Kind: "text",
				Note: "Табличный документ. Его оформление открывается только в 1С; ниже — текст ячеек.", Text: moxcelText(e.payload)})
		case strings.HasPrefix(strings.TrimPrefix(string(e.payload), bom), "{"):
			file.addManagedForm(c.guid, title, e)
		}
	}

	// Справка — HTML в base64 внутри «<id>.1».
	for _, id := range []string{objectID, mainGUID} {
		if e := root.find(id + ".1"); id != "" && e != nil {
			if m := base64Re.FindStringSubmatch(string(e.payload)); m != nil {
				if raw, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(m[1]), "")); err == nil {
					if text := htmlToText(strings.TrimPrefix(string(raw), bom)); text != "" {
						doc.Sections = append(doc.Sections, Section{ID: "help", Group: "Справка", Title: "Справка", Kind: "text", Text: text})
					}
				}
			}
			break
		}
	}

	if len(doc.Sections) == 0 {
		return nil, errors.New("в файле не найдено ни модулей, ни форм")
	}
	return file, nil
}

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
// в скобочном описании формы (обычно третий элемент списка).
func (f *File) addManagedForm(guid, title string, e *v8Elem) {
	form := string(e.payload)
	var span [2]int
	found := false
	for _, s := range stringsAtDepth(form, 1) {
		text := form[s[0]:s[1]]
		if looksLikeCode(text) {
			span, found = s, true
			break
		}
	}
	sec := Section{ID: "form:" + guid, Group: "Формы", Title: title, Kind: "module"}
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

// Save применяет правки (id раздела → новый текст) и возвращает новый файл.
// Неизменённые элементы переносятся побайтно.
func (f *File) Save(edits map[string]string) ([]byte, error) {
	if f.v8 == nil {
		return nil, errors.New("сохранение в этот формат не поддерживается — сохраните модуль в текстовый файл")
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
