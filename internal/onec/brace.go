package onec

import (
	"regexp"
	"strings"
)

// Скобочный формат 1С — вложенные списки {…}, строки в кавычках, кавычка
// внутри строки удваивается: {1,"Текст ""в кавычках""",{2,3}}.

// stringsAtDepth возвращает границы содержимого строк (без кавычек) на глубине depth.
func stringsAtDepth(s string, depth int) [][2]int {
	var out [][2]int
	level := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '{':
			level++
		case '}':
			level--
		case '"':
			start := i + 1
			j := start
			for j < len(s) {
				if s[j] == '"' {
					if j+1 < len(s) && s[j+1] == '"' {
						j += 2
						continue
					}
					break
				}
				j++
			}
			if level == depth {
				out = append(out, [2]int{start, j})
			}
			i = j
		}
	}
	return out
}

func unquote(s string) string { return strings.ReplaceAll(s, `""`, `"`) }
func quote(s string) string   { return strings.ReplaceAll(s, `"`, `""`) }

var guidRe = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)

// objectNameRe — «{1,0,<guid>},"Имя",{1,"ru","Синоним"}» в описании объекта метаданных.
var objectNameRe = regexp.MustCompile(`\{\d+,\d+,([0-9a-f-]{36})\},"((?:[^"]|"")*)",\s*\{\d+(?:,"[^"]*","((?:[^"]|"")*)")?`)

type objectName struct{ id, name, synonym string }

// objectNames находит в описании все пары «внутренний id — имя — синоним».
func objectNames(desc string) []objectName {
	var out []objectName
	for _, m := range objectNameRe.FindAllStringSubmatch(desc, -1) {
		out = append(out, objectName{id: m[1], name: unquote(m[2]), synonym: unquote(m[3])})
	}
	return out
}

// htmlToText превращает HTML справки в читаемый текст.
var (
	htmlBreakRe = regexp.MustCompile(`(?i)<\s*(br|/p|/div|/h[1-6]|/li|/tr)\s*/?>`)
	htmlTagRe   = regexp.MustCompile(`(?s)<[^>]*>`)
	htmlHeadRe  = regexp.MustCompile(`(?is)<(head|style|script)[^>]*>.*?</(head|style|script)>`)
	blankLines  = regexp.MustCompile(`\n[ \t]*\n[ \t\n]*`)
)

func htmlToText(h string) string {
	h = htmlHeadRe.ReplaceAllString(h, "")
	h = htmlBreakRe.ReplaceAllString(h, "\n")
	h = htmlTagRe.ReplaceAllString(h, "")
	h = strings.NewReplacer("&nbsp;", " ", "&lt;", "<", "&gt;", ">", "&quot;", `"`, "&amp;", "&", "\r", "").Replace(h)
	return strings.TrimSpace(blankLines.ReplaceAllString(h, "\n\n"))
}
