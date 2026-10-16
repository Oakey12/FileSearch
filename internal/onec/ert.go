package onec

import (
	"bytes"
	"compress/zlib"
	"errors"
	"io"
	"os"
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

func openERT(path string) (*File, error) {
	streams, err := readOLE(path)
	if err != nil {
		return nil, err
	}
	file := &File{setters: map[string]func(string){}, ertPath: path}
	doc := &file.Doc
	doc.Format = "1С:Предприятие 7.7"
	doc.Kind = "Внешняя обработка или отчёт"
	doc.Name = baseName(path)

	var other []string
	for _, s := range streams {
		name := s.path[strings.LastIndex(s.path, "/")+1:]
		switch name {
		case ertModuleStream:
			text, ok := unpack77(s.data)
			sec := Section{ID: "module", Group: "Модули", Title: "Модуль", Kind: "module", Editable: true}
			if ok || isMostlyText(text) {
				sec.Text = normalizeEOL(cp1251(text))
			} else {
				sec.Note = "Модуль не удалось прочитать: возможно, он защищён паролем."
				sec.Editable = false
			}
			// Записывать умеем модуль в корне файла — там он у внешних обработок 7.7.
			if sec.Editable && s.path == ertModuleStream {
				doc.CanSave = true
				file.setters["module"] = func(text string) { file.ertModule = &text }
			} else {
				sec.Editable = false
			}
			doc.Sections = append([]Section{sec}, doc.Sections...)
		case ertDialogStream:
			doc.Sections = append(doc.Sections, Section{
				ID: "dialog", Group: "Формы", Title: "Форма (описание диалога)", Kind: "text",
				Text: normalizeEOL(cp1251(cutMFCLength(s.data))),
			})
		default:
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
		return nil, errors.New("в файле нет модуля 1С 7.7")
	}
	return file, nil
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
