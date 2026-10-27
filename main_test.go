package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRoundTripAndTemplate(t *testing.T) {
	initData()
	raw, err := os.ReadFile("../save-the-spire/src/example.json")
	if err != nil {
		t.Skip("example.json not available")
	}
	var doc map[string]interface{}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	p := filepath.Join(dir, "IRONCLAD.autosave")
	b, err := encodeSave(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
	doc2, err := decodeSave(p)
	if err != nil {
		t.Fatal(err)
	}
	if int(num(doc2, "gold")) != int(num(doc, "gold")) {
		t.Fatalf("gold round-trip mismatch")
	}

	// 标准构筑: 打击/防御/起手 + 能量遗物 (无流派)
	cfg := templateCfg{
		char: "IRONCLAD", nStrike: 5, nDefend: 4, upStrike: true, upStarter: true,
		energyCount: 4, rareDefault: true, rareExtra: []string{"Girya"},
		hp: 120, gold: 99, hand: 10,
	}
	msg, err := applyTemplate(doc2, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(msg)
	if int(num(doc2, "max_health")) != 120 || int(num(doc2, "current_health")) != 120 {
		t.Fatal("hp not applied")
	}
	deck := doc2["cards"].([]interface{})
	if len(deck) != 1+5+4 {
		t.Fatalf("deck = %d, want 10", len(deck))
	}
	relicsArr := doc2["relics"].([]interface{})
	// 起始遗物 + 1能量 + 4稀有 + 1额外 = 7, 且 Girya 去重后不重复出现
	if len(relicsArr) != 7 {
		t.Fatalf("relics = %d, want 7: %v", len(relicsArr), relicsArr)
	}
	// 涅奥模式: 进度字段全部重置到第一层
	if doc2["current_room"] != "com.megacrit.cardcrawl.rooms.NeowRoom" || int(num(doc2, "floor_num")) != 0 || int(num(doc2, "act_num")) != 1 {
		t.Fatal("涅奥开局进度未重置:", doc2["current_room"], doc2["floor_num"])
	}
	if doc2["chose_neow_reward"] != false {
		t.Fatal("chose_neow_reward 未重置")
	}

	// 流派构筑: 抛弃原卡组, 6张核心 + 2流派遗物
	cfg2 := templateCfg{
		char: "IRONCLAD", useArch: true, arch: "力量重击流", archCount: 6, upArch: true,
		archRelics: true, energyCount: 3, rareDefault: true, hp: 120, gold: 200, hand: 10, threeKeys: true,
	}
	if _, err := applyTemplate(doc2, cfg2); err != nil {
		t.Fatal(err)
	}
	deck = doc2["cards"].([]interface{})
	if len(deck) != 6 {
		t.Fatalf("archetype deck = %d, want 6", len(deck))
	}
	for _, v := range deck {
		id := v.(map[string]interface{})["id"]
		bad := true
		for _, cid := range curated.ARCH["IRONCLAD"]["力量重击流"] {
			if id == cid {
				bad = false
			}
		}
		if bad {
			t.Fatalf("非核心卡混入: %v", id)
		}
	}
	relicsArr = doc2["relics"].([]interface{})
	// 起始 + 2流派遗物 + 4稀有 = 7 (无能量遗物)
	if len(relicsArr) != 7 {
		t.Fatalf("archetype relics = %d, want 7: %v", len(relicsArr), relicsArr)
	}
	for _, k := range []string{"has_ruby_key", "has_sapphire_key", "has_emerald_key"} {
		if doc2[k] != true {
			t.Fatalf("%s 应为 true", k)
		}
	}
	if cs := doc2["relic_counters"].([]interface{}); len(cs) != len(relicsArr) {
		t.Fatal("relic_counters misaligned")
	}
	b2, _ := encodeSave(doc2)
	if err := os.WriteFile(p, b2, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := decodeSave(p); err != nil {
		t.Fatal(err)
	}
}

// TestVdfHandcrafted: 按二进制 VDF 规范手工构造的最小 shortcuts 文件
func TestVdfHandcrafted(t *testing.T) {
	var b bytes.Buffer
	vdfWriteMap(&b, "shortcuts", map[string]interface{}{
		"0": map[string]interface{}{
			"appid":   -936695550,
			"AppName": "尖塔工坊 SpireForge",
			"Exe":     `"/usr/bin/flatpak"`,
		},
	})
	b.WriteByte(vbEnd)
	d, err := vdfLoad(b.Bytes())
	if err != nil {
		t.Fatal("parse:", err)
	}
	e := d["shortcuts"].(map[string]interface{})["0"].(map[string]interface{})
	if e["AppName"] != "尖塔工坊 SpireForge" || e["appid"].(int) != -936695550 {
		t.Fatalf("mismatch: %v", e)
	}
}

func TestAct3StartAndSeedRandom(t *testing.T) {
	initData()
	doc := map[string]interface{}{"seed": float64(12345), "metric_seed_played": float64(12345)}
	cfg := templateCfg{
		char: "IRONCLAD", nStrike: 5, nDefend: 4, startAct3: true,
		energyCount: 3, rareDefault: false, hp: 99, gold: 200, hand: 10,
	}
	if _, err := applyTemplate(doc, cfg); err != nil {
		t.Fatal(err)
	}
	if int(num(doc, "act_num")) != 3 {
		t.Fatal("act_num != 3")
	}
	if doc["level_name"] != "TheBeyond" {
		t.Fatal("level_name != TheBeyond")
	}
	if int(num(doc, "floor_num")) != 33 {
		t.Fatal("floor_num != 33")
	}
	if doc["has_ruby_key"] != false || doc["has_sapphire_key"] != false || doc["has_emerald_key"] != false {
		t.Fatal("act3 模式钥匙应为 false (自取)")
	}
	if doc["seed"].(float64) == 12345 {
		t.Fatal("seed 未随机化")
	}
	if _, ok := doc["max_orbs"]; ok {
		t.Fatal("非机器人不应有 max_orbs")
	}
}
