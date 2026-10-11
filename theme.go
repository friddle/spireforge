// SpireForge 主题: 浅色/深色双模式
package main

import (
	"image/color"
	"os"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

var themeMode = "light" // light / dark

type palette struct {
	bg, fg, soft, accent      color.Color
	rowIdle, rowFocus, stroke color.Color
	inputBg, hover, selection color.Color
	ok, bad                   color.Color
}

var palLight = palette{
	bg:        &color.NRGBA{R: 255, G: 255, B: 255, A: 255},
	fg:        &color.NRGBA{R: 26, G: 26, B: 26, A: 255},
	soft:      &color.NRGBA{R: 138, G: 147, B: 166, A: 255},
	accent:    &color.NRGBA{R: 79, G: 107, B: 237, A: 255},
	rowIdle:   &color.NRGBA{R: 247, G: 248, B: 251, A: 255},
	rowFocus:  &color.NRGBA{R: 232, G: 238, B: 255, A: 255},
	stroke:    &color.NRGBA{R: 208, G: 213, B: 224, A: 255},
	inputBg:   &color.NRGBA{R: 255, G: 255, B: 255, A: 255},
	hover:     &color.NRGBA{R: 237, G: 240, B: 247, A: 255},
	selection: &color.NRGBA{R: 221, G: 229, B: 248, A: 255},
	ok:        &color.NRGBA{R: 24, G: 160, B: 88, A: 255},
	bad:       &color.NRGBA{R: 208, G: 48, B: 80, A: 255},
}

var palDark = palette{
	bg:        &color.NRGBA{R: 15, G: 17, B: 26, A: 255},
	fg:        &color.NRGBA{R: 232, G: 234, B: 242, A: 255},
	soft:      &color.NRGBA{R: 150, G: 156, B: 178, A: 255},
	accent:    &color.NRGBA{R: 124, G: 124, B: 255, A: 255},
	rowIdle:   &color.NRGBA{R: 27, G: 30, B: 44, A: 255},
	rowFocus:  &color.NRGBA{R: 46, G: 52, B: 88, A: 255},
	stroke:    &color.NRGBA{R: 62, G: 66, B: 92, A: 255},
	inputBg:   &color.NRGBA{R: 22, G: 25, B: 38, A: 255},
	hover:     &color.NRGBA{R: 30, G: 34, B: 51, A: 255},
	selection: &color.NRGBA{R: 46, G: 52, B: 88, A: 255},
	ok:        &color.NRGBA{R: 96, G: 211, B: 148, A: 255},
	bad:       &color.NRGBA{R: 240, G: 113, B: 120, A: 255},
}

func curPal() palette {
	if themeMode == "dark" {
		return palDark
	}
	return palLight
}

func themeFile() string {
	if d, err := os.UserConfigDir(); err == nil {
		return filepath.Join(d, "spireforge", "theme")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config/spireforge/theme")
}

func loadThemePref() {
	if b, err := os.ReadFile(themeFile()); err == nil && strings.TrimSpace(string(b)) == "dark" {
		themeMode = "dark"
	}
}

func saveThemePref() {
	_ = os.MkdirAll(filepath.Dir(themeFile()), 0o755)
	_ = os.WriteFile(themeFile(), []byte(themeMode), 0o644)
}

type spireTheme struct {
	fyne.Theme
}

func (t spireTheme) Color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	p := curPal()
	switch n {
	case theme.ColorNameBackground:
		return p.bg
	case theme.ColorNameForeground:
		return p.fg
	case theme.ColorNamePrimary:
		return p.accent
	case theme.ColorNameHover:
		return p.hover
	case theme.ColorNameDisabled:
		return p.soft
	case theme.ColorNameInputBackground:
		return p.inputBg
	case theme.ColorNameInputBorder:
		return p.accent
	case theme.ColorNameSelection:
		return p.selection
	case theme.ColorNameSuccess:
		return p.ok
	case theme.ColorNameError:
		return p.bad
	default:
		return t.Theme.Color(n, v)
	}
}

func (t spireTheme) Size(n fyne.ThemeSizeName) float32 {
	if n == theme.SizeNameText {
		return 15
	}
	return t.Theme.Size(n)
}
