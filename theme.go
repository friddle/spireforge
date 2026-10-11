// SpireForge 自定义深色主题
package main

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

var (
	colAccent   color.Color = &color.NRGBA{R: 124, G: 124, B: 255, A: 255}
	colBgDeep   color.Color = &color.NRGBA{R: 15, G: 17, B: 26, A: 255}
	colTextSoft color.Color = &color.NRGBA{R: 150, G: 156, B: 178, A: 255}
	colOK       color.Color = &color.NRGBA{R: 96, G: 211, B: 148, A: 255}
	colBad      color.Color = &color.NRGBA{R: 240, G: 113, B: 120, A: 255}
)

type spireTheme struct {
	fyne.Theme
}

func (t spireTheme) Color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	switch n {
	case theme.ColorNameBackground:
		return colBgDeep
	case theme.ColorNameForeground:
		return &color.NRGBA{R: 232, G: 234, B: 242, A: 255}
	case theme.ColorNamePrimary:
		return colAccent
	case theme.ColorNameHover:
		return &color.NRGBA{R: 60, G: 64, B: 96, A: 255}
	case theme.ColorNameDisabled:
		return &color.NRGBA{R: 110, G: 114, B: 134, A: 255}
	case theme.ColorNameInputBackground:
		return &color.NRGBA{R: 22, G: 25, B: 38, A: 255}
	case theme.ColorNameInputBorder:
		return colAccent
	case theme.ColorNameSelection:
		return &color.NRGBA{R: 60, G: 60, B: 120, A: 255}
	default:
		return t.Theme.Color(n, v)
	}
}

func (t spireTheme) Size(n fyne.ThemeSizeName) float32 {
	switch n {
	case theme.SizeNameText:
		return 15
	default:
		return t.Theme.Size(n)
	}
}
