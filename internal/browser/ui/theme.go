// Package ui implements the Shard browser user interface using Fyne.
package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// ShardTheme is the Chrome-inspired Shard browser theme.
type ShardTheme struct{}

var _ fyne.Theme = (*ShardTheme)(nil)

func (t *ShardTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	switch name {
	case theme.ColorNameBackground:
		return color.RGBA{R: 0xF1, G: 0xF3, B: 0xF4, A: 0xFF} // Chrome toolbar gray
	case theme.ColorNameForeground:
		return color.RGBA{R: 0x20, G: 0x21, B: 0x24, A: 0xFF} // dark text
	case theme.ColorNameInputBackground:
		return color.RGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF} // white input
	case theme.ColorNameButton:
		return color.RGBA{R: 0xF1, G: 0xF3, B: 0xF4, A: 0xFF}
	case theme.ColorNameDisabledButton:
		return color.RGBA{R: 0xE0, G: 0xE0, B: 0xE0, A: 0xFF}
	case theme.ColorNamePlaceHolder:
		return color.RGBA{R: 0x9A, G: 0x9A, B: 0x9A, A: 0xFF}
	case theme.ColorNamePrimary:
		return color.RGBA{R: 0x1A, G: 0x73, B: 0xE8, A: 0xFF} // Google blue
	case theme.ColorNameHover:
		return color.RGBA{R: 0xE8, G: 0xEA, B: 0xED, A: 0xFF}
	case theme.ColorNameFocus:
		return color.RGBA{R: 0x1A, G: 0x73, B: 0xE8, A: 0x40}
	case theme.ColorNameScrollBar:
		return color.RGBA{R: 0xC0, G: 0xC0, B: 0xC0, A: 0xFF}
	case theme.ColorNameShadow:
		return color.RGBA{R: 0x00, G: 0x00, B: 0x00, A: 0x20}
	case theme.ColorNameSeparator:
		return color.RGBA{R: 0xDA, G: 0xDC, B: 0xE0, A: 0xFF}
	}
	return theme.DefaultTheme().Color(name, variant)
}

func (t *ShardTheme) Font(style fyne.TextStyle) fyne.Resource {
	return theme.DefaultTheme().Font(style)
}

func (t *ShardTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(name)
}

func (t *ShardTheme) Size(name fyne.ThemeSizeName) float32 {
	switch name {
	case theme.SizeNamePadding:
		return 4
	case theme.SizeNameInnerPadding:
		return 8
	case theme.SizeNameText:
		return 13
	case theme.SizeNameHeadingText:
		return 20
	case theme.SizeNameSubHeadingText:
		return 15
	case theme.SizeNameCaptionText:
		return 11
	case theme.SizeNameScrollBar:
		return 6
	case theme.SizeNameScrollBarSmall:
		return 3
	case theme.SizeNameInputBorder:
		return 1
	case theme.SizeNameSeparatorThickness:
		return 1
	}
	return theme.DefaultTheme().Size(name)
}

// Chrome-like color palette
var (
	ChromeGray     = color.RGBA{R: 0xF1, G: 0xF3, B: 0xF4, A: 0xFF}
	ChromeTabBG    = color.RGBA{R: 0xDE, G: 0xE1, B: 0xE6, A: 0xFF}
	ChromeActiveTab = color.RGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}
	ChromeBlue     = color.RGBA{R: 0x1A, G: 0x73, B: 0xE8, A: 0xFF}
	ChromeDark     = color.RGBA{R: 0x20, G: 0x21, B: 0x24, A: 0xFF}
	ChromeMedGray  = color.RGBA{R: 0x5F, G: 0x63, B: 0x68, A: 0xFF}
	ChromeLightGray = color.RGBA{R: 0xDA, G: 0xDC, B: 0xE0, A: 0xFF}
	ChromeHover    = color.RGBA{R: 0xE8, G: 0xEA, B: 0xED, A: 0xFF}
	ChromeRed      = color.RGBA{R: 0xEA, G: 0x43, B: 0x35, A: 0xFF}
	ChromeGreen    = color.RGBA{R: 0x34, G: 0xA8, B: 0x53, A: 0xFF}
	ChromeYellow   = color.RGBA{R: 0xFB, G: 0xBC, B: 0x05, A: 0xFF}
)
