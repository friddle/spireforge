// SpireForge 核心逻辑: 数据、存档编解码、模板应用、手柄事件
package main

import (
	"crypto/rand"
	_ "embed"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ---------- 内嵌数据 ----------

//go:embed data/cards.json
var cardsJSON []byte

//go:embed data/relics.json
var relicsJSON []byte

//go:embed data/curated.json
var curatedJSON []byte

//go:embed data/potions.json
var potionsJSON []byte

//go:embed data/skeleton.json
var skeletonJSON []byte

type cardInfo struct {
	En     string `json:"en"`
	Zh     string `json:"zh"`
	Color  string `json:"color"`
	Rarity string `json:"rarity"`
}
type relicInfo struct {
	En string `json:"en"`
	Zh string `json:"zh"`
}
type curatedData struct {
	ARCH         map[string]map[string][]string `json:"ARCH"`
	ARCH_RELICS  map[string][]string            `json:"ARCH_RELICS"`
	ENERGY       []string                       `json:"ENERGY"`
	RARE_DEFAULT []string                       `json:"RARE_DEFAULT"`
	RARE_MORE    []string                       `json:"RARE_MORE"`
}

type potionInfo struct {
	En string `json:"en"`
	Zh string `json:"zh"`
}

var cards map[string]cardInfo
var relics map[string]relicInfo
var potions map[string]potionInfo
var curated curatedData

func initData() {
	cards = map[string]cardInfo{}
	relics = map[string]relicInfo{}
	potions = map[string]potionInfo{}
	curated = curatedData{}
	_ = json.Unmarshal(cardsJSON, &cards)
	_ = json.Unmarshal(relicsJSON, &relics)
	_ = json.Unmarshal(potionsJSON, &potions)
	_ = json.Unmarshal(curatedJSON, &curated)
}

// loadSkeleton 生成全新开局存档文档 (从涅奥开始)
func loadSkeleton() (map[string]interface{}, error) {
	var doc map[string]interface{}
	if err := json.Unmarshal(skeletonJSON, &doc); err != nil {
		return nil, err
	}
	return doc, nil
}

func potionName(id string) string {
	if id == "Potion Slot" {
		return L("(空槽)", "(empty slot)")
	}
	if lang == "en" {
		if p, ok := potions[id]; ok && p.En != "" {
			return p.En
		}
		return id
	}
	if p, ok := potions[id]; ok && p.Zh != "" {
		return p.Zh
	}
	if p, ok := potions[id]; ok {
		return p.En
	}
	return id
}

// ---------- 角色元数据 ----------

type charMeta struct {
	zh       string
	en       string
	starters []string
	strike   string
	nStrike  int
	defend   string
	nDefend  int
	relic    string
	hp       int
}

var charOrder = []string{"IRONCLAD", "THE_SILENT", "DEFECT", "WATCHER"}

var charDB = map[string]charMeta{
	"IRONCLAD":   {"战士", "Ironclad", []string{"Bash"}, "Strike_R", 5, "Defend_R", 4, "Burning Blood", 80},
	"THE_SILENT": {"猎手", "The Silent", []string{"Survivor", "Neutralize"}, "Strike_G", 5, "Defend_G", 4, "Ring of the Snake", 70},
	"DEFECT":     {"机器人", "Defect", []string{"Zap", "Dualcast"}, "Strike_B", 4, "Defend_B", 3, "Cracked Core", 72},
	"WATCHER":    {"观者", "Watcher", []string{"Eruption", "Vigilance"}, "Strike_P", 4, "Defend_P", 4, "Pure Water", 72},
}

// ---------- 双语 ----------

var lang = "zh" // zh / en

func L(zh, en string) string {
	if lang == "en" {
		return en
	}
	return zh
}

func langFile() string {
	if d, err := os.UserConfigDir(); err == nil {
		return filepath.Join(d, "spireforge", "lang")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config/spireforge/lang")
}

func loadLangPref() {
	if b, err := os.ReadFile(langFile()); err == nil && strings.TrimSpace(string(b)) == "en" {
		lang = "en"
	}
}

func toggleLang() {
	if lang == "en" {
		lang = "zh"
	} else {
		lang = "en"
	}
	_ = os.MkdirAll(filepath.Dir(langFile()), 0o755)
	_ = os.WriteFile(langFile(), []byte(lang), 0o644)
}

var cardZhFallback = map[string]string{
	"Bash": "痛击", "Survivor": "生存本能", "Neutralize": "中和", "Zap": "电弧",
	"Dualcast": "双重施法", "Eruption": "爆发", "Vigilance": "警惕", "Reform": "重构",
	"Strike_R": "打击", "Strike_G": "打击", "Strike_B": "打击", "Strike_P": "打击",
	"Defend_R": "防御", "Defend_G": "防御", "Defend_B": "防御", "Defend_P": "防御",
}
var relicZhFallback = map[string]string{
	"Burning Blood": "燃烧之血", "Ring of the Snake": "蛇之戒指", "Cracked Core": "破碎核心", "Pure Water": "净水",
}

// 能量遗物负面效果说明
var energyRelicDescZh = map[string]string{
	"Coffee Dripper":      "休息点无法休息回血",
	"Ectoplasm":           "无法获得金币",
	"Fusion Hammer":       "休息点无法升级卡牌",
	"Runic Dome":          "看不到敌人意图",
	"Cursed Key":          "每开一个宝箱失去6血",
	"Philosopher's Stone": "所有敌人+1力量",
	"Sozu":                "无法获得药水",
}
var energyRelicDescEn = map[string]string{
	"Coffee Dripper":      "Cannot rest at rest sites",
	"Ectoplasm":           "Cannot gain gold",
	"Fusion Hammer":       "Cannot upgrade cards at rest sites",
	"Runic Dome":          "Cannot see enemy intents",
	"Cursed Key":          "Lose 6 HP per chest opened",
	"Philosopher's Stone": "All enemies gain 1 Strength",
	"Sozu":                "Cannot gain potions",
}

func energyDesc(id string) string {
	return L(energyRelicDescZh[id], energyRelicDescEn[id])
}

var cardEnFallback = map[string]string{
	"Strike_R": "Strike", "Strike_G": "Strike", "Strike_B": "Strike", "Strike_P": "Strike",
	"Defend_R": "Defend", "Defend_G": "Defend", "Defend_B": "Defend", "Defend_P": "Defend",
}

func cardName(id string) string {
	c, ok := cards[id]
	if lang == "en" {
		if ok && c.En != "" {
			return c.En
		}
		if e, ok2 := cardEnFallback[id]; ok2 {
			return e
		}
		return id
	}
	if ok && c.Zh != "" {
		return c.Zh
	}
	if z, ok2 := cardZhFallback[id]; ok2 {
		return z
	}
	if ok {
		return c.En
	}
	return id
}

func relicName(id string) string {
	if lang == "en" {
		if r, ok := relics[id]; ok && r.En != "" {
			return r.En
		}
		return id
	}
	if r, ok := relics[id]; ok && r.Zh != "" {
		return r.Zh
	}
	if z, ok2 := relicZhFallback[id]; ok2 {
		return z
	}
	if r, ok := relics[id]; ok {
		return r.En
	}
	return id
}

// ---------- 存档编解码 ----------

func xorKey(data []byte) []byte {
	key := []byte("key")
	out := make([]byte, len(data))
	for i := range data {
		out[i] = data[i] ^ key[i%len(key)]
	}
	return out
}

func decodeSave(path string) (map[string]interface{}, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	dec, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil {
		return nil, fmt.Errorf("base64 解码失败: %w", err)
	}
	var doc map[string]interface{}
	if err := json.Unmarshal(xorKey(dec), &doc); err != nil {
		return nil, fmt.Errorf("JSON 解析失败(不是有效存档): %w", err)
	}
	return doc, nil
}

func encodeSave(doc map[string]interface{}) ([]byte, error) {
	plain, err := json.Marshal(doc)
	if err != nil {
		return nil, err
	}
	return []byte(base64.StdEncoding.EncodeToString(xorKey(plain))), nil
}

func detectChar(path string) string {
	base := filepath.Base(path)
	for _, c := range charOrder {
		if strings.HasPrefix(base, c) {
			return c
		}
	}
	return ""
}

func num(doc map[string]interface{}, key string) float64 {
	if v, ok := doc[key].(float64); ok {
		return v
	}
	return 0
}

func toIface(ss []string) []interface{} {
	out := make([]interface{}, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

func mustRead(p string) []byte {
	b, _ := os.ReadFile(p)
	return b
}

func savesDir() string {
	if d := os.Getenv("SPIREFORCE_SAVES_DIR"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	for _, c := range []string{
		home + "/.local/share/Steam/steamapps/common/SlayTheSpire/saves",
		home + "/.steam/steam/steamapps/common/SlayTheSpire/saves",
	} {
		if fi, err := os.Stat(c); err == nil && fi.IsDir() {
			return c
		}
	}
	return home + "/.local/share/Steam/steamapps/common/SlayTheSpire/saves"
}

func listSaves() []string {
	ents, _ := os.ReadDir(savesDir())
	var out []string
	for _, e := range ents {
		if e.IsDir() || !strings.Contains(e.Name(), "autosave") || strings.Contains(e.Name(), ".bak-") {
			continue
		}
		out = append(out, filepath.Join(savesDir(), e.Name()))
	}
	sort.Slice(out, func(i, j int) bool {
		return fileModTime(out[i]).After(fileModTime(out[j]))
	})
	return out
}

func fileModTime(p string) time.Time {
	fi, err := os.Stat(p)
	if err != nil {
		return time.Time{}
	}
	return fi.ModTime()
}

// ---------- 第三幕(三钥匙局面)数据池 (取自真实第三幕存档) ----------

var act3Monsters = []string{
	"Transient", "Writhing Mass", "Sphere and 2 Shapes", "Spire Growth",
	"4 Shapes", "3 Darklings", "Cultist", "Snecko", "Byrds", "Centurion and Mystic",
}
var act3Elites = []string{
	"Nemesis", "Reptomancer", "Giant Head",
	"Nemesis", "Reptomancer", "Giant Head",
	"Nemesis", "Giant Head", "Reptomancer",
}
var act3Events = []string{
	"Falling", "MindBloom", "The Moai Head", "Mysterious Sphere", "SensoryStone",
	"Tomb of Lord Red Mask", "Winding Halls", "The Library", "Ssserpent", "Secret Portal",
}
var act3Bosses = []string{"Awakened One", "Time Eater", "Donu and Deca"}

func randomSeed() int64 {
	v, err := rand.Int(rand.Reader, big.NewInt(9223372036854775807))
	if err != nil {
		return time.Now().UnixNano()
	}
	n := v.Int64()
	if n > 4503599627370496 { // 游戏种子UI支持范围内取正数段
		n = n % 4503599627370496
	}
	return n
}

// ---------- 模板应用 ----------

type templateCfg struct {
	char        string
	nStrike     int
	nDefend     int
	upStrike    bool
	upDefend    bool
	upStarter   bool
	arch        string
	useArch     bool // 流派构筑: 抛弃原卡组, 纯核心卡
	archCount   int
	archRelics  bool
	upArch      bool
	energyCount int  // 3=无能量遗物, 每+1自动挂一件
	orbSlots    int  // 仅机器人(DEFECT)生效
	threeKeys   bool // 三把钥匙: 开启终幕(心脏)
	startAct3   bool // 第三幕开局(三钥匙局面, 钥匙需自取)
	rareDefault bool
	rareExtra   []string
	hp          int
	gold        int
	hand        int
}

func applyTemplate(doc map[string]interface{}, cfg templateCfg) (string, error) {
	meta, ok := charDB[cfg.char]
	if !ok {
		return "", fmt.Errorf(L("未知角色 %s", "unknown character %s"), cfg.char)
	}
	var deck []interface{}
	addCard := func(id string, up int) {
		deck = append(deck, map[string]interface{}{"id": id, "upgrades": float64(up)})
	}
	archMode := cfg.useArch && cfg.arch != "" && cfg.arch != "无"
	if archMode {
		// 流派构筑: 原卡组全部抛弃, 仅保留核心卡
		ids := curated.ARCH[cfg.char][cfg.arch]
		n := cfg.archCount
		if n > len(ids) {
			n = len(ids)
		}
		for i := 0; i < n; i++ {
			addCard(ids[i%len(ids)], b2i(cfg.upArch))
		}
	} else {
		for _, st := range meta.starters {
			addCard(st, b2i(cfg.upStarter))
		}
		if cfg.nStrike > 0 {
			addCard(meta.strike, b2i(cfg.upStrike))
		}
		for i := 1; i < cfg.nStrike; i++ {
			addCard(meta.strike, b2i(cfg.upStrike))
		}
		for i := 0; i < cfg.nDefend; i++ {
			addCard(meta.defend, b2i(cfg.upDefend))
		}
	}
	doc["cards"] = deck

	// 种子: 每次生成都随机, 保证地图/事件/奖励不复读
	seed := randomSeed()
	doc["seed"] = float64(seed)
	doc["metric_seed_played"] = float64(seed)

	// 第三幕开局(三钥匙局面): 钥匙不直接给, 进第三幕自己收集
	if cfg.startAct3 {
		doc["act_num"] = float64(3)
		doc["level_name"] = "TheBeyond"
		doc["floor_num"] = float64(33)
		doc["current_room"] = "com.megacrit.cardcrawl.rooms.MonsterRoom"
		doc["monster_list"] = toIface(act3Monsters)
		doc["elite_monster_list"] = toIface(act3Elites)
		doc["event_list"] = toIface(act3Events)
		doc["boss_list"] = toIface(act3Bosses)
		doc["boss"] = act3Bosses[seed%int64(len(act3Bosses))]
		doc["event_chances"] = []interface{}{0.3, 0.4, 0.06, 0.08}
		doc["potion_chance"] = float64(-30)
		doc["is_final_act_on"] = false
		// 钥匙局面: 关闭直接获得, 第三幕内自行收集
		doc["has_ruby_key"] = false
		doc["has_sapphire_key"] = false
		doc["has_emerald_key"] = false
	}

	var rel []string
	seen := map[string]bool{}
	addRelic := func(id string) {
		if id != "" && !seen[id] {
			seen[id] = true
			rel = append(rel, id)
		}
	}
	addRelic(meta.relic)
	if cfg.energyCount > 3 {
		need := cfg.energyCount - 3
		if need > len(curated.ENERGY) {
			need = len(curated.ENERGY)
		}
		for _, r := range curated.ENERGY[:need] {
			addRelic(r)
		}
	}
	if archMode && cfg.archRelics {
		for _, r := range curated.ARCH_RELICS[cfg.arch] {
			addRelic(r)
		}
	}
	if cfg.rareDefault {
		for _, r := range curated.RARE_DEFAULT {
			addRelic(r)
		}
	}
	for _, r := range cfg.rareExtra {
		addRelic(r)
	}
	relAny := make([]interface{}, len(rel))
	for i, r := range rel {
		relAny[i] = r
	}
	doc["relics"] = relAny
	counters := make([]interface{}, len(rel))
	for i := range counters {
		counters[i] = float64(-1)
	}
	doc["relic_counters"] = counters

	if cfg.hp > 0 {
		doc["max_health"] = float64(cfg.hp)
		doc["current_health"] = float64(cfg.hp)
	}
	// 机器人: 充能球槽位 (其他角色删除该字段)
	if cfg.char == "DEFECT" && cfg.orbSlots > 0 {
		doc["max_orbs"] = float64(cfg.orbSlots)
	} else {
		delete(doc, "max_orbs")
	}
	// 三把钥匙: 打败三幕Boss后可进终幕(心脏)
	doc["has_ruby_key"] = cfg.threeKeys
	doc["has_sapphire_key"] = cfg.threeKeys
	doc["has_emerald_key"] = cfg.threeKeys
	doc["gold"] = float64(cfg.gold)
	if cfg.hand > 0 {
		doc["hand_size"] = float64(cfg.hand)
	}
	return fmt.Sprintf("%s · %d%s · %s %d · %s %d · HP %d · %s %d",
		L(meta.zh, meta.en), cfg.energyCount, L("能量", "energy"),
		L("牌组", "deck"), len(deck), L("遗物", "relics"), len(rel),
		cfg.hp, L("金币", "gold"), cfg.gold), nil
}

// ---------- 手柄事件 (evdev 直读, 不依赖 Steam Input 配置) ----------

const (
	evKey    = 0x01
	evAbs    = 0x03
	absHat0X = 16
	absHat0Y = 17
	btnSouth = 0x130 // A
	btnEast  = 0x131 // B
	btnWest  = 0x132 // Y
	btnNorth = 0x133 // X
	btnStart = 0x13B
)

func padWatch(events chan<- string, quit <-chan struct{}) {
	opened := map[string]bool{}
	for {
		scanDevices(events, quit, opened)
		select {
		case <-quit:
			return
		case <-time.After(5 * time.Second):
		}
	}
}

func scanDevices(events chan<- string, quit <-chan struct{}, opened map[string]bool) {
	ents, _ := os.ReadDir("/dev/input")
	for _, e := range ents {
		if !strings.HasPrefix(e.Name(), "event") {
			continue
		}
		if opened[e.Name()] {
			continue
		}
		opened[e.Name()] = true
		go func(name string) {
			f, err := os.OpenFile("/dev/input/"+name, os.O_RDONLY, 0)
			if err != nil {
				return
			}
			defer f.Close()
			buf := make([]byte, 24)
			var lastX, lastY int32
			const stickThr = 16000
			for {
				n, err := f.Read(buf)
				if err != nil || n < 24 {
					return
				}
				typ := binary.LittleEndian.Uint16(buf[16:18])
				code := binary.LittleEndian.Uint16(buf[18:20])
				value := int32(binary.LittleEndian.Uint32(buf[20:24]))
				select {
				case <-quit:
					return
				default:
				}
				// 左摇杆 (Steam Input 虚拟手柄同样走这些码值)
				if typ == evAbs && code == 0 { // ABS_X
					if value > stickThr && lastX <= stickThr {
						sendEvent(events, quit, "right")
					} else if value < -stickThr && lastX >= -stickThr {
						sendEvent(events, quit, "left")
					}
					lastX = value
					continue
				}
				if typ == evAbs && code == 1 { // ABS_Y
					if value > stickThr && lastY <= stickThr {
						sendEvent(events, quit, "down")
					} else if value < -stickThr && lastY >= -stickThr {
						sendEvent(events, quit, "up")
					}
					lastY = value
					continue
				}
				isPress := typ == evKey && value == 1
				isHat := typ == evAbs && code >= absHat0X && code <= absHat0Y && value != 0
				if !isPress && !isHat {
					continue
				}
				var out string
				switch {
				case isHat && code == absHat0Y && value == -1:
					out = "up"
				case isHat && code == absHat0Y && value == 1:
					out = "down"
				case isHat && code == absHat0X && value == -1:
					out = "left"
				case isHat && code == absHat0X && value == 1:
					out = "right"
				case isPress && code == btnSouth:
					out = "a"
				case isPress && code == btnEast:
					out = "b"
				case isPress && code == btnNorth:
					out = "x"
				case isPress && code == btnWest:
					out = "y"
				case isPress && code == btnStart:
					out = "start"
				}
				if out != "" {
					sendEvent(events, quit, out)
				}
			}
		}(e.Name())
	}
}

func sendEvent(events chan<- string, quit <-chan struct{}, out string) {
	select {
	case events <- out:
	case <-quit:
	}
}
