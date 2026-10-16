// Package onec читает внешние обработки и отчёты 1С: контейнеры 1С:Предприятия 8
// (.epf, .erf) и составные OLE-документы 1С:Предприятия 7.7 (.ert).
//
// Формат контейнера 1С 8 (по исходникам v8unpack):
//
//	заголовок файла: next_page_addr(4) page_size(4) storage_ver(4) reserved(4);
//	блок: "\r\n" + data_size + " " + page_size + " " + next_page_addr + " \r\n"
//	      (по 8 шестнадцатеричных цифр, у формата 8.3.16+ — по 16), затем данные;
//	первый блок — таблица элементов: тройки (адрес заголовка, адрес данных, 0x7fffffff);
//	заголовок элемента: дата создания(8), дата изменения(8), резерв(4), имя в UTF-16LE;
//	данные элемента обычно сжаты deflate без заголовка zlib и могут сами быть контейнером.
package onec

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strconv"
	"unicode/utf16"
)

const (
	undef32      = 0x7fffffff
	pageSize     = 512
	maxDepth     = 6         // вложенность контейнеров
	maxInflated  = 256 << 20 // защита от «бомбы» при распаковке
	maxBlockHops = 1 << 20   // защита от зацикленной цепочки страниц
)

var errNotV8 = errors.New("это не контейнер 1С:Предприятия 8")

// format описывает ширину полей: старый (до 8.3.16) или новый 64-битный контейнер.
type format struct {
	fileHeader int // размер заголовка файла
	hexDigits  int // цифр в полях заголовка блока
	addrSize   int // байт в адресе таблицы элементов
	undef      uint64
	base       int // смещение адресов (у нового формата — «магическое» 0x1359)
}

var (
	format15 = format{fileHeader: 16, hexDigits: 8, addrSize: 4, undef: undef32, base: 0}
	format16 = format{fileHeader: 20, hexDigits: 16, addrSize: 8, undef: 0xffffffffffffffff, base: 0x1359}
)

func (f format) blockHeader() int { return 2 + f.hexDigits*3 + 3 + 2 }

// v8Elem — элемент контейнера. Пока элемент не изменён, при записи
// используются исходные байты (stored), поэтому всё, чего мы не трогали,
// сохраняется побайтно.
type v8Elem struct {
	Name     string
	header   []byte     // исходный заголовок элемента (даты, имя)
	stored   []byte     // данные элемента в файле (возможно, сжатые)
	deflated bool       // данные были сжаты
	payload  []byte     // распакованные данные
	child    *container // если данные — вложенный контейнер
	dirty    bool
}

type container struct {
	f        format
	storeVer uint32
	elems    []*v8Elem
}

func detectFormat(b []byte) (format, bool) {
	if len(b) >= format16.fileHeader+format16.blockHeader() &&
		binary.LittleEndian.Uint64(b) == format16.undef && isBlockHeader(b[format16.fileHeader+format16.base:], format16) {
		return format16, true
	}
	if len(b) >= format15.fileHeader+format15.blockHeader() &&
		binary.LittleEndian.Uint32(b) == undef32 && isBlockHeader(b[format15.fileHeader:], format15) {
		return format15, true
	}
	return format{}, false
}

func isBlockHeader(b []byte, f format) bool {
	n := f.blockHeader()
	if len(b) < n || b[0] != '\r' || b[1] != '\n' || b[n-2] != '\r' || b[n-1] != '\n' {
		return false
	}
	for i := 0; i < 3; i++ {
		if b[2+(i+1)*f.hexDigits+i] != ' ' {
			return false
		}
	}
	return true
}

// IsV8 сообщает, что данные — контейнер 1С 8.
func IsV8(b []byte) bool {
	_, ok := detectFormat(b)
	return ok
}

func (f format) parseHeader(b []byte) (dataSize, page, next uint64, err error) {
	if !isBlockHeader(b, f) {
		return 0, 0, 0, errors.New("повреждён заголовок блока")
	}
	field := func(i int) (uint64, error) {
		start := 2 + i*(f.hexDigits+1)
		return strconv.ParseUint(string(b[start:start+f.hexDigits]), 16, 64)
	}
	if dataSize, err = field(0); err != nil {
		return
	}
	if page, err = field(1); err != nil {
		return
	}
	next, err = field(2)
	return
}

// readBlock собирает данные блока по цепочке страниц.
func (f format) readBlock(b []byte, addr uint64) ([]byte, error) {
	var out []byte
	total := -1
	for hops := 0; ; hops++ {
		off := int(addr) + f.base
		if hops > maxBlockHops || addr > uint64(len(b)) || off+f.blockHeader() > len(b) {
			return nil, errors.New("повреждена цепочка блоков")
		}
		size, page, next, err := f.parseHeader(b[off:])
		if err != nil {
			return nil, err
		}
		if total < 0 {
			if size > uint64(len(b)) {
				return nil, errors.New("неверный размер блока")
			}
			total = int(size)
			out = make([]byte, 0, total)
		}
		n := int(min(page, uint64(total-len(out))))
		start := off + f.blockHeader()
		if start+n > len(b) {
			return nil, errors.New("блок выходит за конец файла")
		}
		out = append(out, b[start:start+n]...)
		if next == f.undef || len(out) >= total {
			return out, nil
		}
		addr = next
	}
}

func elemName(header []byte) string {
	var u []uint16
	for i := 20; i+1 < len(header); i += 2 {
		c := binary.LittleEndian.Uint16(header[i:])
		if c == 0 {
			break
		}
		u = append(u, c)
	}
	return string(utf16.Decode(u))
}

func inflateLimited(b []byte) ([]byte, error) {
	r := flate.NewReader(bytes.NewReader(b))
	defer r.Close()
	out, err := io.ReadAll(io.LimitReader(r, maxInflated+1))
	if err != nil {
		return nil, err
	}
	if len(out) > maxInflated {
		return nil, errors.New("слишком большой элемент")
	}
	return out, nil
}

func parseContainer(b []byte, depth int) (*container, error) {
	f, ok := detectFormat(b)
	if !ok {
		return nil, errNotV8
	}
	if depth > maxDepth {
		return nil, errors.New("слишком глубокая вложенность контейнеров")
	}
	c := &container{f: f}
	if f == format15 {
		c.storeVer = binary.LittleEndian.Uint32(b[8:])
	}
	toc, err := f.readBlock(b, uint64(f.fileHeader))
	if err != nil {
		return nil, fmt.Errorf("таблица элементов: %w", err)
	}
	step := f.addrSize * 3
	for i := 0; i+step <= len(toc); i += step {
		read := func(j int) uint64 {
			if f.addrSize == 4 {
				return uint64(binary.LittleEndian.Uint32(toc[i+j*4:]))
			}
			return binary.LittleEndian.Uint64(toc[i+j*8:])
		}
		headerAddr, dataAddr, sentinel := read(0), read(1), read(2)
		if sentinel != f.undef {
			break
		}
		header, err := f.readBlock(b, headerAddr)
		if err != nil {
			return nil, fmt.Errorf("заголовок элемента: %w", err)
		}
		e := &v8Elem{Name: elemName(header), header: header}
		if dataAddr != f.undef {
			if e.stored, err = f.readBlock(b, dataAddr); err != nil {
				return nil, fmt.Errorf("элемент %s: %w", e.Name, err)
			}
		}
		e.payload = e.stored
		if inf, err := inflateLimited(e.stored); err == nil && len(e.stored) > 0 {
			e.payload, e.deflated = inf, true
		}
		if IsV8(e.payload) {
			if e.child, err = parseContainer(e.payload, depth+1); err != nil {
				return nil, fmt.Errorf("элемент %s: %w", e.Name, err)
			}
		}
		c.elems = append(c.elems, e)
	}
	return c, nil
}

func (c *container) find(name string) *v8Elem {
	for _, e := range c.elems {
		if e.Name == name {
			return e
		}
	}
	return nil
}

// --- Запись -------------------------------------------------------------------

// Контейнер всегда записывается в старом формате (до 8.3.16): его читают все
// версии платформы 8.x, в том числе 8.3.16 и новее.

func writeBlock(buf *bytes.Buffer, data []byte, page int) {
	if page < len(data) {
		page = len(data)
	}
	fmt.Fprintf(buf, "\r\n%08x %08x %08x \r\n", len(data), page, undef32)
	buf.Write(data)
	buf.Write(make([]byte, page-len(data)))
}

func deflate(b []byte) ([]byte, error) {
	var out bytes.Buffer
	w, err := flate.NewWriter(&out, flate.BestCompression)
	if err != nil {
		return nil, err
	}
	if _, err := w.Write(b); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// isDirty — изменён ли элемент или что-то внутри него.
func (e *v8Elem) isDirty() bool {
	if e.dirty {
		return true
	}
	if e.child != nil {
		for _, ce := range e.child.elems {
			if ce.isDirty() {
				return true
			}
		}
	}
	return false
}

// storedBytes — данные элемента в том виде, в котором они лежат в файле.
func (e *v8Elem) storedBytes() ([]byte, error) {
	if !e.isDirty() && e.stored != nil {
		return e.stored, nil
	}
	payload := e.payload
	if e.child != nil {
		var err error
		if payload, err = e.child.bytes(); err != nil {
			return nil, err
		}
	}
	if e.deflated {
		return deflate(payload)
	}
	return payload, nil
}

func (c *container) bytes() ([]byte, error) {
	type item struct{ header, data []byte }
	items := make([]item, len(c.elems))
	for i, e := range c.elems {
		data, err := e.storedBytes()
		if err != nil {
			return nil, err
		}
		items[i] = item{e.header, data}
	}

	tocSize := len(items) * 12
	tocPage := max(pageSize, tocSize)
	addr := format15.fileHeader + format15.blockHeader() + tocPage
	toc := make([]byte, tocSize)
	var body bytes.Buffer
	for i, it := range items {
		headerAddr := addr
		var block bytes.Buffer
		writeBlock(&block, it.header, len(it.header))
		dataAddr := headerAddr + block.Len()
		writeBlock(&block, it.data, max(pageSize, len(it.data)))
		binary.LittleEndian.PutUint32(toc[i*12:], uint32(headerAddr))
		binary.LittleEndian.PutUint32(toc[i*12+4:], uint32(dataAddr))
		binary.LittleEndian.PutUint32(toc[i*12+8:], undef32)
		body.Write(block.Bytes())
		addr += block.Len()
	}

	var out bytes.Buffer
	head := make([]byte, 16)
	binary.LittleEndian.PutUint32(head, undef32)
	binary.LittleEndian.PutUint32(head[4:], pageSize)
	ver := c.storeVer
	if ver == 0 {
		ver = 0x1B
	}
	binary.LittleEndian.PutUint32(head[8:], ver)
	out.Write(head)
	writeBlock(&out, toc, tocPage)
	out.Write(body.Bytes())
	return out.Bytes(), nil
}

// newElemHeader — заголовок нового элемента (для тестов и сборки).
func newElemHeader(name string) []byte {
	u := utf16.Encode([]rune(name))
	h := make([]byte, 20+len(u)*2+4)
	for i, c := range u {
		binary.LittleEndian.PutUint16(h[20+i*2:], c)
	}
	return h
}
