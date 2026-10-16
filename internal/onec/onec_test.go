package onec

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	mainGUID   = "8ea1b72a-50d6-410b-be75-d340c5f0a097"
	objectID   = "265f2324-c4b4-4cc4-b231-405102e48000"
	formGUID   = "d26457bd-9f8d-411b-ae43-2c3f45629304"
	ordFormID  = "931a21ba-13ac-4651-abed-5a66ec0e84a7"
	objectCode = "Процедура Привет() Экспорт\n\tСообщить(\"Привет, \"\"мир\"\"\");\nКонецПроцедуры\n"
	formCode   = "&НаКлиенте\nПроцедура Открыть(Команда)\n\tСообщить(\"форма\");\nКонецПроцедуры\n"
	ordCode    = "Процедура КнопкаНажатие(Элемент)\nКонецПроцедуры\n"
)

func elem(name string, payload []byte, deflated bool) *v8Elem {
	e := &v8Elem{Name: name, header: newElemHeader(name), payload: payload, deflated: deflated}
	if IsV8(payload) {
		e.child, _ = parseContainer(payload, 1)
	}
	if deflated {
		e.stored, _ = deflate(payload)
	} else {
		e.stored = payload
	}
	return e
}

func mustBytes(t *testing.T, c *container) []byte {
	t.Helper()
	b, err := c.bytes()
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// sampleEPF собирает обработку, устроенную как настоящая .epf 1С 8.
func sampleEPF(t *testing.T) []byte {
	t.Helper()
	objModule := mustBytes(t, &container{f: format15, elems: []*v8Elem{
		elem("info", []byte(bom+"{3,1,0,\"\",0}"), false),
		elem("text", []byte(bom+windowsEOL(objectCode)), false),
	}})
	ordForm := mustBytes(t, &container{f: format15, elems: []*v8Elem{
		elem("form", []byte(bom+"{1,{2,\"Кнопка\"}}"), false),
		elem("module", []byte(bom+windowsEOL(ordCode)), false),
	}})
	help := base64.StdEncoding.EncodeToString([]byte("<html><head><style>x{}</style></head><body><h1>Справка</h1><p>Как пользоваться &amp; что делать</p></body></html>"))
	managed := bom + "{4,\r\n{50,0,{\"ru\",\"Текст запроса\"}},\"" + quote(windowsEOL(formCode)) + "\",\r\n{0,0}}"

	root := &container{f: format15, elems: []*v8Elem{
		elem("root", []byte(bom+"{2,"+mainGUID+",}"), true),
		elem(mainGUID, []byte(bom+"{1,\r\n{"+mainGUID+"},1,{0,\r\n{3,\r\n{1,0,"+objectID+"},\"КонсольЗапросов\",\r\n{1,\"ru\",\"Консоль запросов\"},\"\",0}}"), true),
		elem(objectID+".0", objModule, true),
		elem(objectID+".1", []byte(bom+"{5,1,\"ru\",\r\n{#base64:"+help+"}}"), true),
		elem(formGUID, []byte(bom+"{1,{1,{0,{13,{3,{1,0,"+formGUID+"},\"ТекстЗапроса\",{1,\"ru\",\"Текст запроса\"}}}}}}"), true),
		elem(formGUID+".0", []byte(managed), true),
		elem(ordFormID, []byte(bom+"{1,{3,{1,0,"+ordFormID+"},\"Форма\",{1,\"ru\",\"Форма\"}}}"), true),
		elem(ordFormID+".0", ordForm, true),
		elem("version", []byte(bom+"{{216,0,{80324,0}}}"), true),
	}}
	return mustBytes(t, root)
}

func writeTemp(t *testing.T, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func section(t *testing.T, doc Document, id string) Section {
	t.Helper()
	for _, s := range doc.Sections {
		if s.ID == id {
			return s
		}
	}
	t.Fatalf("нет раздела %q, есть: %+v", id, doc.Sections)
	return Section{}
}

func TestOpenEPF(t *testing.T) {
	f, err := Open(writeTemp(t, "Консоль.epf", sampleEPF(t)))
	if err != nil {
		t.Fatal(err)
	}
	doc := f.Doc
	if doc.Name != "КонсольЗапросов" || doc.Synonym != "Консоль запросов" || doc.Kind != "Внешняя обработка" || !doc.CanSave {
		t.Fatalf("шапка: %+v", doc)
	}
	if got := section(t, doc, "object").Text; got != objectCode {
		t.Errorf("модуль объекта: %q", got)
	}
	managed := section(t, doc, "form:"+formGUID)
	if managed.Title != "ТекстЗапроса" || managed.Text != formCode || !managed.Editable {
		t.Errorf("управляемая форма: %+v", managed)
	}
	if got := section(t, doc, "form:"+ordFormID).Text; got != ordCode {
		t.Errorf("обычная форма: %q", got)
	}
	help := section(t, doc, "help").Text
	if !strings.Contains(help, "Как пользоваться & что делать") || strings.Contains(help, "<") || strings.Contains(help, "x{}") {
		t.Errorf("справка: %q", help)
	}
}

func TestSaveEPF(t *testing.T) {
	original := sampleEPF(t)
	f, err := Open(writeTemp(t, "a.epf", original))
	if err != nil {
		t.Fatal(err)
	}
	newObject := "Процедура Новая()\n\t// изменено в «Поиске файлов»\nКонецПроцедуры\n"
	newForm := "&НаСервере\nПроцедура Тест()\n\tА = \"кавычки \"\"внутри\"\"\";\nКонецПроцедуры\n"
	saved, err := f.Save(map[string]string{"object": newObject, "form:" + formGUID: newForm})
	if err != nil {
		t.Fatal(err)
	}

	g, err := Open(writeTemp(t, "b.epf", saved))
	if err != nil {
		t.Fatalf("сохранённый файл не открывается: %v", err)
	}
	if got := section(t, g.Doc, "object").Text; got != newObject {
		t.Errorf("модуль объекта после сохранения: %q", got)
	}
	if got := section(t, g.Doc, "form:"+formGUID).Text; got != newForm {
		t.Errorf("модуль формы после сохранения: %q", got)
	}
	if got := section(t, g.Doc, "form:"+ordFormID).Text; got != ordCode {
		t.Errorf("нетронутая форма изменилась: %q", got)
	}

	// Нетронутые элементы переносятся побайтно, у изменённых сохраняются BOM и \r\n.
	for _, name := range []string{"root", mainGUID, objectID + ".1", ordFormID + ".0", "version"} {
		if !bytes.Equal(f.v8.find(name).stored, g.v8.find(name).stored) {
			t.Errorf("элемент %s изменился, хотя его не трогали", name)
		}
	}
	text := g.v8.find(objectID + ".0").child.find("text").payload
	if !bytes.HasPrefix(text, []byte(bom)) || !bytes.Contains(text, []byte("\r\n")) {
		t.Errorf("модуль объекта потерял BOM или \\r\\n: %q", text[:min(len(text), 20)])
	}
}

func TestResaveWithoutChangesIsStable(t *testing.T) {
	original := sampleEPF(t)
	f, err := Open(writeTemp(t, "a.epf", original))
	if err != nil {
		t.Fatal(err)
	}
	saved, err := f.Save(nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(saved, original) {
		t.Error("сохранение без правок должно давать тот же файл")
	}
}

func TestRejectsDamagedFiles(t *testing.T) {
	good := sampleEPF(t)
	cases := map[string][]byte{
		"пустой":          {},
		"не 1С":           []byte("hello world"),
		"обрезанный":      good[:len(good)/3],
		"битый заголовок": append(append([]byte{}, good[:20]...), bytes.Repeat([]byte{'x'}, 200)...),
	}
	for name, data := range cases {
		if _, err := Open(writeTemp(t, "x.epf", data)); err == nil {
			t.Errorf("%s: ожидалась ошибка", name)
		}
	}
}

func TestLoopedBlockChain(t *testing.T) {
	// Страница, ссылающаяся сама на себя, не должна зацикливать чтение.
	var buf bytes.Buffer
	buf.Write([]byte{0xff, 0xff, 0xff, 0x7f, 0, 2, 0, 0, 0x1b, 0, 0, 0, 0, 0, 0, 0})
	buf.WriteString("\r\n00000400 00000010 00000010 \r\n")
	buf.Write(make([]byte, 16))
	if _, err := parseContainer(buf.Bytes(), 0); err == nil {
		t.Fatal("ожидалась ошибка зацикленной цепочки")
	}
}

func TestStringsAtDepth(t *testing.T) {
	s := `{1,"верх",{2,"низ ""в кавычках"""},"второй"}`
	var got []string
	for _, sp := range stringsAtDepth(s, 1) {
		got = append(got, unquote(s[sp[0]:sp[1]]))
	}
	if strings.Join(got, "|") != "верх|второй" {
		t.Fatalf("строки уровня 1: %q", got)
	}
}

func TestCutMFCLength(t *testing.T) {
	if got := cutMFCLength([]byte{3, 'a', 'b', 'c'}); string(got) != "abc" {
		t.Errorf("1 байт: %q", got)
	}
	if got := cutMFCLength([]byte{0xFF, 0x00, 0x01, 'x'}); string(got) != "x" {
		t.Errorf("0xFF + 2 байта: %q", got)
	}
	if got := cutMFCLength([]byte{0xFF, 0xFF, 0xFF, 0, 0, 1, 0, 'y'}); string(got) != "y" {
		t.Errorf("0xFF 0xFFFF + 4 байта: %q", got)
	}
}

// testdata/sample77.ert собран скриптом testdata/make-ert.ps1 через системный
// StgCreateDocfile: OLE-документ с потоками, как у внешней обработки 1С 7.7.
func TestOpenERT(t *testing.T) {
	f, err := Open(filepath.Join("testdata", "sample77.ert"))
	if err != nil {
		t.Fatal(err)
	}
	doc := f.Doc
	if doc.Format != "1С:Предприятие 7.7" || doc.Name != "sample77" || !doc.CanSave {
		t.Fatalf("шапка: %+v", doc)
	}
	module := section(t, doc, "module")
	want := "Перем Итог;\n\nПроцедура Сформировать()\n\tИтог = 0;\n\tСообщить(\"Отчёт сформирован\");\nКонецПроцедуры\n"
	if module.Text != want || module.Kind != "module" {
		t.Errorf("модуль 7.7 (deflate + Windows-1251): %q", module.Text)
	}
	if dialog := section(t, doc, "dialog").Text; !strings.Contains(dialog, "Кнопка Сформировать") || strings.HasPrefix(dialog, "\x00") {
		t.Errorf("описание формы (длина MFC + Windows-1251): %q", dialog)
	}
	if !strings.Contains(section(t, doc, "streams").Text, "Container.Contents") {
		t.Error("в составе файла нет служебных потоков")
	}
	text, err := ExtractText(filepath.Join("testdata", "sample77.ert"))
	if err != nil || !strings.Contains(text, "Отчёт сформирован") {
		t.Errorf("текст для поиска: %q, %v", text, err)
	}
}

func copyFile(t *testing.T, src, name string) string {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	return writeTemp(t, name, data)
}

// .ert сохраняется в .ert: меняется только модуль, остальное копируется как есть.
func TestSaveERT(t *testing.T) {
	src := copyFile(t, filepath.Join("testdata", "sample77.ert"), "sample77.ert")
	f, err := Open(src)
	if err != nil {
		t.Fatal(err)
	}
	newModule := "Процедура Сформировать()\n\tСообщить(\"Ёлка и смайлик 😀\");\nКонецПроцедуры\n"
	dst := filepath.Join(filepath.Dir(src), "сохранённый.ert")
	if err := f.SaveTo(dst, map[string]string{"module": newModule}); err != nil {
		t.Fatal(err)
	}

	g, err := Open(dst)
	if err != nil {
		t.Fatalf("сохранённый .ert не открывается: %v", err)
	}
	// Смайлика нет в Windows-1251 — он заменяется на «?», остальное сохраняется.
	want := strings.Replace(newModule, "😀", "?", 1)
	if got := section(t, g.Doc, "module").Text; got != want {
		t.Errorf("модуль после сохранения: %q", got)
	}
	if section(t, g.Doc, "dialog").Text != section(t, f.Doc, "dialog").Text {
		t.Error("описание формы изменилось, хотя его не трогали")
	}
	if section(t, g.Doc, "streams").Text != section(t, f.Doc, "streams").Text {
		t.Error("состав файла изменился")
	}
	if _, err := os.Stat(dst + ".tmp-filesearch"); !os.IsNotExist(err) {
		t.Error("временный файл не удалён")
	}
}

// «Сохранить» поверх исходного файла: файл заменяется атомарно.
func TestSaveToOverwrite(t *testing.T) {
	path := writeTemp(t, "a.epf", sampleEPF(t))
	f, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.SaveTo(path, map[string]string{"object": "// перезаписано\n"}); err != nil {
		t.Fatal(err)
	}
	g, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := section(t, g.Doc, "object").Text; got != "// перезаписано\n" {
		t.Errorf("после перезаписи: %q", got)
	}
	if section(t, g.Doc, "form:"+formGUID).Text != formCode {
		t.Error("модуль формы изменился, хотя его не трогали")
	}
}

// Проверка на настоящей обработке — если указать путь в ONEC_SAMPLE.
// Сам файл в репозиторий не кладём: это чужая разработка.
func TestRealSample(t *testing.T) {
	path := os.Getenv("ONEC_SAMPLE")
	if path == "" {
		t.Skip("ONEC_SAMPLE не задан")
	}
	f, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%s «%s» (%s), разделов: %d", f.Doc.Kind, f.Doc.Name, f.Doc.Synonym, len(f.Doc.Sections))
	for _, s := range f.Doc.Sections {
		t.Logf("  [%s] %s — %d символов, правка: %v %s", s.Group, s.Title, len([]rune(s.Text)), s.Editable, s.Note)
	}
	if f.Doc.CanSave {
		saved, err := f.Save(nil)
		if err != nil {
			t.Fatal(err)
		}
		g, err := Open(writeTemp(t, "copy"+filepath.Ext(path), saved))
		if err != nil {
			t.Fatalf("пересохранённый файл не открывается: %v", err)
		}
		if len(g.Doc.Sections) != len(f.Doc.Sections) {
			t.Fatalf("после пересохранения разделов %d, было %d", len(g.Doc.Sections), len(f.Doc.Sections))
		}
	}
}
