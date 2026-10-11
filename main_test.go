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

	cfg := templateCfg{
		char: "IRONCLAD", nStrike: 5, nDefend: 4, upStrike: true, upStarter: true,
		arch: "力量重击流", upArch: true, energyCount: 4,
		rareDefault: true, rareExtra: []string{"Girya"}, hp: 120, gold: 99, hand: 10,
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
	if len(deck) != 1+5+4+8 {
		t.Fatalf("deck = %d, want 18", len(deck))
	}
	relicsArr := doc2["relics"].([]interface{})
	if len(relicsArr) != 1+1+4+1 {
		t.Fatalf("relics = %d, want 7: %v", len(relicsArr), relicsArr)
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
