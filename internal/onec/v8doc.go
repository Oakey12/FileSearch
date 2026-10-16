package onec

import (
	"encoding/base64"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Разбор содержимого контейнера 1С 8 — одинаково для внешних обработок и отчётов
// (.epf, .erf) и для конфигураций и расширений (.cf, .cfe).
//
// Каждый объект метаданных — элемент с именем-GUID (описание в скобочном формате)
// и элементы «<GUID>.<N>» с его частями: модулями, формой, макетом, справкой.
// Модуль — вложенный контейнер с элементом text (если модуль сохранён без
// исходного текста, там только image).

// Классы объектов конфигурации: GUID класса → название группы в дереве.
// Проверены на реальных конфигурациях; остальные — по описаниям формата.
var classLabels = map[string]string{
	"0fe48980-252d-11d6-a3c7-0050bae0a776": "Общие модули",
	"2f1a5187-fb0e-4b05-9489-dc5dd6412348": "Общие команды",
	"07ee8426-87f1-11d5-b99c-0050bae0a95d": "Общие формы",
	"0c89c792-16c3-11d5-b96b-0050bae0a95d": "Общие макеты",
	"7dcd43d9-aca5-4926-b549-1842e6a4e8cf": "Общие картинки",
	"37f2fa9a-b276-11d4-9435-004095e12fc7": "Подсистемы",
	"09736b02-9cac-4e3f-b4f7-d3e9576ab948": "Роли",
	"0195e80c-b157-11d4-9435-004095e12fc7": "Константы",
	"cf4abea6-37b2-11d4-940f-008048da11f9": "Справочники",
	"061d872a-5787-460e-95ac-ed74ea3a3e84": "Документы",
	"4612bd75-71b7-4a5c-8cc5-2b0b65f9fa0d": "Журналы документов",
	"f6a80749-5ad7-400b-8519-39dc5dff2542": "Перечисления",
	"631b75a0-29e2-11d6-a3c7-0050bae0a776": "Отчёты",
	"bf845118-327b-4682-b5c6-285d2a0eb296": "Обработки",
	"82a1b659-b220-4d94-a9bd-14d757b95a48": "Планы видов характеристик",
	"238e7e88-3c5f-48b2-8a3b-81ebbecb20ed": "Планы счетов",
	"30b100d6-b29f-47ac-aec7-cb8ca8a54767": "Планы видов расчёта",
	"13134201-f60b-11d5-a3c7-0050bae0a776": "Регистры сведений",
	"b64d9a40-1642-11d6-a3c7-0050bae0a776": "Регистры накопления",
	"2deed9b8-0056-4ffe-a473-c20a6c32a0bc": "Регистры бухгалтерии",
	"f2de87a8-64e5-45eb-a22d-b3aedab050e7": "Регистры расчёта",
	"fcd3404e-1523-48ce-9bc0-ecdb822684a1": "Бизнес-процессы",
	"3e63355c-1378-4953-be9b-1deb5fb6bec5": "Задачи",
	"857c4a91-e5f4-4fac-86ec-787626f1c108": "Планы обмена",
	"8657032e-7740-4e1d-a3ba-5dd6e8afb78f": "Веб-сервисы",
	"0fffc09c-8f4c-47cc-b41c-8d5c5a221d79": "HTTP-сервисы",
	"11bdaf85-d5ad-4d91-bb24-aa0eee139052": "Регламентные задания",
	"4e828da6-0f44-4b5b-b1c0-a2b3cfe7bdcc": "Подписки на события",
	"24c43748-c938-45d0-8d14-01424a72b11e": "Параметры сеанса",
	"46b4cd97-fd13-4eaa-aba2-3bddd7699218": "Хранилища настроек",
	"58848766-36ea-4076-8800-e91eb49590d7": "Элементы стиля",
	"9cd510ce-abfc-11d4-9434-004095e12fc7": "Языки",
	"c045099e-13b9-4fb6-9d50-fca00202971e": "Определяемые типы",
}

// Порядок групп в дереве конфигурации — как в Конфигураторе.
var configGroupOrder = []string{
	"Конфигурация", "Общие модули", "Общие команды", "Общие формы", "Общие макеты", "Подсистемы",
	"Константы", "Справочники", "Документы", "Журналы документов", "Перечисления", "Отчёты", "Обработки",
	"Планы видов характеристик", "Планы счетов", "Планы видов расчёта", "Регистры сведений",
	"Регистры накопления", "Регистры бухгалтерии", "Регистры расчёта", "Бизнес-процессы", "Задачи",
	"Планы обмена", "Веб-сервисы", "HTTP-сервисы", "Регламентные задания", "Подписки на события",
	"Хранилища настроек", "Прочие объекты",
}

var externalGroupOrder = []string{"Модули", "Формы", "Макеты", "Справка", "Описание"}

var (
	formDescRe     = regexp.MustCompile(`^\{1,\n(?:\{1,\n)?\{0,\n\{13,`)
	templateDescRe = regexp.MustCompile(`^\{1,\n\{2,(\d+),`)
	classGroupRe   = regexp.MustCompile(`\{([0-9a-f-]{36}),(\d+)((?:,\s*[0-9a-f-]{36})+)\}`)
)

// Типы макетов (второе число в описании макета).
const (
	tplSpreadsheet = 0
	tplPicture     = 1
	tplDCS         = 6
)

type v8Object struct {
	guid, name, synonym string
	class               string // группа в дереве («Справочники» …)
	isForm, isTemplate  bool
	tplType             int
	owner               string // GUID владельца формы или макета
}

type internalName struct{ name, owner string }

func normalizedDesc(e *v8Elem) string {
	return strings.ReplaceAll(strings.TrimPrefix(string(e.payload), bom), "\r\n", "\n")
}

func openV8(path string, data []byte) (*File, error) {
	root, err := parseContainer(data, 0)
	if err != nil {
		return nil, err
	}
	file := &File{v8: root, setters: map[string]func(string){}}
	doc := &file.Doc
	doc.Format = "1С:Предприятие 8"
	doc.CanSave = true

	// Объекты: имя, синоним, вид (форма, макет) по описанию.
	objs := map[string]*v8Object{}
	internal := map[string]internalName{} // внутренний id → имя и объект-владелец
	for _, e := range root.elems {
		if len(e.Name) != 36 || !guidRe.MatchString(e.Name) {
			continue
		}
		desc := normalizedDesc(e)
		o := &v8Object{guid: e.Name}
		names := objectNames(desc)
		for _, n := range names {
			if n.id == e.Name && o.name == "" {
				o.name, o.synonym = n.name, n.synonym
			}
			if _, seen := internal[n.id]; !seen {
				internal[n.id] = internalName{n.name, e.Name}
			}
		}
		if o.name == "" && len(names) > 0 {
			o.name, o.synonym = names[0].name, names[0].synonym
		}
		switch {
		case formDescRe.MatchString(desc):
			o.isForm = true
		case templateDescRe.MatchString(desc):
			o.isTemplate = true
			o.tplType, _ = strconv.Atoi(templateDescRe.FindStringSubmatch(desc)[1])
		}
		objs[e.Name] = o
	}

	// Корень: конфигурация («{2,{свой GUID},…») или главный объект обработки (из root).
	topGUID, isConfig := "", false
	for _, e := range root.elems {
		if len(e.Name) == 36 && strings.HasPrefix(normalizedDesc(e), "{2,\n{"+e.Name+"}") {
			topGUID, isConfig = e.Name, true
			break
		}
	}
	if topGUID == "" {
		if e := root.find("root"); e != nil {
			topGUID = guidRe.FindString(string(e.payload))
		}
	}
	top := objs[topGUID]
	if top == nil {
		return nil, errors.New("не найдено описание обработки или конфигурации")
	}
	doc.Name, doc.Synonym = top.name, top.synonym
	if doc.Name == "" {
		doc.Name = baseName(path)
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".erf":
		doc.Kind = "Внешний отчёт"
	case ".cfe":
		doc.Kind = "Расширение конфигурации"
	case ".cf":
		doc.Kind = "Конфигурация"
	default:
		doc.Kind = "Внешняя обработка"
		if isConfig {
			doc.Kind = "Конфигурация"
		}
	}

	// Классы объектов — из групп в описании конфигурации.
	if isConfig {
		for _, m := range classGroupRe.FindAllStringSubmatch(normalizedDesc(root.find(topGUID)), -1) {
			label, ok := classLabels[m[1]]
			if !ok {
				label = "Прочие объекты"
			}
			for _, g := range guidRe.FindAllString(m[3], -1) {
				if o := objs[g]; o != nil && o.class == "" {
					o.class = label
				}
			}
		}
	}

	// Владельцы форм и макетов: объект, в описании которого они упомянуты.
	for _, e := range root.elems {
		o := objs[e.Name]
		if o == nil || o.isForm || o.isTemplate || e.Name == topGUID {
			continue
		}
		for _, g := range guidRe.FindAllString(string(e.payload), -1) {
			if c := objs[g]; c != nil && c != o && (c.isForm || c.isTemplate) && c.owner == "" {
				c.owner = e.Name
			}
		}
	}

	b := &v8Builder{file: file, root: root, objs: objs, internal: internal, top: topGUID, isConfig: isConfig}
	for _, e := range root.elems {
		b.element(e)
	}
	if !isConfig {
		b.help()
	}
	b.sort()

	if len(doc.Sections) == 0 {
		return nil, errors.New("в файле не найдено ни модулей, ни форм")
	}
	return file, nil
}

type v8Builder struct {
	file     *File
	root     *container
	objs     map[string]*v8Object
	internal map[string]internalName
	top      string
	isConfig bool
}

// ownerOf — объект, которому принадлежит элемент «<prefix>.N»: сам объект
// с таким GUID или объект, в описании которого prefix — внутренний id.
func (b *v8Builder) ownerOf(prefix string) (o *v8Object, viaInternal bool) {
	if o := b.objs[prefix]; o != nil {
		return o, false
	}
	if in, ok := b.internal[prefix]; ok {
		return b.objs[in.owner], true
	}
	return nil, false
}

func (b *v8Builder) group(o *v8Object) string {
	if !b.isConfig {
		switch {
		case o != nil && o.isForm:
			return "Формы"
		case o != nil && o.isTemplate:
			return "Макеты"
		}
		return "Модули"
	}
	for o != nil {
		if o.guid == b.top {
			return "Конфигурация"
		}
		if o.class != "" {
			return o.class
		}
		if o.owner == "" {
			break
		}
		o = b.objs[o.owner]
	}
	return "Прочие объекты"
}

// title — «Владелец › Форма» для форм и макетов объектов, иначе имя.
func (b *v8Builder) title(o *v8Object) string {
	if o == nil {
		return "Без имени"
	}
	if (o.isForm || o.isTemplate) && o.owner != "" && o.owner != b.top {
		if owner := b.objs[o.owner]; owner != nil && owner.name != "" {
			return owner.name + " › " + o.name
		}
	}
	return o.name
}

// moduleName — название модуля по номеру части и классу владельца.
func moduleName(class string, n int, viaInternal bool) string {
	switch {
	case viaInternal && n == 2:
		return "Модуль команды"
	case class == "Общие модули":
		return ""
	case class == "Общие команды":
		return "Модуль команды"
	case class == "Константы" && n == 0:
		return "Модуль менеджера значения"
	case strings.HasPrefix(class, "Регистры") && n == 1:
		return "Модуль набора записей"
	case class == "Планы обмена" && n == 3:
		return "Модуль менеджера"
	case n == 0:
		return "Модуль объекта"
	case n == 2:
		return "Модуль менеджера"
	}
	return fmt.Sprintf("Модуль (%d)", n)
}

func (b *v8Builder) element(e *v8Elem) {
	dot := strings.LastIndex(e.Name, ".")
	if dot != 36 {
		return
	}
	prefix := e.Name[:36]
	n, err := strconv.Atoi(e.Name[37:])
	if err != nil {
		return
	}
	o, viaInternal := b.ownerOf(prefix)
	doc := &b.file.Doc

	// Модуль: вложенный контейнер с text (или только image — без исходного текста).
	if e.child != nil && (e.child.find("text") != nil || e.child.find("image") != nil) && e.child.find("module") == nil {
		owner := o
		title := b.title(owner)
		group := b.group(owner)
		kind := moduleName(group, n, viaInternal)
		if viaInternal {
			if in := b.internal[prefix]; in.name != "" && owner != nil && in.name != owner.name {
				title = owner.name + " › " + in.name
			}
		}
		if !b.isConfig && owner != nil && owner.guid == b.top {
			title, kind = "", "Модуль объекта"
		}
		sec := Section{ID: "elem:" + e.Name, Group: group, Kind: "module"}
		switch {
		case title == "":
			sec.Title = kind
		case kind == "":
			sec.Title = title
		default:
			sec.Title = title + ": " + kind
		}
		if !b.isConfig && sec.Title == "Модуль объекта" {
			sec.ID = "object"
		}
		if text := e.child.find("text"); text != nil {
			sec.Editable = true
			b.file.addTextElem(sec, text)
		} else {
			sec.Note = "Модуль сохранён без исходного текста — в файле только скомпилированный код, прочитать его нельзя."
			doc.Sections = append(doc.Sections, sec)
		}
		return
	}

	if o == nil || viaInternal || n != 0 {
		return
	}
	group, title := b.group(o), b.title(o)
	id := "form:" + o.guid
	switch {
	case e.child != nil && e.child.find("module") != nil:
		// Обычная форма: контейнер с элементами form и module.
		b.file.addTextElem(Section{ID: id, Group: group, Title: title, Kind: "module", Editable: true}, e.child.find("module"))
	case o.isTemplate || strings.HasPrefix(string(e.payload), "MOXCEL"):
		b.template(o, e, group, title)
	case o.isForm || (!b.isConfig && strings.HasPrefix(strings.TrimPrefix(string(e.payload), bom), "{")):
		b.file.addManagedForm(id, group, title, e)
	}
}

func (b *v8Builder) template(o *v8Object, e *v8Elem, group, title string) {
	sec := Section{ID: "tpl:" + o.guid, Group: group, Title: title, Kind: "text"}
	payload := string(e.payload)
	switch {
	case strings.HasPrefix(payload, "MOXCEL") || o.tplType == tplSpreadsheet:
		sec.Note = "Табличный документ. Его оформление открывается только в 1С; ниже — текст ячеек."
		sec.Text = moxcelText(e.payload)
	case o.tplType == tplPicture:
		return // картинка — показывать нечего
	default:
		// Текстовый макет, схема компоновки данных или XML. У схем перед XML
		// бывает служебный двоичный заголовок — его сохраняем как есть.
		start := 0
		if i := strings.Index(payload[:min(len(payload), 256)], "<?xml"); i >= 0 {
			start = i
			sec.Note = "Схема компоновки данных или XML-макет."
		} else if strings.HasPrefix(payload, bom) {
			start = len(bom)
		}
		text := payload[start:]
		if o.tplType == tplDCS && start == 0 || !isMostlyText([]byte(text[:min(len(text), 4096)])) {
			return
		}
		sec.Text = normalizeEOL(text)
		if looksLikeCode(text) && sec.Note == "" {
			sec.Kind = "module" // текстовый макет с кодом 1С — с подсветкой
		}
		sec.Editable = true
		head := e.payload[:start]
		b.file.setters[sec.ID] = func(newText string) {
			e.payload = append(append([]byte{}, head...), windowsEOL(newText)...)
			e.dirty = true
		}
	}
	b.file.Doc.Sections = append(b.file.Doc.Sections, sec)
}

// help — справка внешней обработки: HTML в base64 внутри «<id>.1».
func (b *v8Builder) help() {
	for prefix, in := range b.internal {
		if in.owner != b.top {
			continue
		}
		e := b.root.find(prefix + ".1")
		if e == nil {
			continue
		}
		if m := base64Re.FindStringSubmatch(string(e.payload)); m != nil {
			if raw, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(m[1]), "")); err == nil {
				if text := htmlToText(strings.TrimPrefix(string(raw), bom)); text != "" {
					b.file.Doc.Sections = append(b.file.Doc.Sections, Section{ID: "help", Group: "Справка", Title: "Справка", Kind: "text", Text: text})
					return
				}
			}
		}
	}
}

// sort — группы в порядке Конфигуратора, внутри — по алфавиту, модуль объекта первым.
func (b *v8Builder) sort() {
	order := externalGroupOrder
	if b.isConfig {
		order = configGroupOrder
	}
	rank := func(g string) int {
		for i, x := range order {
			if x == g {
				return i
			}
		}
		return len(order)
	}
	secs := b.file.Doc.Sections
	sort.SliceStable(secs, func(i, j int) bool {
		gi, gj := rank(secs[i].Group), rank(secs[j].Group)
		if gi != gj {
			return gi < gj
		}
		if secs[i].Group != secs[j].Group {
			return secs[i].Group < secs[j].Group
		}
		if (secs[i].ID == "object") != (secs[j].ID == "object") {
			return secs[i].ID == "object"
		}
		return secs[i].Title < secs[j].Title
	})
}
