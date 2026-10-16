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
//
// Для списка разделов читаются описания объектов и только начала частей — этого
// хватает, чтобы понять, модуль это, форма или макет. Сам текст раздела читает
// загрузчик, когда раздел открыли.

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
	formDescRe     = regexp.MustCompile(`^\{1,\n(?:\{1,\n)?\{0,\n\{1[3-9],`) // версия описания формы: 13, 14 (8.3.2x)…
	templateDescRe = regexp.MustCompile(`^\{1,\n\{2,(\d+),`)
	classGroupRe   = regexp.MustCompile(`\{([0-9a-f-]{36}),(\d+)((?:,\s*[0-9a-f-]{36})+)\}`)
)

// Типы макетов (второе число в описании макета).
const (
	tplSpreadsheet = 0
	tplPicture     = 1
	tplDCS         = 6
)

// headSize — сколько байт с начала части нужно, чтобы понять, что в ней.
const headSize = 8 << 10

type v8Object struct {
	guid, name, synonym string
	class               string // группа в дереве («Справочники» …)
	isForm, isTemplate  bool
	tplType             int
	owner               string   // GUID владельца формы или макета
	refs                []string // объекты, упомянутые в описании (кандидаты в формы и макеты)
}

type internalName struct{ name, owner string }

func normalizedDesc(e *v8Elem) string {
	return strings.ReplaceAll(strings.TrimPrefix(string(e.payload), bom), "\r\n", "\n")
}

func isGUIDName(name string) bool { return len(name) == 36 && isGUIDAt(name, 0) }

func openV8(path string, file *File, root *container) error {
	file.v8 = root
	doc := &file.Doc
	doc.Format = "1С:Предприятие 8"
	doc.CanSave = true

	// Префиксы частей «<id>.N»: внутренние id запоминаем только для них — у большой
	// конфигурации имён реквизитов сотни тысяч, а частей у них нет.
	guids := map[string]string{} // GUID элемента → та же строка (общая копия)
	prefixes := map[string]bool{}
	for _, e := range root.elems {
		if isGUIDName(e.Name) {
			guids[e.Name] = e.Name
		} else if dot := strings.LastIndex(e.Name, "."); dot == 36 {
			prefixes[e.Name[:36]] = true
		}
	}

	// Объекты: имя, синоним, вид (форма, макет) по описанию. Описания разбираются
	// параллельно, каждое читается и сразу отпускается — в памяти остаются только
	// имена и ссылки. Итог собирается в порядке элементов, как при чтении подряд.
	type descResult struct {
		o     *v8Object
		names []objectName // внутренние id, у которых есть части
		isTop bool
	}
	descs := make([]descResult, len(root.elems))
	err := forEach(file.ctx, len(root.elems), func(i int) error {
		e := root.elems[i]
		if guids[e.Name] == "" {
			return nil
		}
		if err := e.load(); err != nil {
			return err
		}
		defer e.unload()
		desc := normalizedDesc(e)
		o := &v8Object{guid: guids[e.Name]}
		r := descResult{o: o, isTop: strings.HasPrefix(desc, "{2,\n{"+e.Name+"}")}
		names := objectNames(desc)
		for _, n := range names {
			if n.id == e.Name && o.name == "" {
				o.name, o.synonym = n.name, n.synonym
			}
			if prefixes[n.id] {
				r.names = append(r.names, n)
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
		if !o.isForm && !o.isTemplate {
			seen := map[string]bool{}
			eachGUID(desc, func(g string) {
				if same, ok := guids[g]; ok && g != e.Name && !seen[g] {
					seen[g] = true
					o.refs = append(o.refs, same)
				}
			})
		}
		descs[i] = r
		return nil
	})
	if err != nil {
		return err
	}
	objs := make(map[string]*v8Object, len(guids))
	internal := map[string]internalName{} // внутренний id → имя и объект-владелец
	topGUID, isConfig := "", false
	for _, r := range descs {
		if r.o == nil {
			continue
		}
		for _, n := range r.names {
			if _, seen := internal[n.id]; !seen {
				internal[n.id] = internalName{n.name, r.o.guid}
			}
		}
		if topGUID == "" && r.isTop {
			topGUID, isConfig = r.o.guid, true
		}
		objs[r.o.guid] = r.o
	}
	descs = nil

	// Корень: конфигурация («{2,{свой GUID},…») или главный объект обработки (из root).
	if topGUID == "" {
		if e := root.find("root"); e != nil {
			if err := e.load(); err != nil {
				return err
			}
			topGUID = firstGUID(string(e.payload))
			e.unload()
		}
	}
	top := objs[topGUID]
	if top == nil {
		return errors.New("не найдено описание обработки или конфигурации")
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
		e := root.find(topGUID)
		if err := e.load(); err != nil {
			return err
		}
		for _, m := range classGroupRe.FindAllStringSubmatch(normalizedDesc(e), -1) {
			label, ok := classLabels[m[1]]
			if !ok {
				label = "Прочие объекты"
			}
			eachGUID(m[3], func(g string) {
				if o := objs[g]; o != nil && o.class == "" {
					o.class = label
				}
			})
		}
		e.unload()
	}

	// Владельцы форм и макетов: объект, в описании которого они упомянуты.
	for _, e := range root.elems {
		o := objs[e.Name]
		if o == nil || o.isForm || o.isTemplate || e.Name == topGUID {
			continue
		}
		for _, g := range o.refs {
			if c := objs[g]; c != nil && (c.isForm || c.isTemplate) && c.owner == "" {
				c.owner = e.Name
			}
		}
	}
	for _, o := range objs {
		o.refs = nil
	}

	// Части «<id>.N»: что в каждой — модуль, форма, макет — по началу данных.
	// Тоже параллельно; сами разделы собираются по порядку.
	infos := make([]partInfo, len(root.elems))
	err = forEach(file.ctx, len(root.elems), func(i int) error {
		e := root.elems[i]
		if strings.LastIndex(e.Name, ".") != 36 {
			return nil
		}
		info, err := inspectPart(e)
		infos[i] = info
		return err
	})
	if err != nil {
		return err
	}
	b := &v8Builder{file: file, root: root, objs: objs, internal: internal, top: topGUID, isConfig: isConfig}
	for i, e := range root.elems {
		b.element(e, infos[i])
	}
	if !isConfig {
		if err := b.help(); err != nil {
			return err
		}
	}
	b.sort()

	if len(doc.Sections) == 0 {
		return errors.New("в файле не найдено ни модулей, ни форм")
	}
	return nil
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

// Модули самой конфигурации — в порядке важности, главный первым.
var configModules = []string{"Модуль управляемого приложения", "Модуль сеанса", "Модуль обычного приложения", "Модуль внешнего соединения"}

func configModuleRank(title string) int {
	for i, t := range configModules {
		if t == title {
			return i
		}
	}
	return len(configModules)
}

// moduleName — название модуля по номеру части и классу владельца.
func moduleName(class string, n int, viaInternal bool) string {
	switch {
	case class == "Конфигурация":
		// Части самой конфигурации (проверено по содержимому модулей).
		switch n {
		case 0:
			return "Модуль обычного приложения"
		case 5:
			return "Модуль внешнего соединения"
		case 6:
			return "Модуль управляемого приложения"
		case 7:
			return "Модуль сеанса"
		}
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

func contains(names []string, name string) bool {
	for _, n := range names {
		if n == name {
			return true
		}
	}
	return false
}

// partInfo — что лежит в части «<id>.N» (по её началу).
type partInfo struct {
	part       bool     // это часть объекта
	names      []string // элементы вложенного контейнера (nil — не контейнер)
	moxcel     bool     // табличный документ
	brace      bool     // скобочный текст (управляемая форма)
	xml        bool     // XML после служебного заголовка (схема компоновки)
	mostlyText bool
	code       bool // текст похож на код 1С
}

func inspectPart(e *v8Elem) (partInfo, error) {
	names, head, err := e.childNames()
	if err != nil {
		return partInfo{}, err
	}
	info := partInfo{part: true, names: names}
	if names != nil {
		return info, nil
	}
	payload := string(head)
	info.moxcel = strings.HasPrefix(payload, "MOXCEL")
	info.brace = strings.HasPrefix(strings.TrimPrefix(payload, bom), "{")
	text := payload[textStart(payload):]
	info.xml = strings.HasPrefix(text, "<?xml")
	info.mostlyText = isMostlyText([]byte(text[:min(len(text), 4096)]))
	info.code = looksLikeCode(text)
	return info, nil
}

func (b *v8Builder) element(e *v8Elem, info partInfo) {
	if !info.part {
		return
	}
	prefix := e.Name[:36]
	n, err := strconv.Atoi(e.Name[37:])
	if err != nil {
		return
	}
	o, viaInternal := b.ownerOf(prefix)
	names := info.names

	// Модуль: вложенный контейнер с text (или только image — без исходного текста).
	if (contains(names, "text") || contains(names, "image")) && !contains(names, "module") {
		title, group := b.title(o), b.group(o)
		kind := moduleName(group, n, viaInternal)
		if viaInternal && o != nil {
			if in := b.internal[prefix]; in.name != "" && in.name != o.name {
				title = o.name + " › " + in.name
			}
		}
		switch {
		case !b.isConfig && o != nil && o.guid == b.top:
			title, kind = "", "Модуль объекта"
		case b.isConfig && o != nil && o.guid == b.top:
			title = "" // «Модуль сеанса», а не «УправлениеТорговлей: Модуль сеанса»
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
		if contains(names, "text") {
			sec.Editable = true
			b.file.addPart(sec, innerText(e, "text"))
		} else {
			sec.Note = "Модуль сохранён без исходного текста — в файле только скомпилированный код, прочитать его нельзя."
			b.file.addPart(sec, part{})
		}
		return
	}

	if o == nil || viaInternal || n != 0 {
		return
	}
	group, title := b.group(o), b.title(o)
	id := "form:" + o.guid
	switch {
	case contains(names, "module"):
		// Обычная форма: контейнер с элементами form и module.
		b.file.addPart(Section{ID: id, Group: group, Title: title, Kind: "module", Editable: true}, innerText(e, "module"))
	case names != nil:
		// другой вложенный контейнер — показывать нечего
	case o.isTemplate || info.moxcel:
		b.template(o, e, info, group, title)
	case o.isForm || (!b.isConfig && info.brace):
		b.file.addPart(Section{ID: id, Group: group, Title: title, Kind: "module", Editable: true}, managedForm(e))
	}
}

func (b *v8Builder) template(o *v8Object, e *v8Elem, info partInfo, group, title string) {
	sec := Section{ID: "tpl:" + o.guid, Group: group, Title: title, Kind: "text"}
	switch {
	case info.moxcel || o.tplType == tplSpreadsheet:
		sec.Note = "Табличный документ. Его оформление открывается только в 1С; ниже — текст ячеек."
		b.file.addPart(sec, part{load: func() (SectionContent, error) {
			if err := e.load(); err != nil {
				return SectionContent{}, err
			}
			defer e.unload()
			return SectionContent{Text: moxcelText(e.payload)}, nil
		}})
	case o.tplType == tplPicture:
		return // картинка — показывать нечего
	default:
		// Текстовый макет, схема компоновки данных или XML. У схем перед XML
		// бывает служебный двоичный заголовок — его сохраняем как есть.
		if info.xml {
			sec.Note = "Схема компоновки данных или XML-макет."
		}
		if o.tplType == tplDCS && !info.xml || !info.mostlyText {
			return
		}
		if info.code && sec.Note == "" {
			sec.Kind = "module" // текстовый макет с кодом 1С — с подсветкой
		}
		sec.Editable = true
		b.file.addPart(sec, textTemplate(e))
	}
}

// textStart — где в макете начинается текст: после служебного заголовка схемы или BOM.
func textStart(payload string) int {
	if i := strings.Index(payload[:min(len(payload), 256)], "<?xml"); i >= 0 {
		return i
	}
	if strings.HasPrefix(payload, bom) {
		return len(bom)
	}
	return 0
}

// help — справка внешней обработки: HTML в base64 внутри «<id>.1».
func (b *v8Builder) help() error {
	for prefix, in := range b.internal {
		if in.owner != b.top {
			continue
		}
		e := b.root.find(prefix + ".1")
		if e == nil {
			continue
		}
		if err := e.load(); err != nil {
			return err
		}
		text := helpText(e.payload)
		e.unload()
		if text != "" {
			b.file.addPart(Section{ID: "help", Group: "Справка", Title: "Справка", Kind: "text"}, part{
				load: func() (SectionContent, error) { return SectionContent{Text: text}, nil },
			})
			return nil
		}
	}
	return nil
}

func helpText(payload []byte) string {
	m := base64Re.FindSubmatch(payload)
	if m == nil {
		return ""
	}
	raw, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(string(m[1])), ""))
	if err != nil {
		return ""
	}
	return htmlToText(strings.TrimPrefix(string(raw), bom))
}

// sort — группы в порядке Конфигуратора, внутри — по алфавиту, модуль объекта первым.
func (b *v8Builder) sort() {
	order := externalGroupOrder
	if b.isConfig {
		order = configGroupOrder
	}
	rank := make(map[string]int, len(order))
	for i, x := range order {
		rank[x] = i
	}
	rankOf := func(g string) int {
		if r, ok := rank[g]; ok {
			return r
		}
		return len(order)
	}
	f := b.file
	idx := make([]int, len(f.Doc.Sections))
	for i := range idx {
		idx[i] = i
	}
	secs := f.Doc.Sections
	sort.SliceStable(idx, func(i, j int) bool {
		a, c := secs[idx[i]], secs[idx[j]]
		if ga, gc := rankOf(a.Group), rankOf(c.Group); ga != gc {
			return ga < gc
		}
		if a.Group != c.Group {
			return a.Group < c.Group
		}
		if (a.ID == "object") != (c.ID == "object") {
			return a.ID == "object"
		}
		if ra, rc := configModuleRank(a.Title), configModuleRank(c.Title); ra != rc {
			return ra < rc
		}
		return a.Title < c.Title
	})
	sorted := make([]Section, len(secs))
	parts := make([]part, len(secs))
	for i, k := range idx {
		sorted[i], parts[i] = secs[k], f.parts[k]
	}
	f.Doc.Sections, f.parts = sorted, parts
}

// --- Загрузчики разделов -----------------------------------------------------------

// innerText — модуль: элемент inner («text» или «module») во вложенном контейнере.
func innerText(e *v8Elem, inner string) part {
	find := func() (*v8Elem, error) {
		if err := e.load(); err != nil {
			return nil, err
		}
		if e.child == nil || e.child.find(inner) == nil {
			return nil, fmt.Errorf("элемент %s: нет %s", e.Name, inner)
		}
		t := e.child.find(inner)
		return t, t.load()
	}
	return part{
		load: func() (SectionContent, error) {
			defer e.unload()
			t, err := find()
			if err != nil {
				return SectionContent{}, err
			}
			return SectionContent{Text: normalizeEOL(strings.TrimPrefix(string(t.payload), bom)), Editable: true}, nil
		},
		set: func(text string) error {
			t, err := find()
			if err != nil {
				return err
			}
			out := windowsEOL(text)
			if strings.HasPrefix(string(t.payload), bom) {
				out = bom + out
			}
			t.payload, t.dirty = []byte(out), true
			return nil
		},
	}
}

// formModuleSpan — где в описании управляемой формы строка с модулем.
func formModuleSpan(form string) ([2]int, bool) {
	for _, s := range stringsAtDepth(form, 1) {
		if looksLikeCode(form[s[0]:s[1]]) {
			return s, true
		}
	}
	return [2]int{}, false
}

// managedForm — управляемая форма: модуль хранится строкой верхнего уровня
// в скобочном описании формы.
func managedForm(e *v8Elem) part {
	return part{
		load: func() (SectionContent, error) {
			if err := e.load(); err != nil {
				return SectionContent{}, err
			}
			defer e.unload()
			form := string(e.payload)
			span, ok := formModuleSpan(form)
			if !ok {
				return SectionContent{Note: "У этой формы нет модуля."}, nil
			}
			return SectionContent{Text: normalizeEOL(unquote(form[span[0]:span[1]])), Editable: true}, nil
		},
		set: func(text string) error {
			if err := e.load(); err != nil {
				return err
			}
			form := string(e.payload)
			span, ok := formModuleSpan(form)
			if !ok {
				return errors.New("у этой формы нет модуля")
			}
			e.payload, e.dirty = []byte(form[:span[0]]+quote(windowsEOL(text))+form[span[1]:]), true
			return nil
		},
	}
}

// textTemplate — текстовый макет или схема: служебный заголовок перед текстом
// сохраняется как есть.
func textTemplate(e *v8Elem) part {
	return part{
		load: func() (SectionContent, error) {
			if err := e.load(); err != nil {
				return SectionContent{}, err
			}
			defer e.unload()
			payload := string(e.payload)
			return SectionContent{Text: normalizeEOL(payload[textStart(payload):]), Editable: true}, nil
		},
		set: func(text string) error {
			if err := e.load(); err != nil {
				return err
			}
			head := e.payload[:textStart(string(e.payload))]
			e.payload, e.dirty = append(append([]byte{}, head...), windowsEOL(text)...), true
			return nil
		},
	}
}
