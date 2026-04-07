package themepicker

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// BasicTheme is a concrete implementation of Theme.
type BasicTheme struct {
	NameVal       string
	DescVal       string
	PrimaryVal    color.Color
	SecondaryVal  color.Color
	SuccessVal    color.Color
	WarningVal    color.Color
	ErrorVal      color.Color
	MutedVal      color.Color
	BackgroundVal color.Color
	ForegroundVal color.Color
}

func (t BasicTheme) DisplayName() string { return t.NameVal }
func (t BasicTheme) Desc() string        { return t.DescVal }

func (t BasicTheme) GetPrimary() color.Color    { return t.PrimaryVal }
func (t BasicTheme) GetSecondary() color.Color  { return t.SecondaryVal }
func (t BasicTheme) GetSuccess() color.Color    { return t.SuccessVal }
func (t BasicTheme) GetWarning() color.Color    { return t.WarningVal }
func (t BasicTheme) GetError() color.Color      { return t.ErrorVal }
func (t BasicTheme) GetMuted() color.Color      { return t.MutedVal }
func (t BasicTheme) GetBackground() color.Color { return t.BackgroundVal }
func (t BasicTheme) GetForeground() color.Color { return t.ForegroundVal }

// LazyTheme represents a theme that needs to be loaded.
type LazyTheme struct {
	Name string
}

func (t LazyTheme) DisplayName() string { return t.Name }
func (t LazyTheme) Desc() string        { return "Loading..." }

// Default colors for unloaded theme.
func (t LazyTheme) GetPrimary() color.Color    { return lipgloss.Color("240") }
func (t LazyTheme) GetSecondary() color.Color  { return lipgloss.Color("240") }
func (t LazyTheme) GetSuccess() color.Color    { return lipgloss.Color("240") }
func (t LazyTheme) GetWarning() color.Color    { return lipgloss.Color("240") }
func (t LazyTheme) GetError() color.Color      { return lipgloss.Color("240") }
func (t LazyTheme) GetMuted() color.Color      { return lipgloss.Color("240") }
func (t LazyTheme) GetBackground() color.Color { return lipgloss.Color("") }
func (t LazyTheme) GetForeground() color.Color { return lipgloss.Color("") }
