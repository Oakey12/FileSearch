package onec

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
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
	Text     string `json:"text,omitempty"` // у больших файлов пусто: текст — через Content
	Lines    int    `json:"lines"`          // 0 — ещё не известно
	Editable bool   `json:"editable"`
	Note     string `json:"note,omitempty"`
}

// SectionContent — текст раздела, прочитанный по запросу.
type SectionContent struct {
	Text     string `json:"text"`
	Note     string `json:"note,omitempty"`
	Lines    int    `json:"lines"`
	Editable bool   `json:"editable"`
}

// Document — разобранный файл 1С.
type Document struct {
	Path     string    `json:"path"`
	Name     string    `json:"name"`
	Synonym  string    `json:"synonym,omitempty"`
	Kind     string    `json:"kind"`   // «Внешняя обработка», «Конфигурация» …
	Format   string    `json:"format"` // «1С:Предприятие 8», «1С:Предприятие 7.7»
	Sections []Section `json:"sections"`
	CanSave  bool      `json:"canSave"` // можно сохранить правки обратно в файл этого формата
	Lazy     bool      `json:"lazy,omitempty"`
}

// part — как прочитать и как изменить раздел.
type part struct {
	load func() (SectionContent, error)
	set  func(text string) error
}

// File — открытый файл: документ для показа и всё, что нужно для чтения
// разделов и сохранения правок. Безопасен только для одной горутины.
type File struct {
	Doc   Document
	ctx   context.Context // отмена разбора (только на время Open)
	src   *fileSource
	v8    *container
	parts []part // параллельно Doc.Sections

	// 1С 7.7: новые тексты модулей (путь потока → текст).
	ole77Edits map[string]string
}

// eagerLimit — файлы до этого размера читаются сразу целиком: у разделов
// известны тексты и число строк. Больше — список сразу, тексты по запросу.
var eagerLimit int64 = 32 << 20

// Open читает файл 1С: .epf, .erf, .cf, .cfe (1С 8) или .ert, .md (1С 7.7).
// Файл не читается в память целиком, поэтому подходит и для конфигураций
// в сотни мегабайт.
func Open(path string) (*File, error) {
	return OpenContext(context.Background(), path)
}

// OpenContext — Open с возможностью прервать разбор большого файла
// (например, когда пользователь уже выбрал в списке другой файл).
func OpenContext(ctx context.Context, path string) (*File, error) {
	src, err := newFileSource(path)
	if err != nil {
		return nil, err
	}
	defer src.release()
	head := make([]byte, min(int64(sniffSize), src.size))
	if _, err := src.ReadAt(head, 0); err != nil && err != io.EOF {
		return nil, err
	}
	file := &File{src: src, ctx: ctx}
	defer func() { file.ctx = nil }()
	switch {
	case IsV8(head):
		root, err := openContainer(src, src.size, 0)
		if err != nil {
			return nil, err
		}
		err = openV8(path, file, root)
		if err != nil {
			return nil, err
		}
	case IsOLE(head):
		if err := open77(path, file); err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("это не файл 1С (ожидается .epf, .erf, .cf, .cfe, .ert или .md)")
	}
	file.Doc.Path = path
	if src.size > eagerLimit {
		file.Doc.Lazy = true
		return file, nil
	}
	for i := range file.Doc.Sections {
		c, err := file.parts[i].content()
		if err != nil {
			return nil, err
		}
		s := &file.Doc.Sections[i]
		s.Text, s.Lines = c.Text, c.Lines
		if c.Note != "" {
			s.Note, s.Editable = c.Note, c.Editable
		}
	}
	return file, nil
}

func (p part) content() (SectionContent, error) {
	if p.load == nil {
		return SectionContent{}, nil
	}
	c, err := p.load()
	if err != nil {
		return c, err
	}
	if c.Text != "" {
		c.Lines = strings.Count(c.Text, "\n") + 1
	}
	return c, nil
}

func (f *File) addPart(sec Section, p part) {
	f.Doc.Sections = append(f.Doc.Sections, sec)
	f.parts = append(f.parts, p)
}

func (f *File) indexOf(id string) int {
	for i, s := range f.Doc.Sections {
		if s.ID == id {
			return i
		}
	}
	return -1
}

// Outline — документ без текстов разделов: для конфигураций с тысячами модулей
// интерфейс получает список, а текст модуля запрашивает при выборе (Content).
func (f *File) Outline() Document {
	doc := f.Doc
	doc.Sections = make([]Section, len(f.Doc.Sections))
	for i, s := range f.Doc.Sections {
		s.Text = ""
		doc.Sections[i] = s
	}
	return doc
}

// Content — текст раздела. У больших файлов он читается с диска при каждом
// запросе и в памяти не остаётся (кроме изменённых, ещё не сохранённых частей).
func (f *File) Content(id string) (SectionContent, error) {
	i := f.indexOf(id)
	if i < 0 {
		return SectionContent{}, errors.New("раздел не найден")
	}
	s := f.Doc.Sections[i]
	if !f.Doc.Lazy {
		return SectionContent{Text: s.Text, Note: s.Note, Lines: s.Lines, Editable: s.Editable}, nil
	}
	defer f.release()
	c, err := f.parts[i].content()
	if err != nil {
		return c, err
	}
	if c.Note == "" && c.Text == "" {
		c.Note, c.Editable = s.Note, s.Editable && f.parts[i].set != nil
	}
	return c, nil
}

// SectionText — текст раздела по id (false — раздела нет или он не читается).
func (f *File) SectionText(id string) (string, bool) {
	c, err := f.Content(id)
	return c.Text, err == nil
}

// release закрывает исходный файл до следующего обращения.
func (f *File) release() {
	if f.src != nil {
		f.src.release()
	}
}

// ExtractText — модули файла одним текстом (для поиска по содержимому),
// не больше limit байт: у большой конфигурации читаются только первые модули.
func ExtractText(path string, limit int) (string, error) {
	f, err := Open(path)
	if err != nil {
		return "", err
	}
	defer f.release()
	var b strings.Builder
	for i, s := range f.Doc.Sections {
		if s.Kind != "module" {
			continue
		}
		text := s.Text
		if f.Doc.Lazy {
			c, err := f.parts[i].content()
			if err != nil {
				continue // испорченный модуль не мешает искать в остальных
			}
			text = c.Text
		}
		if text == "" {
			continue
		}
		fmt.Fprintf(&b, "// ===== %s =====\n%s\n\n", s.Title, text)
		if b.Len() >= limit {
			break
		}
	}
	out := b.String()
	if len(out) > limit {
		out = strings.ToValidUTF8(out[:limit], "")
	}
	return out, nil
}

func baseName(path string) string {
	return strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
}

func normalizeEOL(s string) string { return strings.ReplaceAll(s, "\r\n", "\n") }
func windowsEOL(s string) string   { return strings.ReplaceAll(normalizeEOL(s), "\n", "\r\n") }

const bom = "\uFEFF"

var (
	base64Re    = regexp.MustCompile(`#base64:([A-Za-z0-9+/=\s]+)`)
	moxcelStrRe = regexp.MustCompile(`"((?:[^"]|""){2,})"`)
)

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

// SaveTo применяет правки и записывает файл в исходном формате.
// Запись атомарная: сначала во временный файл рядом, затем переименование, —
// так прерванное сохранение не испортит существующий файл. dst может совпадать
// с исходным файлом. Нетронутые части копируются из исходного файла потоком.
func (f *File) SaveTo(dst string, edits map[string]string) error {
	tmp := dst + ".tmp-filesearch"
	defer os.Remove(tmp)
	if err := f.applyEdits(edits); err != nil {
		return err
	}
	err := func() error {
		defer f.release() // исходный файл должен быть закрыт до переименования
		if f.v8 != nil {
			return writeFile(tmp, f.v8.writeTo)
		}
		if len(f.ole77Edits) == 0 {
			return writeFile(tmp, func(w io.Writer) error {
				_, err := io.Copy(w, io.NewSectionReader(f.src, 0, f.src.size))
				return err
			})
		}
		streams := map[string][]byte{}
		for path, text := range f.ole77Edits {
			packed, err := deflate(encode1251(windowsEOL(text)))
			if err != nil {
				return err
			}
			streams[path] = packed
		}
		f.release()
		return writeOLE(f.src.path, tmp, streams)
	}()
	if err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}

// writeFile записывает файл через буфер и проверяет закрытие.
func writeFile(path string, write func(io.Writer) error) error {
	out, err := os.Create(path)
	if err != nil {
		return err
	}
	w := bufio.NewWriterSize(out, 1<<20)
	if err := write(w); err != nil {
		out.Close()
		return err
	}
	if err := w.Flush(); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func (f *File) applyEdits(edits map[string]string) error {
	for id, text := range edits {
		i := f.indexOf(id)
		if i < 0 || f.parts[i].set == nil {
			return fmt.Errorf("раздел %q нельзя изменить", id)
		}
		if err := f.parts[i].set(text); err != nil {
			return err
		}
	}
	return nil
}

// Save применяет правки к контейнеру 1С 8 и возвращает новый файл целиком
// (для небольших файлов и тестов; большие сохраняет SaveTo).
func (f *File) Save(edits map[string]string) ([]byte, error) {
	if f.v8 == nil {
		return nil, errors.New("это не контейнер 1С 8")
	}
	defer f.release()
	if err := f.applyEdits(edits); err != nil {
		return nil, err
	}
	return f.v8.bytes()
}
