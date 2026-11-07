package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

type compactTheme struct{ fyne.Theme }

func newCompactTheme() fyne.Theme { return compactTheme{theme.DefaultTheme()} }

func (t compactTheme) Size(name fyne.ThemeSizeName) float32 {
	switch name {
	case theme.SizeNameText:
		return 13
	case theme.SizeNameCaptionText:
		return 11
	case theme.SizeNamePadding:
		return 3
	case theme.SizeNameInnerPadding:
		return 5
	case theme.SizeNameLineSpacing:
		return 3
	case theme.SizeNameInlineIcon:
		return 16
	case theme.SizeNameInputRadius, theme.SizeNameSelectionRadius:
		return 4
	}
	return t.Theme.Size(name)
}
