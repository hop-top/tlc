package themepicker

import "github.com/charmbracelet/lipgloss"

// BasicTheme is a concrete implementation of Theme.
type BasicTheme struct {
	NameVal       string
	DescVal       string
	PrimaryVal    lipgloss.Color
	SecondaryVal  lipgloss.Color
	SuccessVal    lipgloss.Color
	WarningVal    lipgloss.Color
	ErrorVal      lipgloss.Color
	MutedVal      lipgloss.Color
	BackgroundVal lipgloss.Color
	ForegroundVal lipgloss.Color
}

func (t BasicTheme) DisplayName() string { return t.NameVal }
func (t BasicTheme) Desc() string        { return t.DescVal }

func (t BasicTheme) GetPrimary() lipgloss.Color    { return t.PrimaryVal }
func (t BasicTheme) GetSecondary() lipgloss.Color  { return t.SecondaryVal }
func (t BasicTheme) GetSuccess() lipgloss.Color    { return t.SuccessVal }
func (t BasicTheme) GetWarning() lipgloss.Color    { return t.WarningVal }
func (t BasicTheme) GetError() lipgloss.Color      { return t.ErrorVal }
func (t BasicTheme) GetMuted() lipgloss.Color      { return t.MutedVal }
func (t BasicTheme) GetBackground() lipgloss.Color { return t.BackgroundVal }
func (t BasicTheme) GetForeground() lipgloss.Color { return t.ForegroundVal }

// LazyTheme represents a theme that needs to be loaded.
type LazyTheme struct {
	Name string
}

func (t LazyTheme) DisplayName() string { return t.Name }
func (t LazyTheme) Desc() string        { return "Loading..." }

// Default colors for unloaded theme.
func (t LazyTheme) GetPrimary() lipgloss.Color    { return lipgloss.Color("240") }
func (t LazyTheme) GetSecondary() lipgloss.Color  { return lipgloss.Color("240") }
func (t LazyTheme) GetSuccess() lipgloss.Color    { return lipgloss.Color("240") }
func (t LazyTheme) GetWarning() lipgloss.Color    { return lipgloss.Color("240") }
func (t LazyTheme) GetError() lipgloss.Color      { return lipgloss.Color("240") }
func (t LazyTheme) GetMuted() lipgloss.Color      { return lipgloss.Color("240") }
func (t LazyTheme) GetBackground() lipgloss.Color { return lipgloss.Color("") }
func (t LazyTheme) GetForeground() lipgloss.Color { return lipgloss.Color("") }
