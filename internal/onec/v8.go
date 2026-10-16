// Package onec читает и сохраняет файлы 1С: контейнеры 1С:Предприятия 8
// (.epf, .erf, .cf, .cfe) и составные OLE-документы 1С:Предприятия 7.7 (.ert, .md).
//
// Формат контейнера 1С 8 (по исходникам v8unpack):
//
//	заголовок файла: next_page_addr(4) page_size(4) storage_ver(4) reserved(4);
//	блок: "\r\n" + data_size + " " + page_size + " " + next_page_addr + " \r\n"
//	      (по 8 шестнадцатеричных цифр, у формата 8.3.16+ — по 16), затем данные;
//	первый блок — таблица элементов: тройки (адрес заголовка, адрес данных, 0x7fffffff);
//	заголовок элемента: дата создания(8), дата изменения(8), резерв(4), имя в UTF-16LE;
//	данные элемента обычно сжаты deflate без заголовка zlib и могут сами быть контейнером.
//
// Конфигурация .cf бывает под гигабайт, поэтому контейнер не читается в память
// целиком: таблица элементов и заголовки — сразу, данные элемента — когда понадобятся
// (load, peek), и их можно отпустить (unload). При сохранении нетронутые элементы
// копируются из исходного файла потоком.
package onec

import (
	"bufio"
	"bytes"
	"compress/flate"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strconv"
	"sync"
	"unicode/utf16"
)

const (
	undef32      = 0x7fffffff
	pageSize     = 512
	maxDepth     = 6         // вложенность контейнеров
	maxInflated  = 256 << 20 // защита от «бомбы» при распаковке
	maxBlockHops = 1 << 20   // защита от зацикленной цепочки страниц
	maxElems     = 1 << 20   // защита от испорченной таблицы элементов
)

var (
	errNotV8      = errors.New("это не контейнер 1С:Предприятия 8")
	errBrokenData = errors.New("блок выходит за конец данных")
)

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

// sniffSize — сколько байт с начала нужно, чтобы узнать формат.
var sniffSize = format16.fileHeader + format16.base + format16.blockHeader()

// v8Elem — элемент контейнера. Данные лежат в src по адресу addr и читаются
// по требованию. Пока элемент не изменён, при записи используются исходные
// байты, поэтому всё, чего мы не трогали, сохраняется побайтно.
type v8Elem struct {
	Name   string
	header []byte // исходный заголовок элемента (даты, имя)
	src    io.ReaderAt
	srcLen int64
	f      format
	addr   uint64 // адрес блока данных; f.undef — данных нет
	depth  int

	loaded   bool
	deflated bool       // данные были сжаты
	payload  []byte     // распакованные данные
	child    *container // если данные — вложенный контейнер
	dirty    bool
}

type container struct {
	f        format
	storeVer uint32
	elems    []*v8Elem
	index    map[string]*v8Elem
}

// detectFormat. Файл 8.3.16+ — это два контейнера подряд: заглушка в старом
// формате («конфигурация требует новой версии платформы») размером ровно 0x1359
// байт и за ней настоящий 64-битный контейнер. Поэтому сначала ищем его.
func detectFormat(b []byte) (format, bool) {
	if len(b) >= format16.base+format16.fileHeader+format16.blockHeader() &&
		binary.LittleEndian.Uint64(b[format16.base:]) == format16.undef && isBlockHeader(b[format16.base+format16.fileHeader:], format16) {
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

// IsV8 сообщает, что данные — контейнер 1С 8 (достаточно первых байт файла).
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

// blockReader читает данные блока по цепочке страниц, не загружая их целиком.
type blockReader struct {
	src      io.ReaderAt
	srcLen   int64
	f        format
	size     int64 // всего данных в блоке
	left     int64 // осталось прочитать
	off      int64 // позиция в src внутри текущей страницы
	pageLeft int64
	next     uint64
	hops     int
	seen     map[uint64]bool // страницы цепочки: ссылка назад — испорченный файл
}

// pageAt читает заголовок страницы по адресу addr. small > 0 — заодно прочитать
// столько байт данных (одним обращением к диску вместо двух).
func pageAt(src io.ReaderAt, srcLen int64, f format, addr uint64, small int) (size, page, next uint64, data []byte, err error) {
	off := int64(addr) + int64(f.base)
	hdr := int64(f.blockHeader())
	if addr >= uint64(srcLen) || off+hdr > srcLen {
		return 0, 0, 0, nil, errBrokenData
	}
	buf := make([]byte, min(hdr+int64(small), srcLen-off))
	if _, err = src.ReadAt(buf, off); err != nil && err != io.EOF {
		return 0, 0, 0, nil, err
	}
	if size, page, next, err = f.parseHeader(buf); err != nil {
		return 0, 0, 0, nil, err
	}
	return size, page, next, buf[hdr:], nil
}

func openBlock(src io.ReaderAt, srcLen int64, f format, addr uint64) (*blockReader, error) {
	size, page, next, _, err := pageAt(src, srcLen, f, addr, 0)
	if err != nil {
		return nil, err
	}
	if size > uint64(srcLen) {
		return nil, errors.New("неверный размер блока")
	}
	off := int64(addr) + int64(f.base) + int64(f.blockHeader())
	return &blockReader{src: src, srcLen: srcLen, f: f, size: int64(size), left: int64(size),
		off: off, pageLeft: min(int64(page), int64(size)), next: next, seen: map[uint64]bool{addr: true}}, nil
}

func (r *blockReader) Read(p []byte) (int, error) {
	if r.left == 0 {
		return 0, io.EOF
	}
	if r.pageLeft == 0 {
		if r.next == r.f.undef {
			return 0, errors.New("цепочка блоков оборвалась")
		}
		if r.hops++; r.hops > maxBlockHops || r.seen[r.next] {
			return 0, errors.New("повреждена цепочка блоков")
		}
		r.seen[r.next] = true
		_, page, next, _, err := pageAt(r.src, r.srcLen, r.f, r.next, 0)
		if err != nil {
			return 0, err
		}
		r.off = int64(r.next) + int64(r.f.base) + int64(r.f.blockHeader())
		r.pageLeft, r.next = min(int64(page), r.left), next
	}
	n := int(min(int64(len(p)), r.pageLeft, r.left))
	if r.off+int64(n) > r.srcLen {
		return 0, errBrokenData
	}
	n, err := r.src.ReadAt(p[:n], r.off)
	r.off += int64(n)
	r.pageLeft -= int64(n)
	r.left -= int64(n)
	if err == io.EOF && n > 0 {
		err = nil
	}
	return n, err
}

// readBlock — данные блока целиком. Маленькие блоки читаются за одно обращение.
func readBlock(src io.ReaderAt, srcLen int64, f format, addr uint64) ([]byte, error) {
	const guess = 512
	size, page, _, data, err := pageAt(src, srcLen, f, addr, guess)
	if err != nil {
		return nil, err
	}
	if size <= page && size <= uint64(len(data)) {
		return data[:size:size], nil
	}
	r, err := openBlock(src, srcLen, f, addr)
	if err != nil {
		return nil, err
	}
	out := make([]byte, r.size)
	if _, err := io.ReadFull(r, out); err != nil {
		return nil, err
	}
	return out, nil
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
	r := flatePool.Get().(io.ReadCloser)
	defer flatePool.Put(r)
	if err := r.(flate.Resetter).Reset(bytes.NewReader(b), nil); err != nil {
		return nil, err
	}
	out := bytes.NewBuffer(make([]byte, 0, min(len(b)*4, maxInflated)))
	if _, err := io.Copy(out, io.LimitReader(r, maxInflated+1)); err != nil {
		return nil, err
	}
	if out.Len() > maxInflated {
		return nil, errors.New("слишком большой элемент")
	}
	return out.Bytes(), nil
}

// parseContainer — контейнер из байтов в памяти (вложенный или маленький файл).
func parseContainer(b []byte, depth int) (*container, error) {
	return openContainer(bytes.NewReader(b), int64(len(b)), depth)
}

// openContainer читает таблицу элементов и их заголовки; данные — позже, по запросу.
func openContainer(src io.ReaderAt, srcLen int64, depth int) (*container, error) {
	head := make([]byte, min(int64(sniffSize), srcLen))
	if _, err := src.ReadAt(head, 0); err != nil && err != io.EOF {
		return nil, err
	}
	f, ok := detectFormat(head)
	if !ok {
		return nil, errNotV8
	}
	if depth > maxDepth {
		return nil, errors.New("слишком глубокая вложенность контейнеров")
	}
	c := &container{f: f}
	if f == format15 {
		c.storeVer = binary.LittleEndian.Uint32(head[8:])
	}
	toc, err := readBlock(src, srcLen, f, uint64(f.fileHeader))
	if err != nil {
		return nil, fmt.Errorf("таблица элементов: %w", err)
	}
	step := f.addrSize * 3
	if len(toc)/step > maxElems {
		return nil, errors.New("повреждена таблица элементов")
	}
	c.elems = make([]*v8Elem, 0, len(toc)/step)
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
		header, err := readBlock(src, srcLen, f, headerAddr)
		if err != nil {
			return nil, fmt.Errorf("заголовок элемента: %w", err)
		}
		c.elems = append(c.elems, &v8Elem{Name: elemName(header), header: header,
			src: src, srcLen: srcLen, f: f, addr: dataAddr, depth: depth})
	}
	return c, nil
}

// load читает и распаковывает данные элемента; вложенный контейнер — разбирает.
func (e *v8Elem) load() error {
	if e.loaded {
		return nil
	}
	if e.addr != e.f.undef {
		stored, err := readBlock(e.src, e.srcLen, e.f, e.addr)
		if err != nil {
			return fmt.Errorf("элемент %s: %w", e.Name, err)
		}
		e.payload = stored
		if len(stored) > 0 {
			if inf, err := inflateLimited(stored); err == nil {
				e.payload, e.deflated = inf, true
			}
		}
	}
	if IsV8(e.payload) {
		child, err := parseContainer(e.payload, e.depth+1)
		if err != nil {
			return fmt.Errorf("элемент %s: %w", e.Name, err)
		}
		e.child = child
	}
	e.loaded = true
	return nil
}

// unload отпускает прочитанные данные, если они не изменены.
func (e *v8Elem) unload() {
	if e.loaded && !e.isDirty() {
		e.loaded, e.payload, e.child, e.deflated = false, nil, nil, false
	}
}

// Распаковщики и буферы для peek переиспользуются: у конфигурации десятки
// тысяч элементов, и новый распаковщик (окно 32 КБ) на каждый — гигабайты мусора.
var (
	flatePool = sync.Pool{New: func() any { return flate.NewReader(bytes.NewReader(nil)) }}
	bufioPool = sync.Pool{New: func() any { return bufio.NewReaderSize(nil, 4<<10) }}
)

// peek — первые n байт распакованных данных без чтения элемента целиком.
// Результат — новый срез, его можно хранить.
func (e *v8Elem) peek(n int) ([]byte, error) {
	if e.loaded {
		return e.payload[:min(n, len(e.payload))], nil
	}
	if e.addr == e.f.undef {
		return nil, nil
	}
	r, err := openBlock(e.src, e.srcLen, e.f, e.addr)
	if err != nil {
		return nil, err
	}
	buf := make([]byte, min(int64(n), maxInflated))
	br := bufioPool.Get().(*bufio.Reader)
	br.Reset(r)
	fr := flatePool.Get().(io.ReadCloser)
	fr.(flate.Resetter).Reset(br, nil)
	k, err := io.ReadFull(fr, buf)
	br.Reset(nil)
	bufioPool.Put(br)
	flatePool.Put(fr)
	if k > 0 && (err == nil || err == io.ErrUnexpectedEOF || err == io.EOF) {
		return buf[:k:k], nil
	}
	// Не сжато — первые байты как есть.
	if r, err = openBlock(e.src, e.srcLen, e.f, e.addr); err != nil {
		return nil, err
	}
	buf = buf[:min(int64(n), r.size)]
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

// childNames — имена элементов вложенного контейнера (nil, если данные — не
// контейнер) и начало данных. Читается только начало элемента: таблица
// и заголовки вложенного контейнера лежат в самом начале.
func (e *v8Elem) childNames() (names []string, head []byte, err error) {
	if e.loaded {
		head = e.payload[:min(len(e.payload), headSize)]
		if e.child == nil {
			return nil, head, nil
		}
		for _, ce := range e.child.elems {
			names = append(names, ce.Name)
		}
		return names, head, nil
	}
	// Обычно таблица и заголовки вложенного контейнера умещаются в 8 КБ.
	for _, n := range []int{8 << 10, 256 << 10, 4 << 20} {
		head, err = e.peek(n)
		if err != nil {
			return nil, nil, err
		}
		if !IsV8(head) {
			return nil, head, nil
		}
		c, err := parseContainer(head, e.depth+1)
		if err == nil {
			for _, ce := range c.elems {
				names = append(names, ce.Name)
			}
			return names, head, nil
		}
		if len(head) < n {
			return nil, nil, err // данные кончились — контейнер испорчен
		}
	}
	// Огромная таблица — читаем элемент целиком.
	if err := e.load(); err != nil {
		return nil, nil, err
	}
	defer e.unload()
	return e.childNames()
}

func (c *container) find(name string) *v8Elem {
	if c.index == nil {
		c.index = make(map[string]*v8Elem, len(c.elems))
		for _, e := range c.elems {
			if _, ok := c.index[e.Name]; !ok {
				c.index[e.Name] = e
			}
		}
	}
	return c.index[name]
}

// --- Запись -------------------------------------------------------------------

// Контейнер всегда записывается в старом формате (до 8.3.16): его читают все
// версии платформы 8.x, в том числе 8.3.16 и новее.

func writeBlockHeader(w io.Writer, size, page int64) error {
	_, err := fmt.Fprintf(w, "\r\n%08x %08x %08x \r\n", size, page, undef32)
	return err
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

// encoded — новые данные изменённого элемента в том виде, как их хранить в файле.
func (e *v8Elem) encoded() ([]byte, error) {
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

// storedSize — размер исходных данных элемента в файле.
func (e *v8Elem) storedSize() (int64, error) {
	if e.addr == e.f.undef {
		return 0, nil
	}
	size, _, _, _, err := pageAt(e.src, e.srcLen, e.f, e.addr, 0)
	return int64(size), err
}

func (c *container) bytes() ([]byte, error) {
	var out bytes.Buffer
	if err := c.writeTo(&out); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

var zeroPage [64 << 10]byte

func writeZeros(w io.Writer, n int64) error {
	for n > 0 {
		k := min(n, int64(len(zeroPage)))
		if _, err := w.Write(zeroPage[:k]); err != nil {
			return err
		}
		n -= k
	}
	return nil
}

// writeTo записывает контейнер. Изменённые элементы кодируются заново, остальные
// копируются из источника потоком — память не зависит от размера файла.
func (c *container) writeTo(w io.Writer) error {
	type item struct {
		e    *v8Elem
		data []byte // новые данные (у изменённых)
		size int64
	}
	items := make([]item, len(c.elems))
	for i, e := range c.elems {
		it := item{e: e}
		var err error
		if e.isDirty() {
			if it.data, err = e.encoded(); err != nil {
				return err
			}
			it.size = int64(len(it.data))
		} else if it.size, err = e.storedSize(); err != nil {
			return fmt.Errorf("элемент %s: %w", e.Name, err)
		}
		items[i] = it
	}

	hdr := int64(format15.blockHeader())
	tocSize := int64(len(items) * 12)
	tocPage := max(pageSize, tocSize)
	addr := int64(format15.fileHeader) + hdr + tocPage
	toc := make([]byte, tocSize)
	for i, it := range items {
		headerAddr := addr
		dataAddr := headerAddr + hdr + int64(len(it.e.header))
		addr = dataAddr + hdr + max(pageSize, it.size)
		if addr > undef32 {
			return errors.New("файл получается больше 2 ГБ — такой контейнер 1С не поддерживает")
		}
		binary.LittleEndian.PutUint32(toc[i*12:], uint32(headerAddr))
		binary.LittleEndian.PutUint32(toc[i*12+4:], uint32(dataAddr))
		binary.LittleEndian.PutUint32(toc[i*12+8:], undef32)
	}

	head := make([]byte, 16)
	binary.LittleEndian.PutUint32(head, undef32)
	binary.LittleEndian.PutUint32(head[4:], pageSize)
	ver := c.storeVer
	if ver == 0 {
		ver = 0x1B
	}
	binary.LittleEndian.PutUint32(head[8:], ver)
	if _, err := w.Write(head); err != nil {
		return err
	}
	if err := writeBlockHeader(w, tocSize, tocPage); err != nil {
		return err
	}
	if _, err := w.Write(toc); err != nil {
		return err
	}
	if err := writeZeros(w, tocPage-tocSize); err != nil {
		return err
	}
	for _, it := range items {
		h := it.e.header
		if err := writeBlockHeader(w, int64(len(h)), int64(len(h))); err != nil {
			return err
		}
		if _, err := w.Write(h); err != nil {
			return err
		}
		if err := writeBlockHeader(w, it.size, max(pageSize, it.size)); err != nil {
			return err
		}
		if it.data != nil || it.size == 0 {
			if _, err := w.Write(it.data); err != nil {
				return err
			}
		} else {
			r, err := openBlock(it.e.src, it.e.srcLen, it.e.f, it.e.addr)
			if err != nil {
				return fmt.Errorf("элемент %s: %w", it.e.Name, err)
			}
			if _, err := io.CopyN(w, r, it.size); err != nil {
				return fmt.Errorf("элемент %s: %w", it.e.Name, err)
			}
		}
		if err := writeZeros(w, max(pageSize, it.size)-it.size); err != nil {
			return err
		}
	}
	return nil
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
