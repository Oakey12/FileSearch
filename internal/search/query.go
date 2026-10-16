package search

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"file-search/internal/content"
)

// Query — разобранный запрос: все слова должны встретиться, в любом порядке.
// Текст в кавычках ищется как одна фраза.
type Query struct {
	Terms []string // уже нормализованы (content.Normalize)
}

// ParseQuery разбирает строку запроса: `отчёт "за июль"` → [отчет, за июль].
func ParseQuery(raw string) Query {
	var terms []string
	for i, part := range strings.Split(raw, `"`) {
		if i%2 == 1 { // внутри кавычек
			if phrase := strings.TrimSpace(part); phrase != "" {
				terms = append(terms, content.Normalize(phrase))
			}
			continue
		}
		for _, word := range strings.Fields(part) {
			terms = append(terms, content.Normalize(word))
		}
	}
	return Query{Terms: terms}
}

// Empty сообщает, что искать нечего.
func (q Query) Empty() bool { return len(q.Terms) == 0 }

// matchName проверяет, что все слова есть в имени файла.
func (q Query) matchName(name string) bool {
	norm := content.Normalize(name)
	for _, term := range q.Terms {
		if !strings.Contains(norm, term) {
			return false
		}
	}
	return true
}

// matchDoc проверяет, что каждое слово есть в имени или в тексте документа.
// Возвращает фрагмент текста вокруг первого найденного слова и число его вхождений.
func (q Query) matchDoc(name string, doc *content.Doc) (ok bool, snippet string, hits int) {
	normName := content.Normalize(name)
	first := -1
	for _, term := range q.Terms {
		at := strings.Index(doc.Norm, term)
		if at < 0 && !strings.Contains(normName, term) {
			return false, "", 0
		}
		if at >= 0 && first < 0 {
			first = at
			hits = min(strings.Count(doc.Norm, term), 999)
		}
	}
	if first >= 0 {
		snippet = makeSnippet(doc, first)
	}
	return true, snippet, hits
}

const (
	snippetBefore = 40 // символов до найденного слова
	snippetAfter  = 110
)

// makeSnippet вырезает фрагмент вокруг позиции at (в байтах Norm) и
// схлопывает переводы строк и повторяющиеся пробелы.
func makeSnippet(doc *content.Doc, at int) string {
	text := doc.Text
	if len(text) != len(doc.Norm) { // редкие символы меняют длину при ToLower
		text = doc.Norm
	}

	start := at
	for n := 0; n < snippetBefore && start > 0; n++ {
		_, size := utf8.DecodeLastRuneInString(text[:start])
		start -= size
	}
	end := at
	for n := 0; n < snippetAfter && end < len(text); n++ {
		_, size := utf8.DecodeRuneInString(text[end:])
		end += size
	}

	var b strings.Builder
	if start > 0 {
		b.WriteString("…")
	}
	space := false
	for _, r := range text[start:end] {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			space = true
			continue
		}
		if space && b.Len() > 0 {
			b.WriteByte(' ')
		}
		space = false
		b.WriteRune(r)
	}
	if end < len(text) {
		b.WriteString("…")
	}
	return b.String()
}
