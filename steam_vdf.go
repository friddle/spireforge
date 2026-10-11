// SpireForge: 二进制 VDF 读写 (shortcuts.vdf) + Steam 库自导入
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ---------- 二进制 VDF ----------

const (
	vbMap    = 0x00
	vbString = 0x01
	vbInt32  = 0x02
	vbEnd    = 0x08
)

// 二进制 VDF 字符串: NUL 结尾 (不是长度前缀)

func vdfWString(b *bytes.Buffer, s string) {
	b.WriteString(s)
	b.WriteByte(0)
}

func vdfWriteString(b *bytes.Buffer, key, val string) {
	b.WriteByte(vbString)
	vdfWString(b, key)
	vdfWString(b, val)
}

func vdfWriteInt(b *bytes.Buffer, key string, val int32) {
	b.WriteByte(vbInt32)
	vdfWString(b, key)
	binary.Write(b, binary.LittleEndian, val)
}

func vdfWriteValue(b *bytes.Buffer, key string, v interface{}) {
	switch vv := v.(type) {
	case map[string]interface{}:
		vdfWriteMap(b, key, vv)
	case string:
		vdfWriteString(b, key, vv)
	case int:
		vdfWriteInt(b, key, int32(vv))
	case int32:
		vdfWriteInt(b, key, vv)
	}
}

func vdfWriteMap(b *bytes.Buffer, key string, m map[string]interface{}) {
	b.WriteByte(vbMap)
	vdfWString(b, key)
	for k, v := range m {
		vdfWriteValue(b, k, v)
	}
	b.WriteByte(vbEnd)
}

// vdfDump 与 python-vdf/Steam 一致: 根级直接写条目, 以 0x08 结束
func vdfDump(m map[string]interface{}) []byte {
	var b bytes.Buffer
	for k, v := range m {
		vdfWriteValue(&b, k, v)
	}
	b.WriteByte(vbEnd)
	return b.Bytes()
}

func vdfReadCString(d []byte, p int) (string, int, error) {
	end := bytes.IndexByte(d[p:], 0)
	if end < 0 {
		return "", p, fmt.Errorf("字符串未终止")
	}
	return string(d[p : p+end]), p + end + 1, nil
}

func vdfParse(d []byte, p int) (map[string]interface{}, int, error) {
	m := map[string]interface{}{}
	for {
		if p >= len(d) {
			return m, p, fmt.Errorf("意外结束")
		}
		typ := d[p]
		p++
		if typ == vbEnd {
			return m, p, nil
		}
		key, np, err := vdfReadCString(d, p)
		if err != nil {
			return m, p, err
		}
		p = np
		switch typ {
		case vbMap:
			sub, np, err := vdfParse(d, p)
			if err != nil {
				return m, np, err
			}
			m[key] = sub
			p = np
		case vbString:
			s, np, err := vdfReadCString(d, p)
			if err != nil {
				return m, np, err
			}
			m[key] = s
			p = np
		case vbInt32:
			if p+4 > len(d) {
				return m, p, fmt.Errorf("截断")
			}
			m[key] = int(int32(binary.LittleEndian.Uint32(d[p : p+4])))
			p += 4
		default:
			return m, p, fmt.Errorf("未知类型 0x%x", typ)
		}
	}
}

func vdfLoad(b []byte) (map[string]interface{}, error) {
	m, _, err := vdfParse(b, 0)
	return m, err
}

// ---------- Steam 库自导入 ----------

// inFlatpak 判断是否运行在 flatpak 沙盒中
func inFlatpak() bool {
	_, err := os.Stat("/app/bin/spireforge")
	return err == nil
}

func steamSelfExe() (exe, launch, startdir string) {
	if inFlatpak() {
		return `"/usr/bin/flatpak"`, "run com.spireforge.SaveForge", "/var/home/friddle"
	}
	ex, err := os.Executable()
	if err != nil {
		ex = "./spireforge"
	}
	return `"` + ex + `"`, "", ""
}

// importToSteam 把本应用注册为 Steam 非 Steam 游戏
func importToSteam() (string, error) {
	home, _ := os.UserHomeDir()
	entries, _ := filepath.Glob(filepath.Join(home, ".local/share/Steam/userdata/*/config"))
	if len(entries) == 0 {
		return "", fmt.Errorf("未找到 Steam userdata 目录")
	}
	name := "尖塔工坊 SpireForge"
	exe, launch, startdir := steamSelfExe()
	appid := crcSigned(exe + name)
	icon := ""
	for _, c := range []string{
		"/var/home/friddle/Games/SpireForge/icon.png",
		home + "/Games/SpireForge/icon.png",
		"icon.png",
	} {
		if _, err := os.Stat(c); err == nil {
			icon = c
			break
		}
	}
	var result string
	for _, cfgDir := range entries {
		p := filepath.Join(cfgDir, "shortcuts.vdf")
		d := map[string]interface{}{"shortcuts": map[string]interface{}{}}
		if b, err := os.ReadFile(p); err == nil {
			if loaded, err := vdfLoad(b); err == nil {
				d = loaded
			}
		}
		scRaw, _ := d["shortcuts"].(map[string]interface{})
		if scRaw == nil {
			scRaw = map[string]interface{}{}
		}
		// 幂等: 删除同名旧条目
		for k, v := range scRaw {
			if vm, ok := v.(map[string]interface{}); ok {
				if vn, _ := vm["AppName"].(string); strings.Contains(vn, "尖塔工坊") {
					delete(scRaw, k)
				}
			}
		}
		idx := 0
		for k := range scRaw {
			var n int
			fmt.Sscanf(k, "%d", &n)
			if n >= idx {
				idx = n + 1
			}
		}
		scRaw[strconv.Itoa(idx)] = map[string]interface{}{
			"appid":               appid,
			"AppName":             name,
			"Exe":                 exe,
			"StartDir":            startdir,
			"icon":                icon,
			"ShortcutPath":        "",
			"LaunchOptions":       launch,
			"IsHidden":            0,
			"AllowDesktopConfig":  1,
			"AllowOverlay":        1,
			"OpenVR":              0,
			"Devkit":              0,
			"DevkitGameID":        "",
			"DevkitOverrideAppID": "",
			"LastPlayTime":        int(timeNow()),
		}
		d["shortcuts"] = scRaw
		if err := os.WriteFile(p, vdfDump(d), 0o644); err != nil {
			return "", err
		}
		result += filepath.Base(filepath.Dir(filepath.Dir(cfgDir))) + " "
	}
	return "已写入 Steam 库 (" + strings.TrimSpace(result) + "), 重启 Steam 生效", nil
}

func crcSigned(s string) int {
	var crc uint32 = 0xFFFFFFFF
	for _, c := range []byte(s) {
		crc ^= uint32(c)
		for i := 0; i < 8; i++ {
			if crc&1 != 0 {
				crc = (crc >> 1) ^ 0xEDB88320
			} else {
				crc >>= 1
			}
		}
	}
	v := (crc ^ 0xFFFFFFFF) | 0x80000000
	return int(int32(v))
}

func timeNow() int64 {
	return time.Now().Unix()
}
