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

// Регулярные выражения на описаниях большой конфигурации (их десятки тысяч)
// съедали треть времени разбора, поэтому GUID и имена ищутся вручную.

func isHexLower(c byte) bool { return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' }

// isGUIDAt — с позиции i начинается GUID вида 8-4-4-4-12 (строчные hex).
func isGUIDAt(s string, i int) bool {
	if i < 0 || i+36 > len(s) {
		return false
	}
	for k := 0; k < 36; k++ {
		c := s[i+k]
		if k == 8 || k == 13 || k == 18 || k == 23 {
			if c != '-' {
				return false
			}
		} else if !isHexLower(c) {
			return false
		}
	}
	return true
}

// eachGUID вызывает fn для каждого GUID в s слева направо (как guidRe.FindAllString).
func eachGUID(s string, fn func(g string)) {
	for i := 0; i+36 <= len(s); {
		d := strings.IndexByte(s[i+8:], '-')
		if d < 0 {
			return
		}
		start := i + d // кандидат: дефис на 9-й позиции
		if isGUIDAt(s, start) {
			fn(s[start : start+36])
			i = start + 36
			continue
		}
		i = start + 1
	}
}

// firstGUID — первый GUID в s или "".
func firstGUID(s string) string {
	found := ""
	eachGUID(s, func(g string) {
		if found == "" {
			found = g
		}
	})
	return found
}

// isIDChars — 36 символов из [0-9a-f-] (как в шаблоне имени объекта).
func isIDChars(s string) bool {
	for i := 0; i < len(s); i++ {
		if c := s[i]; c != '-' && !isHexLower(c) {
			return false
		}
	}
	return true
}

type objectName struct{ id, name, synonym string }

// quoted читает строку в кавычках, начиная с открывающей кавычки s[i].
// Возвращает содержимое (с удвоенными кавычками) и позицию после закрывающей.
func quoted(s string, i int) (string, int, bool) {
	if i >= len(s) || s[i] != '"' {
		return "", 0, false
	}
	lastPair := -1
	for j := i + 1; j < len(s); j++ {
		if s[j] == '"' {
			if j+1 < len(s) && s[j+1] == '"' {
				lastPair = j
				j++
				continue
			}
			return s[i+1 : j], j + 1, true
		}
	}
	// Строка не закрыта: закрывающей считается первая кавычка последней пары
	// «""» — так же откатывается регулярное выражение.
	if lastPair >= 0 {
		return s[i+1 : lastPair], lastPair + 1, true
	}
	return "", 0, false
}

func skipDigits(s string, i int) int {
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	return i
}

// objectNames находит в описании все тройки «внутренний id — имя — синоним»:
// {N,N,<id>},"Имя",{N,"ru","Синоним"} (синонима может не быть).
func objectNames(desc string) []objectName {
	var out []objectName
	floor := 0 // конец прошлого совпадения: раньше новое начинаться не может
	for pos := 0; ; {
		k := strings.Index(desc[pos:], `},"`)
		if k < 0 {
			return out
		}
		j := pos + k // позиция «}»
		pos = j + 1
		idStart := j - 36
		if idStart < 0 || !isIDChars(desc[idStart:j]) {
			continue
		}
		// Перед id: «{цифры,цифры,»
		b := idStart - 1
		if b < 0 || desc[b] != ',' {
			continue
		}
		d2 := b
		for d2 > 0 && desc[d2-1] >= '0' && desc[d2-1] <= '9' {
			d2--
		}
		if d2 == b || d2 < 2 || desc[d2-1] != ',' {
			continue
		}
		d1 := d2 - 1
		for d1 > 0 && desc[d1-1] >= '0' && desc[d1-1] <= '9' {
			d1--
		}
		if d1 == d2-1 || d1 < 1 || desc[d1-1] != '{' || d1-1 < floor {
			continue
		}
		name, i, ok := quoted(desc, j+2)
		if !ok || i >= len(desc) || desc[i] != ',' {
			continue
		}
		i++
		for i < len(desc) && (desc[i] == ' ' || desc[i] == '\t' || desc[i] == '\r' || desc[i] == '\n' || desc[i] == '\f') { // как \s в RE2: без \v
			i++
		}
		if i >= len(desc) || desc[i] != '{' {
			continue
		}
		e := skipDigits(desc, i+1)
		if e == i+1 {
			continue
		}
		synonym, end := "", e
		// Необязательное «,"язык","Синоним"»; язык — без кавычек внутри.
		if e+1 < len(desc) && desc[e] == ',' && desc[e+1] == '"' {
			if q := strings.IndexByte(desc[e+2:], '"'); q >= 0 {
				after := e + 2 + q + 1
				if after+1 < len(desc) && desc[after] == ',' {
					if syn, next, ok := quoted(desc, after+1); ok {
						synonym, end = syn, next
					}
				}
			}
		}
		// Копии строк: подстроки держали бы в памяти всё описание.
		out = append(out, objectName{id: strings.Clone(desc[idStart:j]), name: strings.Clone(unquote(name)), synonym: strings.Clone(unquote(synonym))})
		pos, floor = end, end // дальше — с конца совпадения, как у регулярного выражения
	}
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
