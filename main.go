// SpireForge 核心逻辑: 数据、存档编解码、模板应用、手柄事件
package main

import (
	_ "embed"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
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
		return "(空槽)"
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
	"IRONCLAD":   {"战士", []string{"Bash"}, "Strike_R", 5, "Defend_R", 4, "Burning Blood", 80},
	"THE_SILENT": {"猎手", []string{"Survivor", "Neutralize"}, "Strike_G", 5, "Defend_G", 4, "Ring of the Snake", 70},
	"DEFECT":     {"机器人", []string{"Zap", "Dualcast"}, "Strike_B", 4, "Defend_B", 3, "Cracked Core", 72},
	"WATCHER":    {"观者", []string{"Eruption", "Vigilance"}, "Strike_P", 4, "Defend_P", 4, "Pure Water", 72},
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
var energyRelicDesc = map[string]string{
	"Coffee Dripper":      "休息点无法休息回血",
	"Ectoplasm":           "无法获得金币",
	"Fusion Hammer":       "休息点无法升级卡牌",
	"Runic Dome":          "看不到敌人意图",
	"Cursed Key":          "每开一个宝箱失去6血",
	"Philosopher's Stone": "所有敌人+1力量",
	"Sozu":                "无法获得药水",
}

func cardName(id string) string {
	if c, ok := cards[id]; ok && c.Zh != "" {
		return c.Zh
	}
	if z, ok := cardZhFallback[id]; ok {
		return z
	}
	if c, ok := cards[id]; ok {
		return c.En
	}
	return id
}

func relicName(id string) string {
	if r, ok := relics[id]; ok && r.Zh != "" {
		return r.Zh
	}
	if z, ok := relicZhFallback[id]; ok {
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

// ---------- 模板应用 ----------

type templateCfg struct {
	char        string
	nStrike     int
	nDefend     int
	upStrike    bool
	upDefend    bool
	upStarter   bool
	arch        string
	upArch      bool
	energyCount int // 3=无能量遗物, 每+1自动挂一件
	rareDefault bool
	rareExtra   []string
	hp          int
	gold        int
	hand        int
}

func applyTemplate(doc map[string]interface{}, cfg templateCfg) (string, error) {
	meta, ok := charDB[cfg.char]
	if !ok {
		return "", fmt.Errorf("未知角色 %s", cfg.char)
	}
	var deck []interface{}
	addCard := func(id string, up int) {
		deck = append(deck, map[string]interface{}{"id": id, "upgrades": float64(up)})
	}
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
	if cfg.arch != "" && cfg.arch != "无" {
		for _, id := range curated.ARCH[cfg.char][cfg.arch] {
			addCard(id, b2i(cfg.upArch))
		}
	}
	doc["cards"] = deck

	var rel []string
	rel = append(rel, meta.relic)
	if cfg.energyCount > 3 {
		need := cfg.energyCount - 3
		if need > len(curated.ENERGY) {
			need = len(curated.ENERGY)
		}
		rel = append(rel, curated.ENERGY[:need]...)
	}
	if cfg.rareDefault {
		rel = append(rel, curated.RARE_DEFAULT...)
	}
	rel = append(rel, cfg.rareExtra...)
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
	doc["gold"] = float64(cfg.gold)
	if cfg.hand > 0 {
		doc["hand_size"] = float64(cfg.hand)
	}
	return fmt.Sprintf("%s · %d能量 · 牌组 %d 张 · 遗物 %d 件 · 血量 %d · 金币 %d",
		meta.zh, cfg.energyCount, len(deck), len(rel), cfg.hp, cfg.gold), nil
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
