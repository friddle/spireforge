// SpireForge 自定义深色主题
package main

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

var (
	colAccent   color.Color = &color.NRGBA{R: 79, G: 107, B: 237, A: 255}  // 现代蓝
	colBgDeep   color.Color = &color.NRGBA{R: 255, G: 255, B: 255, A: 255} // 纯白
	colTextSoft color.Color = &color.NRGBA{R: 138, G: 147, B: 166, A: 255} // 灰
	colOK       color.Color = &color.NRGBA{R: 24, G: 160, B: 88, A: 255}
	colBad      color.Color = &color.NRGBA{R: 208, G: 48, B: 80, A: 255}
)

type spireTheme struct {
	fyne.Theme
}

func (t spireTheme) Color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	switch n {
	case theme.ColorNameBackground:
		return colBgDeep
	case theme.ColorNameForeground:
		return &color.NRGBA{R: 26, G: 26, B: 26, A: 255} // 黑色文字
	case theme.ColorNamePrimary:
		return colAccent
	case theme.ColorNameHover:
		return &color.NRGBA{R: 237, G: 240, B: 247, A: 255}
	case theme.ColorNameDisabled:
		return &color.NRGBA{R: 154, G: 163, B: 181, A: 255}
	case theme.ColorNameInputBackground:
		return &color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	case theme.ColorNameInputBorder:
		return colAccent
	case theme.ColorNameSelection:
		return &color.NRGBA{R: 221, G: 229, B: 248, A: 255}
	case theme.ColorNameSuccess:
		return colOK
	case theme.ColorNameError:
		return colBad
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
