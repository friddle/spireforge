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

var colIdle = &color.NRGBA{R: 27, G: 30, B: 44, A: 255}
var colFocus = &color.NRGBA{R: 46, G: 52, B: 88, A: 255}

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
	for _, c := range []string{home + "/Games/SpireForge/img", "img", "/app/share/spireforge/img"} {
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

var zhRarity = map[string]string{
	"COMMON": "普通", "UNCOMMON": "罕见", "RARE": "稀有",
	"BASIC": "基础", "SPECIAL": "特殊", "CURSE": "诅咒", "": "",
}

func newRow(left, right string) *row {
	bg := canvas.NewRectangle(colIdle)
	bg.CornerRadius = 12
	bg.StrokeWidth = 0
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
			r.setVal("● 开", theme.ColorNameSuccess)
		} else {
			r.setVal("○ 关", theme.ColorNameDisabled)
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
			r.bg.FillColor = colFocus
			r.bg.StrokeColor = colAccent
			r.bg.StrokeWidth = 2
		} else {
			r.bg.FillColor = colIdle
			r.bg.StrokeWidth = 0
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
		return "存档: 未加载  ·  目录: " + savesDir()
	}
	charZh := "未知角色"
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
	return fmt.Sprintf("存档: %s (%s) · 血量 %d/%d · 金币 %d · 卡 %d 张 · 遗物 %d",
		filepath.Base(u.svPath), charZh,
		int(num(u.svDoc, "current_health")), int(num(u.svDoc, "max_health")),
		int(num(u.svDoc, "gold")), nC, nR)
}

func (u *ui) loadSave(path string) bool {
	doc, err := decodeSave(path)
	if err != nil {
		u.setStatus("✗ 加载失败: " + err.Error())
		return false
	}
	u.svPath, u.svDoc = path, doc
	if ch := detectChar(path); ch != "" {
		u.cfg.char = ch
	}
	u.setStatus("已加载: " + filepath.Base(path))
	return true
}

func (u *ui) writeBack(msg string) bool {
	out, err := encodeSave(u.svDoc)
	if err != nil {
		u.setStatus("✗ 编码失败: " + err.Error())
		return false
	}
	bak := u.svPath + ".bak-" + time.Now().Format("0102-150405")
	_ = os.WriteFile(bak, mustRead(u.svPath), 0o644)
	if err := os.WriteFile(u.svPath, out, 0o644); err != nil {
		u.setStatus("✗ 写入失败: " + err.Error())
		return false
	}
	u.setStatus("✓ " + msg + " · 备份 " + filepath.Base(bak))
	return true
}

// autoLoad 启动时自动扫描存档目录并加载最近修改的存档
func (u *ui) autoLoad() {
	files := listSaves()
	if len(files) == 0 {
		u.setStatus("未在 " + savesDir() + " 找到存档, 请手动选择")
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
	sel := "未加载"
	if u.svDoc != nil {
		sel = filepath.Base(u.svPath)
	}
	return &pageView{
		title: "尖塔工坊 SpireForge",
		hint:  "A=确认 · B=返回 · 十字键=移动/加减 · X=+10 · Y=−10 · START=退出",
		rows: []*row{
			choiceRow("选择存档文件", sel, func() { u.push(u.pageSelectSave()) }),
			choiceRow("开局模板 (新开局定制)", "", func() {
				if u.svDoc == nil {
					u.pendingFlow = "template"
					u.push(u.pageSelectSave())
					return
				}
				u.push(u.pageChar)
			}),
			choiceRow("卡组编辑 (添加/删除/升级/降级)", "", func() {
				if u.svDoc == nil {
					u.pendingFlow = "deck"
					u.push(u.pageSelectSave())
					return
				}
				u.push(u.pageDeckEditor)
			}),
			choiceRow("遗物与药水管理", "", func() {
				if u.svDoc == nil {
					u.pendingFlow = "relic"
					u.push(u.pageSelectSave())
					return
				}
				u.push(u.pageRelicsEditor)
			}),
			choiceRow("快速数值修改", "", func() {
				if u.svDoc == nil {
					u.pendingFlow = "edit"
					u.push(u.pageSelectSave())
					return
				}
				u.push(u.pageEdit)
			}),
			choiceRow("高级 JSON 编辑", "", func() {
				if u.svDoc == nil {
					u.setStatus("✗ 请先选择存档")
					return
				}
				u.push(u.pageJSON)
			}),
			choiceRow("🎮 导入到 Steam 库", "", func() {
				msg, err := importToSteam()
				if err != nil {
					u.setStatus("✗ " + err.Error())
					return
				}
				u.setStatus("✓ " + msg)
			}),
			choiceRow("退出", "", func() { u.win.Close() }),
		},
	}
}

func (u *ui) pageSelectSave() func() *pageView {
	return func() *pageView {
		pv := &pageView{title: "选择存档文件", hint: "A=加载 · B=返回"}
		files := listSaves()
		if len(files) == 0 {
			pv.rows = append(pv.rows, textRow("(存档目录里没有找到 .autosave 文件)", "", nil))
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
		pv.rows = append(pv.rows, choiceRow("浏览文件系统选择…", "", func() {
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
		pv.rows = append(pv.rows, choiceRow("刷新列表", "", func() { u.render() }))
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
		title: "开局模板 · ① 角色与数值",
		hint:  "A=选中角色 · 左右=加减 (金币+200) · B=返回",
	}
	for _, c := range charOrder {
		c := c
		m := charDB[c]
		mark := "○"
		if u.cfg.char == c {
			mark = "●"
		}
		pv.rows = append(pv.rows, choiceRow(fmt.Sprintf("%s %s (%s)", mark, m.zh, c),
			"起始遗物:"+relicName(m.relic), func() {
				u.cfg.char = c
				u.cfg.nStrike, u.cfg.nDefend = m.nStrike, m.nDefend
				u.render()
			}))
	}
	pv.rows = append(pv.rows,
		stepperRow("默认能量 (3=不挂遗物)", func() int { return u.cfg.energyCount }, func(v int) { u.cfg.energyCount = v }, 3, 6, 1),
		stepperRow("血量", func() int { return u.cfg.hp }, func(v int) { u.cfg.hp = v }, 1, 999, 10),
		stepperRow("初始金币 (每次+200)", func() int { return u.cfg.gold }, func(v int) { u.cfg.gold = v }, 0, 9999, 200),
		stepperRow("打击数量", func() int { return u.cfg.nStrike }, func(v int) { u.cfg.nStrike = v }, 0, 20, 1),
		stepperRow("防御数量", func() int { return u.cfg.nDefend }, func(v int) { u.cfg.nDefend = v }, 0, 20, 1),
		choiceRow("下一步: 升级与流派 →", "", func() { u.push(u.pageDeck) }),
	)
	return pv
}

func (u *ui) pageDeck() *pageView {
	ch := u.cfg.char
	names := make([]string, 0, len(curated.ARCH[ch]))
	for name := range curated.ARCH[ch] {
		names = append(names, name)
	}
	sort.Strings(names)
	pv := &pageView{
		title: "开局模板 · ② 升级与流派 (" + charDB[ch].zh + ")",
		hint:  "A=切换/选定流派并继续 · B=返回",
	}
	pv.rows = append(pv.rows,
		toggleRow("打击牌全部升级", func() bool { return u.cfg.upStrike }, func(b bool) { u.cfg.upStrike = b }),
		toggleRow("防御牌全部升级", func() bool { return u.cfg.upDefend }, func(b bool) { u.cfg.upDefend = b }),
		toggleRow("起手技能卡升级(痛击/电弧等)", func() bool { return u.cfg.upStarter }, func(b bool) { u.cfg.upStarter = b }),
		toggleRow("核心卡全部升级", func() bool { return u.cfg.upArch }, func(b bool) { u.cfg.upArch = b }),
		choiceRow("─ 流派核心卡 ─", "", nil))
	for _, name := range names {
		name := name
		ids := curated.ARCH[ch][name]
		mark := "○"
		if u.cfg.arch == name {
			mark = "●"
		}
		pv.rows = append(pv.rows, choiceRow(fmt.Sprintf("%s %s", mark, name), fmt.Sprintf("%d张", len(ids)), func() {
			u.cfg.arch = name
			u.push(u.pageRare)
		}))
	}
	pv.rows = append(pv.rows, choiceRow("○ 无 (不加核心卡)", "", func() {
		u.cfg.arch = "无"
		u.push(u.pageRare)
	}))
	return pv
}

func (u *ui) autoEnergyText() string {
	if u.cfg.energyCount <= 3 {
		return "无 (默认3能量)"
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
		title:    "开局模板 · ③ 遗物",
		hint:     "A=开/关 · 十字键在两列间移动 · B=返回",
		gridCols: 2,
	}
	pv.rows = append(pv.rows, textRow("能量遗物 (自动)", u.autoEnergyText(), nil))
	pv.rows = append(pv.rows, toggleRow("默认4件: 手里剑/钢笔尖/干枯之手/钨棒", func() bool { return u.cfg.rareDefault }, func(b bool) { u.cfg.rareDefault = b }))
	for _, rid := range curated.RARE_MORE {
		rid := rid
		pv.rows = append(pv.rows, toggleRow(relicName(rid),
			func() bool { return u.extraSel[rid] },
			func(b bool) { u.extraSel[rid] = b }))
	}
	pv.rows = append(pv.rows, choiceRow("下一步: 确认生成 →", "", func() { u.push(u.pageConfirm) }))
	return pv
}

func (u *ui) templateSummary() string {
	m := charDB[u.cfg.char]
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s · %d打击 + %d防御", m.zh, u.cfg.nStrike, u.cfg.nDefend)
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
		fmt.Fprintf(&sb, "(%d类升级)", up)
	}
	if u.cfg.arch != "" && u.cfg.arch != "无" {
		sb.WriteString(" · 流派[" + u.cfg.arch + "]")
		if u.cfg.upArch {
			sb.WriteString("升级")
		}
	}
	fmt.Fprintf(&sb, " · %d能量 · 血量%d · 金币%d · 手牌%d", u.cfg.energyCount, u.cfg.hp, u.cfg.gold, u.cfg.hand)
	return sb.String()
}

func (u *ui) pageConfirm() *pageView {
	pv := &pageView{title: "开局模板 · ④ 确认", hint: "A=执行 · B=返回修改"}
	target := filepath.Join(savesDir(), u.cfg.char+".autosave")
	note := "已存在, 直接覆盖写入"
	if _, err := os.Stat(target); err != nil {
		note = "不存在, 将自动从同角色最近存档复制底版"
	}
	pv.rows = append(pv.rows,
		textRow("配置", u.templateSummary(), nil),
		textRow("写入目标 (固定文件名)", target, nil),
		textRow("目标状态", note, nil))
	if u.svDoc != nil {
		if got := detectChar(u.svPath); got != "" && got != u.cfg.char {
			pv.rows = append(pv.rows, textRow("⚠ 当前加载的是其他角色存档", "将自动切换到 "+filepath.Base(target), nil))
		}
	}
	pv.rows = append(pv.rows,
		choiceRow("✅ 生成并写入存档", "", func() { u.doApplyTemplate() }),
		choiceRow("← 返回修改", "", func() { u.pop() }))
	return pv
}

func (u *ui) doApplyTemplate() {
	if u.cfg.char == "" {
		u.setStatus("✗ 请先在第①步选择角色")
		return
	}
	// 固定文件名: 模板永远写入 <角色>.autosave; 没有存档也能从零生成
	target := filepath.Join(savesDir(), u.cfg.char+".autosave")
	if _, err := os.Stat(target); err != nil {
		for _, f := range listSaves() {
			if detectChar(f) == u.cfg.char {
				if b, err := os.ReadFile(f); err == nil {
					if err := os.WriteFile(target, b, 0o644); err == nil {
						u.setStatus("已复制底版 → " + filepath.Base(target))
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
				u.setStatus("✗ 生成全新存档失败: " + err.Error())
				return
			}
			u.svPath, u.svDoc = target, doc
			u.setStatus("从零生成全新开局 (从涅奥开始)")
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
		title: "快速数值修改",
		hint:  "左右=±1 X=+10 Y=−10 · A 无效 · B=返回",
		rows: []*row{
			stepperRow("当前血量", get("current_health"), set("current_health"), 0, 9999, 10),
			stepperRow("最大血量", get("max_health"), set("max_health"), 1, 9999, 10),
			stepperRow("金币", get("gold"), set("gold"), 0, 999999, 200),
			stepperRow("手牌上限", get("hand_size"), set("hand_size"), 0, 20, 1),
			choiceRow("💾 写回存档", "", func() { u.writeBack("数值已保存") }),
			choiceRow("↻ 重新读取文件", "", func() {
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
		title:  "高级 JSON 编辑 (键盘)",
		hint:   "改完先「应用到内存」再「写回」· B=返回",
		custom: func() fyne.CanvasObject { return entry },
		rows: []*row{
			choiceRow("应用到内存", "", func() {
				var doc map[string]interface{}
				if err := jsonUnmarshal(entry.Text, &doc); err != nil {
					u.setStatus("✗ JSON 无效: " + err.Error())
					return
				}
				u.svDoc = doc
				u.setStatus("✓ JSON 已应用 (未写盘)")
			}),
			choiceRow("💾 写回存档", "", func() { u.writeBack("JSON 已保存") }),
			choiceRow("← 返回", "", func() { u.pop() }),
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
		return "(异常卡)"
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
	pv := &pageView{title: "卡组编辑", hint: "A=进入卡牌操作 · 左右=翻页 · B=返回"}
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
			choiceRow("◀ 上一页", fmt.Sprintf("第 %d/%d 页", u.deckPage+1, pages), func() {
				if u.deckPage > 0 {
					u.deckPage--
					u.render()
				}
			}),
			choiceRow("▶ 下一页", "", func() {
				if u.deckPage < pages-1 {
					u.deckPage++
					u.render()
				}
			}))
	}
	pv.rows = append(pv.rows,
		choiceRow("⬆ 升级所有卡牌 (+1)", "", func() {
			for _, v := range u.deckCards() {
				if m, ok := v.(map[string]interface{}); ok {
					if n := num(m, "upgrades"); n < 9 {
						m["upgrades"] = n + 1
					}
				}
			}
			u.setStatus("已升级全部卡牌 (未写盘)")
			u.render()
		}),
		choiceRow("➕ 添加卡牌", "", func() {
			u.addCardPage = 0
			u.push(u.pageAddCard)
		}),
		choiceRow("💾 写回存档", "", func() { u.writeBack("卡组已保存") }),
		choiceRow("↻ 重新读取", "", func() {
			if u.loadSave(u.svPath) {
				u.render()
			}
		}),
		choiceRow("← 返回", "", func() { u.pop() }),
	)
	return pv
}

// pageCardEdit 单卡操作
func (u *ui) pageCardEdit(idx int) func() *pageView {
	return func() *pageView {
		deck := u.deckCards()
		pv := &pageView{title: fmt.Sprintf("卡牌操作 · #%d", idx+1), hint: "A=执行 · B=返回卡组"}
		if idx >= len(deck) {
			pv.rows = append(pv.rows, textRow("(卡已不存在)", "", nil))
			return pv
		}
		m := deck[idx].(map[string]interface{})
		pv.rows = append(pv.rows, textRow("当前", cardLabel(m), nil))
		pv.rows = append(pv.rows, choiceRow("⬆ 升级 (+1)", "", func() {
			if n := num(m, "upgrades"); n < 9 {
				m["upgrades"] = n + 1
			}
			u.render()
		}))
		pv.rows = append(pv.rows, choiceRow("⬇ 降级 (−1)", "", func() {
			if n := num(m, "upgrades"); n > 0 {
				m["upgrades"] = n - 1
			}
			u.render()
		}))
		pv.rows = append(pv.rows, choiceRow("🗑 删除此卡", "", func() {
			u.svDoc["cards"] = append(deck[:idx], deck[idx+1:]...)
			u.setStatus("已删除 (未写盘)")
			u.pop()
		}))
		pv.rows = append(pv.rows, choiceRow("← 返回", "", func() { u.pop() }))
		return pv
	}
}

var charColor = map[string]string{"IRONCLAD": "RED", "THE_SILENT": "GREEN", "DEFECT": "BLUE", "WATCHER": "PURPLE"}

// pageAddCard 添加卡牌: 类别 -> 分页选择
func (u *ui) pageAddCard() *pageView {
	pv := &pageView{title: "添加卡牌", hint: "A=添加(可连续加) · B=返回卡组"}
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
		choiceRow(fmt.Sprintf("%s 本职业 (%s)", markJob, charDB[ch].zh), "", func() {
			u.addCat = "job"
			u.addCardPage = 0
			u.render()
		}),
		choiceRow(fmt.Sprintf("%s 无色卡", markCol), "", func() {
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
		r := newRowIcon(cardName(cid), zhRarity[cards[cid].Rarity], iconAt("cards", cid))
		r.activate = func() {
			arr, _ := u.svDoc["cards"].([]interface{})
			u.svDoc["cards"] = append(arr, map[string]interface{}{"id": cid, "upgrades": float64(0)})
			u.setStatus("已添加 " + cardName(cid) + " (未写盘)")
			u.render()
		}
		pv.rows = append(pv.rows, r)
	}
	if pages > 1 {
		pv.rows = append(pv.rows,
			choiceRow("◀ 上一页", fmt.Sprintf("第 %d/%d 页 · 共%d张", u.addCardPage+1, pages, len(pool)), func() {
				if u.addCardPage > 0 {
					u.addCardPage--
					u.render()
				}
			}),
			choiceRow("▶ 下一页", "", func() {
				if u.addCardPage < pages-1 {
					u.addCardPage++
					u.render()
				}
			}))
	}
	pv.rows = append(pv.rows, choiceRow("← 返回卡组", "", func() { u.pop() }))
	return pv
}

// pageRelicsEditor 遗物管理
func (u *ui) pageRelicsEditor() *pageView {
	pv := &pageView{title: "遗物管理", hint: "A=遗物操作 · B=返回"}
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
		choiceRow("➕ 添加遗物", "", func() {
			u.relPage = 0
			u.push(u.pageAddRelic)
		}),
		choiceRow("🧪 药水管理", "", func() {
			u.potPage = 0
			u.push(u.pagePotions)
		}),
		choiceRow("💾 写回存档", "", func() { u.writeBack("遗物已保存") }),
		choiceRow("← 返回", "", func() { u.pop() }),
	)
	return pv
}

func (u *ui) pageRelicEdit(idx int) func() *pageView {
	return func() *pageView {
		pv := &pageView{title: "遗物操作", hint: "A=执行 · B=返回"}
		rel, _ := u.svDoc["relics"].([]interface{})
		if idx >= len(rel) {
			pv.rows = append(pv.rows, textRow("(遗物已不存在)", "", nil))
			return pv
		}
		rid, _ := rel[idx].(string)
		pv.rows = append(pv.rows, textRow("当前", relicName(rid), nil))
		if idx == 0 {
			pv.rows = append(pv.rows, textRow("(起始遗物, 不建议删除)", "", nil))
		} else {
			pv.rows = append(pv.rows, choiceRow("🗑 删除此遗物", "", func() {
				u.svDoc["relics"] = append(rel[:idx], rel[idx+1:]...)
				u.syncRelicCounters()
				u.setStatus("已删除 " + relicName(rid) + " (未写盘)")
				u.pop()
			}))
		}
		pv.rows = append(pv.rows, choiceRow("← 返回", "", func() { u.pop() }))
		return pv
	}
}

func (u *ui) pageAddRelic() *pageView {
	pv := &pageView{title: "添加遗物", hint: "A=添加(可连续加) · B=返回"}
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
			u.setStatus("已添加 " + relicName(rid) + " (未写盘)")
			u.render()
		}
		pv.rows = append(pv.rows, r)
	}
	if pages > 1 {
		pv.rows = append(pv.rows,
			choiceRow("◀ 上一页", fmt.Sprintf("第 %d/%d 页 · 共%d件", u.relPage+1, pages, len(pool)), func() {
				if u.relPage > 0 {
					u.relPage--
					u.render()
				}
			}),
			choiceRow("▶ 下一页", "", func() {
				if u.relPage < pages-1 {
					u.relPage++
					u.render()
				}
			}))
	}
	pv.rows = append(pv.rows, choiceRow("← 返回遗物管理", "", func() { u.pop() }))
	return pv
}

// pagePotions 药水管理
func (u *ui) pagePotions() *pageView {
	pv := &pageView{title: "药水管理", hint: "A=槽位操作 · B=返回"}
	var pots []interface{}
	if u.svDoc != nil {
		pots, _ = u.svDoc["potions"].([]interface{})
	}
	if pots == nil {
		pv.rows = append(pv.rows, textRow("(存档没有药水槽字段, 写回时会创建)", "", nil))
	}
	for i, p := range pots {
		i := i
		pid, _ := p.(string)
		right := ""
		if pid != "Potion Slot" {
			right = "有药水"
		}
		row := newRowIcon(fmt.Sprintf("槽位%d: %s", i+1, potionName(pid)), right, iconAt("potions", "bottle"))
		row.activate = func() { u.push(u.pagePotionEdit(i)) }
		pv.rows = append(pv.rows, row)
	}
	pv.rows = append(pv.rows,
		choiceRow("💾 写回存档", "", func() { u.writeBack("药水已保存") }),
		choiceRow("← 返回", "", func() { u.pop() }),
	)
	return pv
}

func (u *ui) pagePotionEdit(idx int) func() *pageView {
	return func() *pageView {
		pv := &pageView{title: fmt.Sprintf("药水槽位 %d", idx+1), hint: "A=执行 · B=返回"}
		pots, _ := u.svDoc["potions"].([]interface{})
		if idx >= len(pots) {
			pv.rows = append(pv.rows, textRow("(槽位已不存在)", "", nil))
			return pv
		}
		pid, _ := pots[idx].(string)
		pv.rows = append(pv.rows, textRow("当前", potionName(pid), nil))
		if pid != "Potion Slot" {
			pv.rows = append(pv.rows, choiceRow("🗑 清空 (删除药水)", "", func() {
				pots[idx] = "Potion Slot"
				u.setStatus("已清空 (未写盘)")
				u.render()
			}))
		}
		pv.rows = append(pv.rows, choiceRow("➕ 更换药水 →", "", func() {
			u.potPage = 0
			u.push(u.pagePickPotion(idx))
		}))
		pv.rows = append(pv.rows, choiceRow("← 返回", "", func() { u.pop() }))
		return pv
	}
}

func (u *ui) pagePickPotion(slot int) func() *pageView {
	return func() *pageView {
		pv := &pageView{title: "选择药水", hint: "A=装入该槽位 · B=返回"}
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
					u.setStatus("已装入 " + potionName(pid) + " (未写盘)")
					u.pop()
				}
			}
			pv.rows = append(pv.rows, r)
		}
		if pages > 1 {
			pv.rows = append(pv.rows,
				choiceRow("◀ 上一页", fmt.Sprintf("第 %d/%d 页 · 共%d种", u.potPage+1, pages, len(pool)), func() {
					if u.potPage > 0 {
						u.potPage--
						u.render()
					}
				}),
				choiceRow("▶ 下一页", "", func() {
					if u.potPage < pages-1 {
						u.potPage++
						u.render()
					}
				}))
		}
		pv.rows = append(pv.rows, choiceRow("← 返回", "", func() { u.pop() }))
		return pv
	}
}

// ---------- main ----------

func main() {
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
			arch: "无", energyCount: 3, rareDefault: true, hp: 120, gold: 200, hand: 10,
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
