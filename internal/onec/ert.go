package onec

import (
	"bytes"
	"compress/zlib"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/richardlehane/mscfb"
	"golang.org/x/text/encoding/charmap"
)

// Формат .ert 1С:Предприятия 7.7 (по исходникам GComp): составной OLE-документ.
//   «MD Programm text» — текст модуля, сжат deflate (движок V77.zlibEngine);
//   «Dialog Stream»    — описание формы, в начале длина в формате MFC CString;
//   «Container.Contents», «Main MetaData Stream» и др. — служебные потоки.
// Тексты — в кодировке Windows-1251.

const (
	ertModuleStream = "MD Programm text"
	ertDialogStream = "Dialog Stream"
)

var oleSignature = []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}

// IsOLE сообщает, что данные — составной документ OLE (как .ert).
func IsOLE(b []byte) bool { return bytes.HasPrefix(b, oleSignature) }

type ertStream struct {
	path string
	data []byte
}

func readOLE(path string) ([]ertStream, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	doc, err := mscfb.New(f)
	if err != nil {
		return nil, err
	}
	var out []ertStream
	total := 0
	for entry, err := doc.Next(); err == nil; entry, err = doc.Next() {
		if entry.FileInfo().IsDir() {
			continue
		}
		if total += int(entry.Size); total > maxInflated {
			return nil, errors.New("слишком большой файл")
		}
		data, err := io.ReadAll(io.LimitReader(entry, maxInflated))
		if err != nil {
			return nil, err
		}
		out = append(out, ertStream{path: strings.Join(append(append([]string{}, entry.Path...), entry.Name), "/"), data: data})
	}
	return out, nil
}

// unpack77 распаковывает поток 7.7: deflate без заголовка, затем zlib, иначе как есть.
func unpack77(b []byte) ([]byte, bool) {
	if out, err := inflateLimited(b); err == nil && len(out) > 0 {
		return out, true
	}
	if r, err := zlib.NewReader(bytes.NewReader(b)); err == nil {
		if out, err := io.ReadAll(io.LimitReader(r, maxInflated)); err == nil && len(out) > 0 {
			return out, true
		}
	}
	return b, false
}

// cutMFCLength убирает длину в начале потока (формат MFC CString:
// 1 байт; 0xFF + 2 байта; 0xFF 0xFFFF + 4 байта).
func cutMFCLength(b []byte) []byte {
	switch {
	case len(b) >= 1 && b[0] < 0xFF:
		return b[1:]
	case len(b) >= 3 && !(b[1] == 0xFF && b[2] == 0xFF):
		return b[3:]
	case len(b) >= 7:
		return b[7:]
	}
	return b
}

// encode1251 переводит текст в Windows-1251; символы, которых в ней нет, — «?».
func encode1251(s string) []byte {
	out := make([]byte, 0, len(s))
	for _, r := range s {
		if b, ok := charmap.Windows1251.EncodeRune(r); ok {
			out = append(out, b)
		} else {
			out = append(out, '?')
		}
	}
	return out
}

func cp1251(b []byte) string {
	s, err := charmap.Windows1251.NewDecoder().Bytes(b)
	if err != nil {
		return string(b)
	}
	return string(s)
}

// Хранилища конфигурации 7.7 (по GComp) и группы в дереве.
var storage77Labels = map[string]string{
	"Document":    "Документы",
	"Subconto":    "Справочники",
	"Journal":     "Журналы документов",
	"CalcJournal": "Журналы расчётов",
	"Report":      "Отчёты",
	"CalcVar":     "Виды расчётов",
	"Register":    "Регистры",
	"UserDef":     "Пользовательские",
	"GlobalData":  "Конфигурация",
}

var (
	number77Re   = regexp.MustCompile(`_Number(\d+)`)
	metaName77Re = regexp.MustCompile(`\{"(\d+)","((?:[^"]|"")*)"`)
)

// open77 читает .ert (внешняя обработка 7.7) и .md (конфигурация 7.7):
// все модули во всех хранилищах, имена объектов — из главного потока метаданных.
func open77(path string) (*File, error) {
	streams, err := readOLE(path)
	if err != nil {
		return nil, err
	}
	isMD := strings.EqualFold(filepath.Ext(path), ".md")
	file := &File{setters: map[string]func(string){}, ole77Path: path, ole77Edits: map[string]string{}}
	doc := &file.Doc
	doc.Format = "1С:Предприятие 7.7"
	doc.Kind = "Внешняя обработка или отчёт"
	if isMD {
		doc.Kind = "Конфигурация"
	}
	doc.Name = baseName(path)

	// Номер объекта → имя и номер объекта → вид (по хранилищу, где он лежит).
	names := map[string]string{}
	kinds := map[string]string{}
	for _, s := range streams {
		if strings.HasSuffix(s.path, "Main MetaData Stream") {
			raw := cutMFCLength(s.data)
			if !isMostlyText(raw) {
				raw, _ = unpack77(s.data)
			}
			for _, m := range metaName77Re.FindAllStringSubmatch(cp1251(raw), -1) {
				if _, ok := names[m[1]]; !ok {
					names[m[1]] = unquote(m[2])
				}
			}
		}
		parts := strings.Split(s.path, "/")
		if label, ok := storage77Labels[parts[0]]; ok {
			if m := number77Re.FindStringSubmatch(s.path); m != nil {
				kinds[m[1]] = label
			}
		}
	}

	var other []string
	for _, s := range streams {
		leaf := s.path[strings.LastIndex(s.path, "/")+1:]
		switch {
		case leaf == ertModuleStream:
			group, title := describe77(s.path, names, kinds)
			sec := Section{ID: "ole:" + s.path, Group: group, Title: title, Kind: "module", Editable: true}
			if s.path == ertModuleStream {
				sec.ID = "module" // модуль внешней обработки
			}
			text, ok := unpack77(s.data)
			if ok || isMostlyText(text) {
				sec.Text = normalizeEOL(cp1251(text))
				streamPath := s.path
				file.setters[sec.ID] = func(text string) { file.ole77Edits[streamPath] = text }
			} else {
				sec.Note = "Модуль не удалось прочитать: возможно, он защищён паролем."
				sec.Editable = false
			}
			doc.Sections = append(doc.Sections, sec)
		case leaf == ertDialogStream && !isMD:
			doc.Sections = append(doc.Sections, Section{
				ID: "dialog", Group: "Формы", Title: "Форма (описание диалога)", Kind: "text",
				Text: normalizeEOL(cp1251(cutMFCLength(s.data))),
			})
		case !isMD:
			other = append(other, s.path)
		}
	}
	if len(other) > 0 {
		sort.Strings(other)
		doc.Sections = append(doc.Sections, Section{
			ID: "streams", Group: "Описание", Title: "Состав файла", Kind: "text",
			Text: "Потоки внутри файла:\n" + strings.Join(other, "\n"),
		})
	}
	if len(doc.Sections) == 0 {
		return nil, errors.New("в файле нет модулей 1С 7.7")
	}
	sort7(doc.Sections, isMD)
	doc.CanSave = len(file.setters) > 0
	return file, nil
}

// describe77 — группа и название модуля по пути потока внутри файла 7.7.
func describe77(path string, names, kinds map[string]string) (group, title string) {
	if path == ertModuleStream {
		return "Модули", "Модуль"
	}
	parts := strings.Split(path, "/")
	id := ""
	if m := number77Re.FindStringSubmatch(path); m != nil {
		id = m[1]
	}
	name := names[id]
	if name == "" && id != "" {
		name = "Объект №" + id
	}
	kind := "Модуль"
	switch {
	case strings.Contains(path, "Transact_Number"):
		kind = "Модуль проведения"
	case strings.Contains(path, "UserHelp_Number"):
		kind = "Описание"
	case parts[0] != "TypedText" && parts[0] != "GlobalData" && len(parts) > 2:
		kind = "Модуль формы"
	}
	group = storage77Labels[parts[0]]
	if group == "" {
		group = kinds[id]
	}
	switch {
	case parts[0] == "GlobalData" || (id == "" && len(parts) <= 2):
		return "Конфигурация", "Глобальный модуль"
	case group == "":
		group = "Прочие модули"
	}
	if name == "" {
		return group, strings.Join(parts[:len(parts)-1], " › ")
	}
	return group, name + ": " + kind
}

var groupOrder77 = []string{"Модули", "Формы", "Конфигурация", "Справочники", "Документы", "Журналы документов",
	"Отчёты", "Журналы расчётов", "Виды расчётов", "Регистры", "Пользовательские", "Прочие модули", "Описание"}

func sort7(secs []Section, isMD bool) {
	rank := func(g string) int {
		for i, x := range groupOrder77 {
			if x == g {
				return i
			}
		}
		return len(groupOrder77)
	}
	sort.SliceStable(secs, func(i, j int) bool {
		if ri, rj := rank(secs[i].Group), rank(secs[j].Group); ri != rj {
			return ri < rj
		}
		return isMD && secs[i].Title < secs[j].Title
	})
}

// isMostlyText — грубая проверка, что байты похожи на текст, а не на мусор.
func isMostlyText(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	bad := 0
	for _, c := range b[:min(len(b), 4096)] {
		if c < 0x09 || (c > 0x0D && c < 0x20) {
			bad++
		}
	}
	return bad*20 < min(len(b), 4096)
}
