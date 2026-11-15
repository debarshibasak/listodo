package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

var (
	ink   = color.NRGBA{R: 35, G: 48, B: 45, A: 255}
	muted = color.NRGBA{R: 117, G: 127, B: 121, A: 255}
	green = color.NRGBA{R: 48, G: 110, B: 88, A: 255}
	line  = color.NRGBA{R: 227, G: 231, B: 224, A: 255}
	paper = color.NRGBA{R: 249, G: 250, B: 246, A: 255}
	white = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	red   = color.NRGBA{R: 177, G: 75, B: 56, A: 255}
)

type appTheme struct{}

type compactTheme struct{ appTheme }

func (compactTheme) Size(name fyne.ThemeSizeName) float32 {
	if name == theme.SizeNameInnerPadding {
		return 2
	}
	return appTheme{}.Size(name)
}

func (appTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	switch name {
	case theme.ColorNameBackground:
		return paper
	case theme.ColorNameForeground:
		return ink
	case theme.ColorNamePrimary, theme.ColorNameFocus:
		return green
	case theme.ColorNameButton:
		return color.NRGBA{R: 238, G: 242, B: 234, A: 255}
	case theme.ColorNameInputBackground, theme.ColorNameMenuBackground, theme.ColorNameOverlayBackground:
		return white
	case theme.ColorNamePlaceHolder, theme.ColorNameDisabled:
		return muted
	case theme.ColorNameSeparator, theme.ColorNameInputBorder:
		return line
	case theme.ColorNameSelection:
		return color.NRGBA{R: 217, G: 235, B: 222, A: 255}
	case theme.ColorNameError:
		return red
	}
	return theme.DefaultTheme().Color(name, theme.VariantLight)
}
func (appTheme) Font(style fyne.TextStyle) fyne.Resource    { return theme.DefaultTheme().Font(style) }
func (appTheme) Icon(name fyne.ThemeIconName) fyne.Resource { return theme.DefaultTheme().Icon(name) }
func (appTheme) Size(name fyne.ThemeSizeName) float32 {
	switch name {
	case theme.SizeNameText:
		return 14
	case theme.SizeNamePadding:
		return 8
	case theme.SizeNameInnerPadding:
		return 10
	case theme.SizeNameInputRadius, theme.SizeNameSelectionRadius:
		return 7
	}
	return theme.DefaultTheme().Size(name)
}

func text(s string, size float32, c color.Color, bold bool) *canvas.Text {
	t := canvas.NewText(s, c)
	t.TextSize = size
	t.TextStyle.Bold = bold
	return t
}
func wrap(s string) *widget.Label { l := widget.NewLabel(s); l.Wrapping = fyne.TextWrapWord; return l }
func small(s string) *canvas.Text { return text(s, 12, muted, false) }
func caption(s string, colorName fyne.ThemeColorName) *widget.RichText {
	r := widget.NewRichText(&widget.TextSegment{Text: s, Style: widget.RichTextStyle{Inline: true, SizeName: theme.SizeNameCaptionText, ColorName: colorName}})
	r.Wrapping = fyne.TextWrapWord
	return r
}
func pad(o fyne.CanvasObject, n float32) *fyne.Container {
	return container.New(layout.NewCustomPaddedLayout(n, n, n, n), o)
}
func card(o fyne.CanvasObject) *fyne.Container {
	r := canvas.NewRectangle(white)
	r.CornerRadius = 10
	r.StrokeColor = line
	r.StrokeWidth = 1
	return container.NewStack(r, pad(o, 16))
}
func pill(s string, c color.Color) *fyne.Container {
	r := canvas.NewRectangle(color.NRGBA{R: 238, G: 243, B: 235, A: 255})
	r.CornerRadius = 5
	return container.NewStack(r, container.New(layout.NewCustomPaddedLayout(4, 4, 8, 8), text(s, 11, c, true)))
}
func fixedWidth(o fyne.CanvasObject, width float32) *fyne.Container {
	r := canvas.NewRectangle(color.Transparent)
	r.SetMinSize(fyne.NewSize(width, 1))
	return container.NewStack(r, o)
}
func spacer(h float32) *canvas.Rectangle {
	r := canvas.NewRectangle(color.Transparent)
	r.SetMinSize(fyne.NewSize(1, h))
	return r
}
