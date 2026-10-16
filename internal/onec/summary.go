package onec

import "strings"

// Summary — то, что показывает панель предпросмотра: что это за файл, из чего
// он состоит и начало главного модуля. Строится из уже разобранного файла и
// читает с диска только один модуль.
type Summary struct {
	Kind     string         `json:"kind"`
	Format   string         `json:"format"`
	Name     string         `json:"name"`
	Synonym  string         `json:"synonym,omitempty"`
	Sections int            `json:"sections"`
	Modules  int            `json:"modules"`
	Groups   []GroupCount   `json:"groups"`
	Main     *SummaryModule `json:"main,omitempty"`
}

// GroupCount — группа в дереве и сколько в ней разделов.
type GroupCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// SummaryModule — начало главного модуля.
type SummaryModule struct {
	Title     string `json:"title"`
	Text      string `json:"text"`
	Lines     int    `json:"lines"` // строк во всём модуле
	Truncated bool   `json:"truncated,omitempty"`
}

// mainModuleTitles — какой модуль показать первым, если он есть.
var mainModuleTitles = []string{"Модуль управляемого приложения", "Модуль объекта", "Глобальный модуль", "Модуль"}

// maxSummaryTries — сколько модулей попробовать прочитать в поисках непустого.
const maxSummaryTries = 30

// Summary собирает сводку; в главном модуле — не больше maxLines строк.
func (f *File) Summary(maxLines int) Summary {
	s := Summary{Kind: f.Doc.Kind, Format: f.Doc.Format, Name: f.Doc.Name, Synonym: f.Doc.Synonym, Sections: len(f.Doc.Sections)}
	var order []int // кандидаты в главный модуль: сначала известные, потом по порядку
	for _, title := range mainModuleTitles {
		for i, sec := range f.Doc.Sections {
			if sec.Kind == "module" && sec.Title == title {
				order = append(order, i)
			}
		}
	}
	for i, sec := range f.Doc.Sections {
		if sec.Kind == "module" {
			s.Modules++
			order = append(order, i)
		}
		if n := len(s.Groups); n > 0 && s.Groups[n-1].Name == sec.Group {
			s.Groups[n-1].Count++
		} else {
			s.Groups = append(s.Groups, GroupCount{Name: sec.Group, Count: 1})
		}
	}

	defer f.release()
	tried := map[int]bool{}
	for _, i := range order {
		if tried[i] || len(tried) >= maxSummaryTries {
			continue
		}
		tried[i] = true
		c, err := f.Content(f.Doc.Sections[i].ID)
		if err != nil || strings.TrimSpace(c.Text) == "" {
			continue
		}
		m := &SummaryModule{Title: f.Doc.Sections[i].Title, Text: c.Text, Lines: c.Lines}
		if c.Lines > maxLines {
			cut := 0
			for n := 0; n < maxLines; n++ {
				cut += strings.IndexByte(m.Text[cut:], '\n') + 1
			}
			m.Text, m.Truncated = strings.Clone(m.Text[:cut]), true // копия: не держать весь модуль
		}
		s.Main = m
		break
	}
	return s
}
