package content

import (
	"strconv"
	"strings"
	"sync"
)

// Doc — извлечённый текст документа и его нормализованная копия для поиска.
type Doc struct {
	Text string // как в документе — для фрагментов в результатах
	Norm string // Normalize(Text) — по нему идёт поиск
}

// Normalize приводит текст к виду для сравнения: нижний регистр, «ё» → «е».
// Для кириллицы и латиницы длина в байтах не меняется, поэтому позиции
// в Norm совпадают с позициями в исходном тексте.
func Normalize(s string) string {
	return strings.ReplaceAll(strings.ToLower(s), "ё", "е")
}

// Cache хранит извлечённый текст, чтобы повторный поиск по той же папке
// не перечитывал и не разбирал документы заново. Запись привязана к размеру
// и времени изменения файла: изменённый файл будет прочитан снова.
type Cache struct {
	mu    sync.Mutex
	items map[string]*Doc
	order []string
	size  int
	max   int
}

// NewCache создаёт кэш, занимающий не больше maxBytes.
func NewCache(maxBytes int) *Cache {
	return &Cache{items: map[string]*Doc{}, max: maxBytes}
}

var emptyDoc = &Doc{}

// Get возвращает текст документа из кэша или извлекает его.
// Если текст достать не удалось (скан, повреждённый файл), возвращается
// пустой документ — и он тоже запоминается, чтобы не разбирать файл снова.
func (c *Cache) Get(path string, size, modified int64) *Doc {
	key := path + "|" + strconv.FormatInt(size, 10) + "|" + strconv.FormatInt(modified, 10)

	c.mu.Lock()
	doc, ok := c.items[key]
	c.mu.Unlock()
	if ok {
		return doc
	}

	doc = emptyDoc
	if text, err := Extract(path, MaxText); err == nil && text != "" {
		doc = &Doc{Text: text, Norm: Normalize(text)}
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.items[key]; !ok {
		c.items[key] = doc
		c.order = append(c.order, key)
		c.size += len(doc.Text) + len(doc.Norm) + len(key)
		for c.size > c.max && len(c.order) > 0 {
			old := c.order[0]
			c.order = c.order[1:]
			if d := c.items[old]; d != nil {
				c.size -= len(d.Text) + len(d.Norm) + len(old)
			}
			delete(c.items, old)
		}
	}
	return doc
}
