// SpireForge UI: 手柄优先的层级推进式页面栈
// 十字键=移动/加减 A=确认 B=返回 X=+10 Y=-10 START=退出
package main

import (
	"encoding/json"
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

func jsonMarshalIndent(v interface{}) (string, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	return string(b), err
}

func jsonUnmarshal(s string, v interface{}) error {
	return json.Unmarshal([]byte(s), v)
}

func colIdle() color.Color   { return curPal().rowIdle }
func colFocus() color.Color  { return curPal().rowFocus }
func colStroke() color.Color { return curPal().stroke }

// ---------- 行 ----------

type row struct {
	w        fyne.CanvasObject
	bg       *canvas.Rectangle
	label    *widget.Label
	value    *widget.RichText
	adjust   func(dir int) // 左=−1 右=+1
	activate func()        // A 键
	refresh  func()        // 从状态同步显示
}

func (r *row) setVal(text string, cn fyne.ThemeColorName) {
	if len(r.value.Segments) == 0 {
		r.value.Segments = []widget.RichTextSegment{&widget.TextSegment{}}
	}
	seg, ok := r.value.Segments[0].(*widget.TextSegment)
	if !ok {
		return
	}
	seg.Text = text
	seg.Style.Inline = true
	seg.Style.ColorName = cn
	r.value.Refresh()
}

// newRowIcon 带左侧图标的行
func newRowIcon(left, right, iconPath string) *row {
	r := newRow(left, right)
	if iconPath != "" {
		img := canvas.NewImageFromFile(iconPath)
		img.FillMode = canvas.ImageFillContain
		icon := container.NewPadded(container.NewGridWrap(fyne.NewSize(42, 42), img))
		r.w = container.NewPadded(container.NewStack(r.bg, container.NewBorder(nil, nil, icon, r.value, r.label)))
	}
	return r
}

func imgBase() string {
	if d := os.Getenv("SPIREFORCE_IMG"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	for _, c := range []string{home + "/Games/SpireForge/img", "img", "thumbs", "/app/share/spireforge/img"} {
		if fi, err := os.Stat(c); err == nil && fi.IsDir() {
			return c
		}
	}
	return ""
}

func normID(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func iconAt(kind, id string) string {
	base := imgBase()
	if base == "" {
		return ""
	}
	p := filepath.Join(base, kind, normID(id)+".png")
	if _, err := os.Stat(p); err == nil {
		return p
	}
	return ""
}

func rarityName(r string) string {
	switch r {
	case "COMMON":
		return L(L("普通", "Common"), "Common")
	case "UNCOMMON":
		return L("罕见", "Uncommon")
	case "RARE":
		return L(L("稀有", "Rare"), "Rare")
	case "BASIC":
		return L("基础", "Basic")
	case "SPECIAL":
		return L(L("特殊", "Special"), "Special")
	case "CURSE":
		return L(L("诅咒", "Curse"), "Curse")
	}
	return ""
}

func newRow(left, right string) *row {
	bg := canvas.NewRectangle(colIdle())
	bg.CornerRadius = 12
	bg.StrokeColor = colStroke()
	bg.StrokeWidth = 1
	l := widget.NewLabel(left)
	l.Wrapping = fyne.TextWrapBreak
	l.TextStyle = fyne.TextStyle{Bold: true}
	v := widget.NewRichText(&widget.TextSegment{Style: widget.RichTextStyle{Inline: true}})
	padded := container.NewPadded(container.NewStack(bg, container.NewBorder(nil, nil, nil, v, l)))
	r := &row{w: padded, bg: bg, label: l, value: v}
	if right != "" {
		r.setVal(right, theme.ColorNameForeground)
	}
	return r
}

func textRow(left, right string, activate func()) *row {
	r := newRow(left, right)
	r.activate = activate
	return r
}

func stepperRow(label string, get func() int, set func(int), min, max, step int) *row {
	r := newRow(label, "")
	r.adjust = func(dir int) {
		v := get() + dir*step
		if v < min {
			v = min
		}
		if v > max {
			v = max
		}
		set(v)
	}
	r.refresh = func() { r.setVal(strconv.Itoa(get()), theme.ColorNameForeground) }
	return r
}

func toggleRow(label string, get func() bool, set func(bool)) *row {
	r := newRow(label, "")
	r.activate = func() { set(!get()) }
	r.refresh = func() {
		if get() {
			r.setVal(L("● 开", "● On"), theme.ColorNameSuccess)
		} else {
			r.setVal(L("○ 关", "○ Off"), theme.ColorNameDisabled)
		}
	}
	return r
}

func choiceRow(left, right string, activate func()) *row {
	r := newRow(left, right)
	r.activate = activate
	return r
}

// ---------- 页 ----------

type pageView struct {
	title    string
	hint     string
	rows     []*row
	gridCols int
	custom   func() fyne.CanvasObject
}

type pg struct {
	build func() *pageView
	focus int
}

type ui struct {
	win         fyne.Window
	stack       []*pg
	curView     *pageView
	header      *widget.RichText
	hint        *widget.RichText
	status      *widget.RichText
	svPath      string
	svDoc       map[string]interface{}
	cfg         templateCfg
	extraSel    map[string]bool
	pendingFlow string
	deckPage    int
	addCat      string // job / colorless
	addCardPage int
	relPage     int
	potPage     int
	events      chan string
	quitCh      chan struct{}
}

// ---------- 导航 ----------

func (u *ui) push(b func() *pageView) {
	u.stack = append(u.stack, &pg{build: b})
	u.render()
}

func (u *ui) pop() {
	if len(u.stack) > 1 {
		u.stack = u.stack[:len(u.stack)-1]
		u.render()
	} else {
		u.win.Close()
	}
}

func (u *ui) render() {
	p := u.stack[len(u.stack)-1]
	pv := p.build()
	u.curView = pv
	if pv.gridCols == 0 {
		pv.gridCols = 1
	}
	if p.focus >= len(pv.rows) {
		p.focus = len(pv.rows) - 1
	}
	if p.focus < 0 {
		p.focus = 0
	}
	for i, r := range pv.rows {
		if r.refresh != nil {
			r.refresh()
		}
		if i == p.focus {
			r.bg.FillColor = colFocus()
			r.bg.StrokeColor = curPal().accent
			r.bg.StrokeWidth = 2
		} else {
			r.bg.FillColor = colIdle()
			r.bg.StrokeColor = colStroke()
			r.bg.StrokeWidth = 1
		}
		r.bg.Refresh()
	}
	u.setHeader(u.saveSummary())
	u.setHint(pv.hint)

	var content fyne.CanvasObject
	if pv.custom != nil {
		objs := make([]fyne.CanvasObject, 0, len(pv.rows)+1)
		objs = append(objs, pv.custom())
		for _, r := range pv.rows {
			objs = append(objs, r.w)
		}
		content = container.NewVScroll(container.NewVBox(objs...))
	} else {
		objs := make([]fyne.CanvasObject, 0, len(pv.rows))
		for _, r := range pv.rows {
			objs = append(objs, r.w)
		}
		var list fyne.CanvasObject
		if pv.gridCols > 1 {
			list = container.NewGridWithColumns(pv.gridCols, objs...)
		} else {
			list = container.NewVBox(objs...)
		}
		content = container.NewVScroll(list)
	}

	title := widget.NewLabelWithStyle("◆ "+pv.title, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	title.Importance = widget.HighImportance
	u.win.SetContent(container.NewBorder(
		container.NewVBox(container.NewPadded(title), container.NewPadded(u.header)),
		container.NewVBox(container.NewPadded(u.hint), container.NewPadded(u.status)),
		nil, nil, content,
	))
}

func (u *ui) cur() *pg { return u.stack[len(u.stack)-1] }

func (u *ui) handle(ev string) {
	// 注: Fyne 窗口无焦点查询 API。用完请按 B/START 退出应用,
	// 否则后台状态下手柄按键仍会操作本工具。
	p := u.cur()
	pv := u.curView
	if pv == nil || len(pv.rows) == 0 {
		return
	}
	gc := pv.gridCols
	if gc == 0 {
		gc = 1
	}
	move := func(d int) {
		n := p.focus + d
		if n < 0 {
			n = 0
		}
		if n > len(pv.rows)-1 {
			n = len(pv.rows) - 1
		}
		if n != p.focus {
			p.focus = n
			u.render()
		}
	}
	switch ev {
	case "up":
		move(-gc)
	case "down":
		move(gc)
	case "left":
		if gc > 1 {
			move(-1)
		} else {
			u.adjustCur(-1)
		}
	case "right":
		if gc > 1 {
			move(1)
		} else {
			u.adjustCur(1)
		}
	case "a":
		r := pv.rows[p.focus]
		if r.activate != nil {
			r.activate()
			u.render()
		}
	case "b":
		u.pop()
	case "x":
		u.adjustCur(10)
	case "y":
		u.adjustCur(-10)
	case "start":
		u.win.Close()
	}
}

func (u *ui) adjustCur(dir int) {
	p := u.cur()
	r := u.curView.rows[p.focus]
	if r.adjust != nil {
		r.adjust(dir)
		if r.refresh != nil {
			r.refresh()
		}
	}
}

// ---------- 状态 ----------

func (u *ui) saveChar() string {
	if u.svPath == "" {
		return ""
	}
	return detectChar(u.svPath)
}

func (u *ui) saveSummary() string {
	if u.svDoc == nil {
		return L("存档: 未加载  ·  目录: ", "Save: none  ·  dir: ") + savesDir()
	}
	charZh := L("未知角色", "Unknown character")
	if c, ok := charDB[u.saveChar()]; ok {
		charZh = c.zh
	}
	nC, nR := 0, 0
	if a, ok := u.svDoc["cards"].([]interface{}); ok {
		nC = len(a)
	}
	if a, ok := u.svDoc["relics"].([]interface{}); ok {
		nR = len(a)
	}
	return fmt.Sprintf(L("存档: %s (%s) · 血量 %d/%d · 金币 %d · 卡 %d 张 · 遗物 %d", "Save: %s (%s) · HP %d/%d · gold %d · %d cards · %d relics"),
		filepath.Base(u.svPath), charZh,
		int(num(u.svDoc, "current_health")), int(num(u.svDoc, "max_health")),
		int(num(u.svDoc, "gold")), nC, nR)
}

func (u *ui) loadSave(path string) bool {
	doc, err := decodeSave(path)
	if err != nil {
		u.setStatus(L("✗ 加载失败: ", "✗ Load failed: ") + err.Error())
		return false
	}
	u.svPath, u.svDoc = path, doc
	if ch := detectChar(path); ch != "" {
		u.cfg.char = ch
	}
	u.setStatus(L("已加载: ", "Loaded: ") + filepath.Base(path))
	return true
}

func (u *ui) writeBack(msg string) bool {
	out, err := encodeSave(u.svDoc)
	if err != nil {
		u.setStatus(L("✗ 编码失败: ", "✗ encode failed: ") + err.Error())
		return false
	}
	bak := u.svPath + ".bak-" + time.Now().Format("0102-150405")
	_ = os.WriteFile(bak, mustRead(u.svPath), 0o644)
	if err := os.WriteFile(u.svPath, out, 0o644); err != nil {
		u.setStatus(L("✗ 写入失败: ", "✗ Write failed: ") + err.Error())
		return false
	}
	u.setStatus("✓ " + msg + L(" · 备份 ", " · backup ") + filepath.Base(bak))
	return true
}

// autoLoad 启动时自动扫描存档目录并加载最近修改的存档
func (u *ui) autoLoad() {
	files := listSaves()
	if len(files) == 0 {
		u.setStatus(L("未在 ", "No saves in ") + savesDir() + L(" 找到存档, 请手动选择", " — select manually"))
		return
	}
	if u.loadSave(files[0]) {
		u.setHeader(u.saveSummary())
	}
}

func richText(rt *widget.RichText, text string, cn fyne.ThemeColorName) {
	seg, ok := rt.Segments[0].(*widget.TextSegment)
	if !ok {
		return
	}
	seg.Text = text
	seg.Style.ColorName = cn
	seg.Style.Inline = true
	rt.Refresh()
}

func (u *ui) setHeader(s string) { richText(u.header, s, theme.ColorNameDisabled) }
func (u *ui) setHint(s string)   { richText(u.hint, s, theme.ColorNameDisabled) }

func (u *ui) setStatus(s string) {
	cn := theme.ColorNameForeground
	switch {
	case strings.HasPrefix(s, "✓"):
		cn = theme.ColorNameSuccess
	case strings.HasPrefix(s, "✗"):
		cn = theme.ColorNameError
	}
	richText(u.status, s, cn)
}

// ---------- 页面 ----------

func (u *ui) pageHome() *pageView {
	sel := L("未加载", "none")
	if u.svDoc != nil {
		sel = filepath.Base(u.svPath)
	}
	return &pageView{
		title: "尖塔工坊 SpireForge",
		hint:  L("A=确认 · B=返回 · 十字键=移动/加减 · X=+10 · Y=−10 · START=退出", "A=confirm · B=back · D-pad=move/adjust · X=+10 · Y=−10 · START=quit"),
		rows: []*row{
			choiceRow(L("开局模板 (新开局定制)", "Run template (custom new run)"), "", func() {
				if u.svDoc == nil {
					u.pendingFlow = "template"
					u.push(u.pageSelectSave())
					return
				}
				u.push(u.pageChar)
			}),
			choiceRow(func() string {
				if lang == "en" {
					return "🌐 Language: English (tap to switch)"
				}
				return "🌐 语言: 中文 (点按切换)"
			}(), "", func() {
				toggleLang()
				u.render()
			}),
			toggleRow(func() string {
				if lang == "en" {
					return "🌓 Night mode"
				}
				return "🌓 夜间模式"
			}(), func() bool { return themeMode == "dark" }, func(b bool) {
				themeMode = map[bool]string{true: "dark", false: "light"}[b]
				saveThemePref()
				fyne.CurrentApp().Settings().SetTheme(spireTheme{theme.DefaultTheme()})
				u.render()
			}),
			choiceRow(L("卡组编辑 (添加/删除/升级/降级)", "Deck editor (add/remove/upgrade/downgrade)"), "", func() {
				if u.svDoc == nil {
					u.pendingFlow = "deck"
					u.push(u.pageSelectSave())
					return
				}
				u.push(u.pageDeckEditor)
			}),
			choiceRow(L("遗物与药水管理", "Relics & potions"), "", func() {
				if u.svDoc == nil {
					u.pendingFlow = "relic"
					u.push(u.pageSelectSave())
					return
				}
				u.push(u.pageRelicsEditor)
			}),
			choiceRow(L("快速数值修改", "Quick stat edit"), "", func() {
				if u.svDoc == nil {
					u.pendingFlow = "edit"
					u.push(u.pageSelectSave())
					return
				}
				u.push(u.pageEdit)
			}),
			choiceRow(L("高级 JSON 编辑", "Advanced JSON editor"), "", func() {
				if u.svDoc == nil {
					u.setStatus(L("✗ 请先选择存档", "✗ Select a save first"))
					return
				}
				u.push(u.pageJSON)
			}),
			choiceRow(L("选择存档文件", "Select save file"), sel, func() { u.push(u.pageSelectSave()) }),
			choiceRow(L("🎮 导入到 Steam 库", "🎮 Import into Steam library"), "", func() {
				msg, err := importToSteam()
				if err != nil {
					u.setStatus("✗ " + err.Error())
					return
				}
				u.setStatus("✓ " + msg)
			}),
			choiceRow(L("退出", "Quit"), "", func() { u.win.Close() }),
		},
	}
}

func (u *ui) pageSelectSave() func() *pageView {
	return func() *pageView {
		pv := &pageView{title: L("选择存档文件", "Select save file"), hint: L("A=加载 · B=返回", "A=load · B=back")}
		files := listSaves()
		if len(files) == 0 {
			pv.rows = append(pv.rows, textRow(L("(存档目录里没有找到 .autosave 文件)", "(no .autosave files in the save directory)"), "", nil))
		}
		for _, f := range files {
			f := f
			right := ""
			if c := detectChar(f); c != "" {
				right = charDB[c].zh + " "
			}
			if fi, err := os.Stat(f); err == nil {
				right += fi.ModTime().Format("01-02 15:04")
			}
			pv.rows = append(pv.rows, choiceRow(filepath.Base(f), right, func() {
				if u.loadSave(f) {
					u.pop()
					u.afterLoad()
				}
			}))
		}
		pv.rows = append(pv.rows, choiceRow(L("浏览文件系统选择…", "Browse filesystem…"), "", func() {
			fd := dialog.NewFileOpen(func(rc fyne.URIReadCloser, err error) {
				if err != nil || rc == nil {
					return
				}
				p := rc.URI().Path()
				rc.Close()
				if u.loadSave(p) {
					u.pop()
					u.afterLoad()
				}
			}, u.win)
			fd.Resize(fyne.NewSize(800, 600))
			fd.Show()
		}))
		pv.rows = append(pv.rows, choiceRow(L("刷新列表", "Refresh list"), "", func() { u.render() }))
		return pv
	}
}

func (u *ui) afterLoad() {
	switch u.pendingFlow {
	case "template":
		u.pendingFlow = ""
		u.push(u.pageChar)
	case "edit":
		u.pendingFlow = ""
		u.push(u.pageEdit)
	case "deck":
		u.pendingFlow = ""
		u.push(u.pageDeckEditor)
	case "relic":
		u.pendingFlow = ""
		u.push(u.pageRelicsEditor)
	default:
		u.render()
	}
}

func (u *ui) pageChar() *pageView {
	pv := &pageView{
		title: L("开局模板 · ① 角色与数值", "Run template · ① Character & stats"),
		hint:  L("A=选中角色 · 左右=加减 (金币+200) · B=返回", "A=pick character · Left/Right=adjust (gold+200) · B=back"),
	}
	for _, c := range charOrder {
		c := c
		m := charDB[c]
		mark := "○"
		if u.cfg.char == c {
			mark = "●"
		}
		pv.rows = append(pv.rows, choiceRow(fmt.Sprintf("%s %s (%s)", mark, L(m.zh, m.en), c),
			L("起始遗物:", "Starter relic: ")+relicName(m.relic), func() {
				u.cfg.char = c
				u.cfg.nStrike, u.cfg.nDefend = m.nStrike, m.nDefend
				if c == "DEFECT" && u.cfg.orbSlots <= 0 {
					u.cfg.orbSlots = 3
				}
				u.render()
			}))
	}
	markN, markA3 := "○", "○"
	if u.cfg.startAct3 {
		markA3 = "●"
	} else {
		markN = "●"
	}
	pv.rows = append(pv.rows,
		choiceRow(L("─ 开局局面 ─", "─ Start point ─"), "", nil),
		choiceRow(fmt.Sprintf(L("%s 涅奥开局 (第一幕正常开始)", "%s Neow start (Act 1 normally)"), markN), "", func() {
			u.cfg.startAct3 = false
			u.render()
		}),
		choiceRow(fmt.Sprintf(L("%s 第三幕开局 · 三钥匙局面 (自取钥匙, 击败三幕Boss后进终幕)", "%s Act 3 start · collect the keys yourself, then Act 4"), markA3),
			L("实验性", "experimental"), func() {
				u.cfg.startAct3 = true
				u.render()
			}),
		stepperRow(L("默认能量 (3=不挂遗物)", "Energy (3 = no relic)"), func() int { return u.cfg.energyCount }, func(v int) { u.cfg.energyCount = v }, 3, 6, 1),
		stepperRow(L("血量", "HP"), func() int { return u.cfg.hp }, func(v int) { u.cfg.hp = v }, 1, 999, 10),
		stepperRow(L("初始金币 (每次+200)", "Starting gold (+200/press)"), func() int { return u.cfg.gold }, func(v int) { u.cfg.gold = v }, 0, 9999, 200),
		stepperRow(L("打击数量", "Strikes"), func() int { return u.cfg.nStrike }, func(v int) { u.cfg.nStrike = v }, 0, 20, 1),
		stepperRow(L("防御数量", "Defends"), func() int { return u.cfg.nDefend }, func(v int) { u.cfg.nDefend = v }, 0, 20, 1),
	)
	if u.cfg.char == "DEFECT" {
		pv.rows = append(pv.rows, stepperRow(L("充能球槽位", "Orb slots"), func() int { return u.cfg.orbSlots }, func(v int) { u.cfg.orbSlots = v }, 1, 10, 1))
	}
	pv.rows = append(pv.rows, choiceRow(L("下一步: 升级与流派 →", "Next: upgrades & archetype →"), "", func() { u.push(u.pageDeck) }))
	return pv
}

func (u *ui) pageDeck() *pageView {
	ch := u.cfg.char
	m := charDB[ch]
	names := make([]string, 0, len(curated.ARCH[ch]))
	for name := range curated.ARCH[ch] {
		names = append(names, name)
	}
	sort.Strings(names)

	pv := &pageView{
		title: L("开局模板 · ② 牌组与流派", "Run template · ② Deck & archetype"),
		hint:  L("A=选定并继续 · B=返回", "A=pick & continue · B=back"),
	}

	pv.rows = append(pv.rows, choiceRow(L("─ 构筑方式 ─", "─ Build mode ─"), "", nil))
	markA, markS := "○", "○"
	if u.cfg.useArch {
		markA = "●"
	} else {
		markS = "●"
	}
	pv.rows = append(pv.rows,
		choiceRow(fmt.Sprintf(L("%s 流派构筑 (抛弃原卡组)", "%s Archetype build (discard base deck)"), markA), "", func() {
			u.cfg.useArch = true
			u.render()
		}),
		choiceRow(fmt.Sprintf(L("%s 标准构筑 (打击/防御+起手卡)", "%s Standard build (strikes/defends + starters)"), markS), "", func() {
			u.cfg.useArch = false
			u.render()
		}))

	if u.cfg.useArch {
		var parts []string
		for _, st := range m.starters {
			parts = append(parts, cardName(st))
		}
		if u.cfg.nStrike > 0 {
			parts = append(parts, fmt.Sprintf("%d×%s", u.cfg.nStrike, cardName(m.strike)))
		}
		if u.cfg.nDefend > 0 {
			parts = append(parts, fmt.Sprintf("%d×%s", u.cfg.nDefend, cardName(m.defend)))
		}
		if len(parts) > 0 {
			pv.rows = append(pv.rows, textRow(L("⚠ 将抛弃", "⚠ Will discard"), strings.Join(parts, L("、", ", ")), nil))
		}
		for _, name := range names {
			name := name
			ids := curated.ARCH[ch][name]
			mk := "○"
			if u.cfg.arch == name {
				mk = "●"
			}
			pv.rows = append(pv.rows, choiceRow(fmt.Sprintf("%s %s", mk, name), fmt.Sprintf(L("核心%d张", "%d core cards"), len(ids)), func() {
				u.cfg.arch = name
				u.render()
			}))
		}
		pv.rows = append(pv.rows,
			stepperRow(L("核心卡数量", "Core card count"), func() int { return u.cfg.archCount }, func(v int) { u.cfg.archCount = v }, 1, 10, 1),
			toggleRow(L("核心卡全部升级", "Upgrade all core cards"), func() bool { return u.cfg.upArch }, func(b bool) { u.cfg.upArch = b }),
			toggleRow(L("附带流派遗物 ×2", "Include 2 archetype relics"), func() bool { return u.cfg.archRelics }, func(b bool) { u.cfg.archRelics = b }),
			choiceRow(L("下一步: 遗物 →", "Next: relics →"), "", func() { u.push(u.pageRare) }))
	} else {
		pv.rows = append(pv.rows,
			stepperRow(L("打击数量", "Strikes"), func() int { return u.cfg.nStrike }, func(v int) { u.cfg.nStrike = v }, 0, 20, 1),
			toggleRow(L("打击牌全部升级", "Upgrade all Strikes"), func() bool { return u.cfg.upStrike }, func(b bool) { u.cfg.upStrike = b }),
			stepperRow(L("防御数量", "Defends"), func() int { return u.cfg.nDefend }, func(v int) { u.cfg.nDefend = v }, 0, 20, 1),
			toggleRow(L("防御牌全部升级", "Upgrade all Defends"), func() bool { return u.cfg.upDefend }, func(b bool) { u.cfg.upDefend = b }),
			toggleRow(L("起手技能卡升级(痛击/电弧等)", "Upgrade starter cards (Bash/Zap etc.)"), func() bool { return u.cfg.upStarter }, func(b bool) { u.cfg.upStarter = b }),
			choiceRow(L("下一步: 遗物 →", "Next: relics →"), "", func() { u.push(u.pageRare) }))
	}
	return pv
}

func (u *ui) autoEnergyText() string {
	if u.cfg.energyCount <= 3 {
		return L("无 (默认3能量)", "None (base 3 energy)")
	}
	need := u.cfg.energyCount - 3
	if need > len(curated.ENERGY) {
		need = len(curated.ENERGY)
	}
	var sb strings.Builder
	for _, rid := range curated.ENERGY[:need] {
		sb.WriteString(relicName(rid))
		sb.WriteString(" ")
	}
	return sb.String()
}

func (u *ui) pageRare() *pageView {
	pv := &pageView{
		title:    L("开局模板 · ③ 遗物", "Run template · ③ Relics"),
		hint:     L("A=开/关 · 十字键在两列间移动 · B=返回", "A=toggle · D-pad crosses columns · B=back"),
		gridCols: 2,
	}
	if u.cfg.startAct3 {
		pv.rows = append(pv.rows, textRow(L("🔑 钥匙", "🔑 Keys"), L("第三幕局面: 关闭直接获得, 第三幕内自行收集", "Act 3 start: keys off — collect them in Act 3"), nil))
	} else {
		pv.rows = append(pv.rows, toggleRow(L("🔑 三把钥匙 (集齐可进终幕·心脏)", "🔑 Three keys (unlock Act 4 · the Heart)"), func() bool { return u.cfg.threeKeys }, func(b bool) { u.cfg.threeKeys = b }))
	}
	pv.rows = append(pv.rows, textRow(L("能量遗物 (自动)", "Energy relics (auto)"), u.autoEnergyText(), nil))
	pv.rows = append(pv.rows, toggleRow(L("默认4件: 手里剑/钢笔尖/干枯之手/钨棒", "Default 4: Shuriken/Pen Nib/Mummified Hand/Tungsten Rod"), func() bool { return u.cfg.rareDefault }, func(b bool) { u.cfg.rareDefault = b }))
	for _, rid := range curated.RARE_MORE {
		rid := rid
		pv.rows = append(pv.rows, toggleRow(relicName(rid),
			func() bool { return u.extraSel[rid] },
			func(b bool) { u.extraSel[rid] = b }))
	}
	pv.rows = append(pv.rows, choiceRow(L("下一步: 确认生成 →", "Next: confirm →"), "", func() { u.push(u.pageConfirm) }))
	return pv
}

func (u *ui) templateSummary() string {
	m := charDB[u.cfg.char]
	var sb strings.Builder
	fmt.Fprintf(&sb, L("%s · %d打击 + %d防御", "%s · %d Strikes + %d Defends"), L(m.zh, m.en), u.cfg.nStrike, u.cfg.nDefend)
	up := 0
	if u.cfg.upStrike {
		up++
	}
	if u.cfg.upDefend {
		up++
	}
	if u.cfg.upStarter {
		up++
	}
	if up > 0 {
		fmt.Fprintf(&sb, L("(%d类升级)", " (%d upgraded)"), up)
	}
	if u.cfg.arch != "" && u.cfg.arch != "无" {
		sb.WriteString(L(" · 流派[", " · archetype [") + u.cfg.arch + "]")
		if u.cfg.upArch {
			sb.WriteString(L("升级", " upgraded"))
		}
	}
	fmt.Fprintf(&sb, L(" · %d能量 · 血量%d · 金币%d · 手牌%d", " · %d energy · HP %d · gold %d · hand %d"), u.cfg.energyCount, u.cfg.hp, u.cfg.gold, u.cfg.hand)
	return sb.String()
}

func (u *ui) pageConfirm() *pageView {
	pv := &pageView{title: L("开局模板 · ④ 确认", "Run template · ④ Confirm"), hint: L("A=执行 · B=返回修改", "A=execute · B=back to edit")}
	target := filepath.Join(savesDir(), u.cfg.char+".autosave")
	note := L("已存在, 直接覆盖写入", "exists — will be overwritten")
	if _, err := os.Stat(target); err != nil {
		note = L("不存在, 将自动从同角色最近存档复制底版", "missing — a base will be copied from the latest same-character save")
	}
	pv.rows = append(pv.rows,
		textRow(L("配置", "Config"), u.templateSummary(), nil),
		textRow(L("写入目标 (固定文件名)", "Target (fixed filename)"), target, nil),
		textRow(L("目标状态", "Target status"), note, nil))
	if u.svDoc != nil {
		if got := detectChar(u.svPath); got != "" && got != u.cfg.char {
			pv.rows = append(pv.rows, textRow(L("⚠ 当前加载的是其他角色存档", "⚠ Loaded save belongs to another character"), L("将自动切换到 ", "Will switch to ")+filepath.Base(target), nil))
		}
	}
	pv.rows = append(pv.rows,
		choiceRow(L("✅ 生成并写入存档", "✅ Generate & write save"), "", func() { u.doApplyTemplate() }),
		choiceRow(L("← 返回修改", "← Back to edit"), "", func() { u.pop() }))
	return pv
}

func (u *ui) doApplyTemplate() {
	if u.cfg.char == "" {
		u.setStatus(L("✗ 请先在第①步选择角色", "✗ Pick a character in step ① first"))
		return
	}
	// 固定文件名: 模板永远写入 <角色>.autosave; 没有存档也能从零生成
	target := filepath.Join(savesDir(), u.cfg.char+".autosave")
	if _, err := os.Stat(target); err != nil {
		for _, f := range listSaves() {
			if detectChar(f) == u.cfg.char {
				if b, err := os.ReadFile(f); err == nil {
					if err := os.WriteFile(target, b, 0o644); err == nil {
						u.setStatus(L("已复制底版 → ", "Copied base → ") + filepath.Base(target))
					}
				}
				break
			}
		}
	}
	if u.svPath != target {
		if !u.loadSave(target) {
			doc, err := loadSkeleton()
			if err != nil {
				u.setStatus(L("✗ 生成全新存档失败: ", "✗ Failed to create fresh save: ") + err.Error())
				return
			}
			u.svPath, u.svDoc = target, doc
			if u.cfg.startAct3 {
				u.setStatus(L("从零生成第三幕开局存档 (三钥匙局面)", "Generated an Act 3 start (collect the keys yourself)"))
			} else {
				u.setStatus(L("从零生成全新开局 (从涅奥开始)", "Generated a fresh run from scratch (starts at Neow)"))
			}
		}
	}
	msg, err := applyTemplate(u.svDoc, u.cfg)
	if err != nil {
		u.setStatus("✗ " + err.Error())
		return
	}
	if u.writeBack(msg) {
		u.stack = u.stack[:1]
		u.render()
	}
}

func (u *ui) pageEdit() *pageView {
	doc := u.svDoc
	get := func(k string) func() int {
		return func() int { return int(num(doc, k)) }
	}
	set := func(k string) func(int) {
		return func(v int) { doc[k] = float64(v) }
	}
	return &pageView{
		title: L("快速数值修改", "Quick stat edit"),
		hint:  L("左右=±1 X=+10 Y=−10 · A 无效 · B=返回", "Left/Right=±1 X=+10 Y=−10 · B=back"),
		rows: []*row{
			stepperRow(L("当前血量", "Current HP"), get("current_health"), set("current_health"), 0, 9999, 10),
			stepperRow(L("最大血量", "Max HP"), get("max_health"), set("max_health"), 1, 9999, 10),
			stepperRow(L("金币", "Gold"), get("gold"), set("gold"), 0, 999999, 200),
			stepperRow(L("手牌上限", "Hand size"), get("hand_size"), set("hand_size"), 0, 20, 1),
			choiceRow(L("💾 写回存档", "💾 Write back to save"), "", func() { u.writeBack(L("数值已保存", "Stats saved")) }),
			choiceRow(L("↻ 重新读取文件", "↻ Reload file"), "", func() {
				if u.loadSave(u.svPath) {
					u.render()
				}
			}),
		},
	}
}

func (u *ui) pageJSON() *pageView {
	entry := widget.NewMultiLineEntry()
	entry.Wrapping = fyne.TextWrapBreak
	entry.SetMinRowsVisible(16)
	if b, err := jsonMarshalIndent(u.svDoc); err == nil {
		entry.SetText(b)
	}
	return &pageView{
		title:  L("高级 JSON 编辑 (键盘)", "Advanced JSON (keyboard)"),
		hint:   L("改完先「应用到内存」再「写回」· B=返回", "Apply to memory, then Write back · B=back"),
		custom: func() fyne.CanvasObject { return entry },
		rows: []*row{
			choiceRow(L("应用到内存", "Apply to memory"), "", func() {
				var doc map[string]interface{}
				if err := jsonUnmarshal(entry.Text, &doc); err != nil {
					u.setStatus(L("✗ JSON 无效: ", "✗ Invalid JSON: ") + err.Error())
					return
				}
				u.svDoc = doc
				u.setStatus(L("✓ JSON 已应用 (未写盘)", "✓ JSON applied (not written)"))
			}),
			choiceRow(L("💾 写回存档", "💾 Write back to save"), "", func() { u.writeBack(L("JSON 已保存", "JSON saved")) }),
			choiceRow(L("← 返回", "← Back"), "", func() { u.pop() }),
		},
	}
}

// ---------- 卡组 / 遗物 / 药水编辑器 ----------

func (u *ui) deckCards() []interface{} {
	if u.svDoc == nil {
		return nil
	}
	arr, _ := u.svDoc["cards"].([]interface{})
	return arr
}

func cardLabel(v interface{}) string {
	m, ok := v.(map[string]interface{})
	if !ok {
		return L("(异常卡)", "(bad card)")
	}
	id, _ := m["id"].(string)
	up := int(num(m, "upgrades"))
	s := cardName(id)
	if up > 0 {
		s += fmt.Sprintf(" +%d", up)
	}
	return s
}

func (u *ui) syncRelicCounters() {
	if arr, ok := u.svDoc["relics"].([]interface{}); ok {
		c := make([]interface{}, len(arr))
		for i := range c {
			c[i] = float64(-1)
		}
		u.svDoc["relic_counters"] = c
	}
}

// pageDeckEditor 卡组编辑(分页)
func (u *ui) pageDeckEditor() *pageView {
	pv := &pageView{title: L("卡组编辑", "Deck editor"), hint: L("A=进入卡牌操作 · 左右=翻页 · B=返回", "A=card actions · Left/Right=pages · B=back")}
	deck := u.deckCards()
	perPage := 8
	pages := (len(deck) + perPage - 1) / perPage
	if u.deckPage >= pages {
		u.deckPage = pages - 1
	}
	if u.deckPage < 0 {
		u.deckPage = 0
	}
	start := u.deckPage * perPage
	end := start + perPage
	if end > len(deck) {
		end = len(deck)
	}
	for i := start; i < end; i++ {
		i := i
		m, _ := deck[i].(map[string]interface{})
		cid, _ := m["id"].(string)
		r := newRowIcon(fmt.Sprintf("%d. %s", i+1, cardLabel(deck[i])), "", iconAt("cards", cid))
		r.activate = func() { u.push(u.pageCardEdit(i)) }
		pv.rows = append(pv.rows, r)
	}
	if pages > 1 {
		pv.rows = append(pv.rows,
			choiceRow(L("◀ 上一页", "◀ Prev page"), fmt.Sprintf(L("第 %d/%d 页", "Page %d/%d"), u.deckPage+1, pages), func() {
				if u.deckPage > 0 {
					u.deckPage--
					u.render()
				}
			}),
			choiceRow(L("▶ 下一页", "▶ Next page"), "", func() {
				if u.deckPage < pages-1 {
					u.deckPage++
					u.render()
				}
			}))
	}
	pv.rows = append(pv.rows,
		choiceRow(L("⬆ 升级所有卡牌 (+1)", "⬆ Upgrade ALL cards (+1)"), "", func() {
			for _, v := range u.deckCards() {
				if m, ok := v.(map[string]interface{}); ok {
					if n := num(m, "upgrades"); n < 9 {
						m["upgrades"] = n + 1
					}
				}
			}
			u.setStatus(L("已升级全部卡牌 (未写盘)", "All cards upgraded (not written)"))
			u.render()
		}),
		choiceRow(L("➕ 添加卡牌", "➕ Add cards"), "", func() {
			u.addCardPage = 0
			u.push(u.pageAddCard)
		}),
		choiceRow(L("💾 写回存档", "💾 Write back to save"), "", func() { u.writeBack(L("卡组已保存", "Deck saved")) }),
		choiceRow(L("↻ 重新读取", "↻ Reload"), "", func() {
			if u.loadSave(u.svPath) {
				u.render()
			}
		}),
		choiceRow(L("← 返回", "← Back"), "", func() { u.pop() }),
	)
	return pv
}

// pageCardEdit 单卡操作
func (u *ui) pageCardEdit(idx int) func() *pageView {
	return func() *pageView {
		deck := u.deckCards()
		pv := &pageView{title: fmt.Sprintf(L("卡牌操作 · #%d", "Card actions · #%d"), idx+1), hint: L("A=执行 · B=返回卡组", "A=execute · B=back to deck")}
		if idx >= len(deck) {
			pv.rows = append(pv.rows, textRow(L("(卡已不存在)", "(card no longer exists)"), "", nil))
			return pv
		}
		m := deck[idx].(map[string]interface{})
		pv.rows = append(pv.rows, textRow(L("当前", "Current"), cardLabel(m), nil))
		pv.rows = append(pv.rows, choiceRow(L("⬆ 升级 (+1)", "⬆ Upgrade (+1)"), "", func() {
			if n := num(m, "upgrades"); n < 9 {
				m["upgrades"] = n + 1
			}
			u.render()
		}))
		pv.rows = append(pv.rows, choiceRow(L("⬇ 降级 (−1)", "⬇ Downgrade (−1)"), "", func() {
			if n := num(m, "upgrades"); n > 0 {
				m["upgrades"] = n - 1
			}
			u.render()
		}))
		pv.rows = append(pv.rows, choiceRow(L("🗑 删除此卡", "🗑 Remove this card"), "", func() {
			u.svDoc["cards"] = append(deck[:idx], deck[idx+1:]...)
			u.setStatus(L("已删除 (未写盘)", "Removed (not written)"))
			u.pop()
		}))
		pv.rows = append(pv.rows, choiceRow(L("← 返回", "← Back"), "", func() { u.pop() }))
		return pv
	}
}

var charColor = map[string]string{"IRONCLAD": "RED", "THE_SILENT": "GREEN", "DEFECT": "BLUE", "WATCHER": "PURPLE"}

// pageAddCard 添加卡牌: 类别 -> 分页选择
func (u *ui) pageAddCard() *pageView {
	pv := &pageView{title: L("添加卡牌", "Add cards"), hint: L("A=添加(可连续加) · B=返回卡组", "A=add (repeatable) · B=back to deck")}
	ch := u.cfg.char
	if cc, ok := charColor[u.saveChar()]; ok {
		ch = u.saveChar()
		_ = cc
	}
	jobColor := charColor[ch]
	cur := u.addCat
	if cur == "" {
		cur = "job"
	}
	markJob, markCol := "", ""
	if cur == "job" {
		markJob = "●"
	} else {
		markCol = "●"
	}
	pv.rows = append(pv.rows,
		choiceRow(fmt.Sprintf(L("%s 本职业 (%s)", "%s class (%s)"), markJob, L(charDB[ch].zh, charDB[ch].en)), "", func() {
			u.addCat = "job"
			u.addCardPage = 0
			u.render()
		}),
		choiceRow(fmt.Sprintf(L("%s 无色卡", "%s colorless"), markCol), "", func() {
			u.addCat = "colorless"
			u.addCardPage = 0
			u.render()
		}))

	var pool []string
	for cid, c := range cards {
		if cur == "job" {
			if c.Color == jobColor {
				pool = append(pool, cid)
			}
		} else if c.Color == "COLORLESS" {
			pool = append(pool, cid)
		}
	}
	sort.Strings(pool)
	perPage := 12
	pages := (len(pool) + perPage - 1) / perPage
	if u.addCardPage >= pages {
		u.addCardPage = pages - 1
	}
	if u.addCardPage < 0 {
		u.addCardPage = 0
	}
	start := u.addCardPage * perPage
	end := start + perPage
	if end > len(pool) {
		end = len(pool)
	}
	for i := start; i < end; i++ {
		cid := pool[i]
		r := newRowIcon(cardName(cid), rarityName(cards[cid].Rarity), iconAt("cards", cid))
		r.activate = func() {
			arr, _ := u.svDoc["cards"].([]interface{})
			u.svDoc["cards"] = append(arr, map[string]interface{}{"id": cid, "upgrades": float64(0)})
			u.setStatus(L("已添加 ", "Added ") + cardName(cid) + L(" (未写盘)", " (not written)"))
			u.render()
		}
		pv.rows = append(pv.rows, r)
	}
	if pages > 1 {
		pv.rows = append(pv.rows,
			choiceRow(L("◀ 上一页", "◀ Prev page"), fmt.Sprintf(L("第 %d/%d 页 · 共%d张", "Page %d/%d · %d cards"), u.addCardPage+1, pages, len(pool)), func() {
				if u.addCardPage > 0 {
					u.addCardPage--
					u.render()
				}
			}),
			choiceRow(L("▶ 下一页", "▶ Next page"), "", func() {
				if u.addCardPage < pages-1 {
					u.addCardPage++
					u.render()
				}
			}))
	}
	pv.rows = append(pv.rows, choiceRow(L("← 返回卡组", "← Back to deck"), "", func() { u.pop() }))
	return pv
}

// pageRelicsEditor 遗物管理
func (u *ui) pageRelicsEditor() *pageView {
	pv := &pageView{title: L("遗物管理", "Relic manager"), hint: L("A=遗物操作 · B=返回", "A=relic actions · B=back")}
	var rel []interface{}
	if u.svDoc != nil {
		rel, _ = u.svDoc["relics"].([]interface{})
	}
	for i, r := range rel {
		i := i
		rid, _ := r.(string)
		row := newRowIcon(relicName(rid), "", iconAt("relics", rid))
		row.activate = func() { u.push(u.pageRelicEdit(i)) }
		pv.rows = append(pv.rows, row)
	}
	pv.rows = append(pv.rows,
		choiceRow(L("➕ 添加遗物", "➕ Add relic"), "", func() {
			u.relPage = 0
			u.push(u.pageAddRelic)
		}),
		choiceRow(L("🧪 药水管理", "🧪 Potion manager"), "", func() {
			u.potPage = 0
			u.push(u.pagePotions)
		}),
		choiceRow(L("💾 写回存档", "💾 Write back to save"), "", func() { u.writeBack(L("遗物已保存", "Relics saved")) }),
		choiceRow(L("← 返回", "← Back"), "", func() { u.pop() }),
	)
	return pv
}

func (u *ui) pageRelicEdit(idx int) func() *pageView {
	return func() *pageView {
		pv := &pageView{title: L("遗物操作", "Relic actions"), hint: L("A=执行 · B=返回", "A=execute · B=back")}
		rel, _ := u.svDoc["relics"].([]interface{})
		if idx >= len(rel) {
			pv.rows = append(pv.rows, textRow(L("(遗物已不存在)", "(relic no longer exists)"), "", nil))
			return pv
		}
		rid, _ := rel[idx].(string)
		pv.rows = append(pv.rows, textRow(L("当前", "Current"), relicName(rid), nil))
		if idx == 0 {
			pv.rows = append(pv.rows, textRow(L("(起始遗物, 不建议删除)", "(starter relic — do not remove)"), "", nil))
		} else {
			pv.rows = append(pv.rows, choiceRow(L("🗑 删除此遗物", "🗑 Remove this relic"), "", func() {
				u.svDoc["relics"] = append(rel[:idx], rel[idx+1:]...)
				u.syncRelicCounters()
				u.setStatus(L("已删除 ", "Removed ") + relicName(rid) + L(" (未写盘)", " (not written)"))
				u.pop()
			}))
		}
		pv.rows = append(pv.rows, choiceRow(L("← 返回", "← Back"), "", func() { u.pop() }))
		return pv
	}
}

func (u *ui) pageAddRelic() *pageView {
	pv := &pageView{title: L("添加遗物", "Add relic"), hint: L("A=添加(可连续加) · B=返回", "A=add (repeatable) · B=back")}
	pool := make([]string, 0, len(relics))
	for rid := range relics {
		pool = append(pool, rid)
	}
	sort.Strings(pool)
	perPage := 12
	pages := (len(pool) + perPage - 1) / perPage
	if u.relPage >= pages {
		u.relPage = pages - 1
	}
	if u.relPage < 0 {
		u.relPage = 0
	}
	start := u.relPage * perPage
	end := start + perPage
	if end > len(pool) {
		end = len(pool)
	}
	for i := start; i < end; i++ {
		rid := pool[i]
		r := newRowIcon(relicName(rid), "", iconAt("relics", rid))
		r.activate = func() {
			arr, _ := u.svDoc["relics"].([]interface{})
			u.svDoc["relics"] = append(arr, rid)
			u.syncRelicCounters()
			u.setStatus(L("已添加 ", "Added ") + relicName(rid) + L(" (未写盘)", " (not written)"))
			u.render()
		}
		pv.rows = append(pv.rows, r)
	}
	if pages > 1 {
		pv.rows = append(pv.rows,
			choiceRow(L("◀ 上一页", "◀ Prev page"), fmt.Sprintf(L("第 %d/%d 页 · 共%d件", "Page %d/%d · %d relics"), u.relPage+1, pages, len(pool)), func() {
				if u.relPage > 0 {
					u.relPage--
					u.render()
				}
			}),
			choiceRow(L("▶ 下一页", "▶ Next page"), "", func() {
				if u.relPage < pages-1 {
					u.relPage++
					u.render()
				}
			}))
	}
	pv.rows = append(pv.rows, choiceRow(L("← 返回遗物管理", "← Back to relics"), "", func() { u.pop() }))
	return pv
}

// pagePotions 药水管理
func (u *ui) pagePotions() *pageView {
	pv := &pageView{title: L("药水管理", "Potion manager"), hint: L("A=槽位操作 · B=返回", "A=slot actions · B=back")}
	var pots []interface{}
	if u.svDoc != nil {
		pots, _ = u.svDoc["potions"].([]interface{})
	}
	if pots == nil {
		pv.rows = append(pv.rows, textRow(L("(存档没有药水槽字段, 写回时会创建)", "(save has no potion field; created on write)"), "", nil))
	}
	for i, p := range pots {
		i := i
		pid, _ := p.(string)
		right := ""
		if pid != "Potion Slot" {
			right = L("有药水", "filled")
		}
		row := newRowIcon(fmt.Sprintf(L("槽位%d: %s", "Slot %d: %s"), i+1, potionName(pid)), right, iconAt("potions", "bottle"))
		row.activate = func() { u.push(u.pagePotionEdit(i)) }
		pv.rows = append(pv.rows, row)
	}
	pv.rows = append(pv.rows,
		choiceRow(L("💾 写回存档", "💾 Write back to save"), "", func() { u.writeBack(L("药水已保存", "Potions saved")) }),
		choiceRow(L("← 返回", "← Back"), "", func() { u.pop() }),
	)
	return pv
}

func (u *ui) pagePotionEdit(idx int) func() *pageView {
	return func() *pageView {
		pv := &pageView{title: fmt.Sprintf(L("药水槽位 %d", "Potion slot %d"), idx+1), hint: L("A=执行 · B=返回", "A=execute · B=back")}
		pots, _ := u.svDoc["potions"].([]interface{})
		if idx >= len(pots) {
			pv.rows = append(pv.rows, textRow(L("(槽位已不存在)", "(slot no longer exists)"), "", nil))
			return pv
		}
		pid, _ := pots[idx].(string)
		pv.rows = append(pv.rows, textRow(L("当前", "Current"), potionName(pid), nil))
		if pid != "Potion Slot" {
			pv.rows = append(pv.rows, choiceRow(L("🗑 清空 (删除药水)", "🗑 Empty (remove potion)"), "", func() {
				pots[idx] = "Potion Slot"
				u.setStatus(L("已清空 (未写盘)", "Emptied (not written)"))
				u.render()
			}))
		}
		pv.rows = append(pv.rows, choiceRow(L("➕ 更换药水 →", "➕ Change potion →"), "", func() {
			u.potPage = 0
			u.push(u.pagePickPotion(idx))
		}))
		pv.rows = append(pv.rows, choiceRow(L("← 返回", "← Back"), "", func() { u.pop() }))
		return pv
	}
}

func (u *ui) pagePickPotion(slot int) func() *pageView {
	return func() *pageView {
		pv := &pageView{title: L("选择药水", "Pick a potion"), hint: L("A=装入该槽位 · B=返回", "A=fill this slot · B=back")}
		pool := make([]string, 0, len(potions))
		for pid := range potions {
			pool = append(pool, pid)
		}
		sort.Strings(pool)
		perPage := 12
		pages := (len(pool) + perPage - 1) / perPage
		if u.potPage >= pages {
			u.potPage = pages - 1
		}
		if u.potPage < 0 {
			u.potPage = 0
		}
		start := u.potPage * perPage
		end := start + perPage
		if end > len(pool) {
			end = len(pool)
		}
		for i := start; i < end; i++ {
			pid := pool[i]
			r := newRowIcon(potionName(pid), "", iconAt("potions", "bottle"))
			r.activate = func() {
				if pots, ok := u.svDoc["potions"].([]interface{}); ok && slot < len(pots) {
					pots[slot] = pid
					u.setStatus(L("已装入 ", "Loaded ") + potionName(pid) + L(" (未写盘)", " (not written)"))
					u.pop()
				}
			}
			pv.rows = append(pv.rows, r)
		}
		if pages > 1 {
			pv.rows = append(pv.rows,
				choiceRow(L("◀ 上一页", "◀ Prev page"), fmt.Sprintf(L("第 %d/%d 页 · 共%d种", "Page %d/%d · %d potions"), u.potPage+1, pages, len(pool)), func() {
					if u.potPage > 0 {
						u.potPage--
						u.render()
					}
				}),
				choiceRow(L("▶ 下一页", "▶ Next page"), "", func() {
					if u.potPage < pages-1 {
						u.potPage++
						u.render()
					}
				}))
		}
		pv.rows = append(pv.rows, choiceRow(L("← 返回", "← Back"), "", func() { u.pop() }))
		return pv
	}
}

// ---------- main ----------

func main() {
	loadLangPref()
	loadThemePref()
	// 命令行模式: --import-steam 直接注册 Steam 库条目 (便于远程验证)
	if len(os.Args) > 1 && os.Args[1] == "--import-steam" {
		initData()
		msg, err := importToSteam()
		if err != nil {
			fmt.Println("FAIL:", err)
			os.Exit(1)
		}
		fmt.Println("OK:", msg)
		return
	}
	initData()
	a := app.NewWithID("com.spireforge.SaveForge")
	a.Settings().SetTheme(spireTheme{theme.DefaultTheme()})
	w := a.NewWindow("尖塔工坊 SpireForge")
	w.Resize(fyne.NewSize(1024, 740))

	u := &ui{
		win:      w,
		header:   widget.NewRichText(&widget.TextSegment{Style: widget.RichTextStyle{ColorName: theme.ColorNameDisabled, Inline: true}}),
		hint:     widget.NewRichText(&widget.TextSegment{Style: widget.RichTextStyle{ColorName: theme.ColorNameDisabled, Inline: true}}),
		status:   widget.NewRichText(&widget.TextSegment{Style: widget.RichTextStyle{ColorName: theme.ColorNameForeground, Inline: true}}),
		extraSel: map[string]bool{},
		events:   make(chan string, 64),
		quitCh:   make(chan struct{}),
		cfg: templateCfg{
			char: "IRONCLAD", nStrike: 5, nDefend: 4, upStarter: true,
			arch: "无", useArch: false, archCount: 6, archRelics: true, energyCount: 3, orbSlots: 3, threeKeys: true, startAct3: false, rareDefault: true, hp: 120, gold: 200, hand: 10,
		},
	}
	for _, rt := range []*widget.RichText{u.header, u.hint, u.status} {
		rt.Wrapping = fyne.TextWrapBreak
	}

	u.stack = []*pg{{build: u.pageHome}}
	u.autoLoad()
	u.render()

	// 键盘支持 (桌面): 方向键=移动/加减 回车=确认 Esc=返回
	w.Canvas().SetOnTypedKey(func(k *fyne.KeyEvent) {
		if _, editing := u.win.Canvas().Focused().(*widget.Entry); editing {
			return // 文本编辑时不劫持按键
		}
		var ev string
		switch k.Name {
		case fyne.KeyUp:
			ev = "up"
		case fyne.KeyDown:
			ev = "down"
		case fyne.KeyLeft:
			ev = "left"
		case fyne.KeyRight:
			ev = "right"
		case fyne.KeyReturn, fyne.KeyEnter:
			ev = "a"
		case fyne.KeyEscape:
			ev = "b"
		}
		if ev != "" {
			fyne.Do(func() { u.handle(ev) })
		}
	})

	go padWatch(u.events, u.quitCh)
	go func() {
		last, lastT := "", time.Time{}
		for ev := range u.events {
			now := time.Now()
			if ev == last && now.Sub(lastT) < 120*time.Millisecond {
				continue // 物理手柄+Steam虚拟手柄会发双份事件, 去重
			}
			last, lastT = ev, now
			ev := ev
			fyne.Do(func() { u.handle(ev) })
		}
	}()

	w.ShowAndRun()
	close(u.quitCh)
}
