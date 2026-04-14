package styles

import (
	"image/color"

	"charm.land/lipgloss/v2"
	kitcli "hop.top/kit/cli"
)

// DefaultTagColors are the ANSI color codes used for tag rendering
// when no custom tag colors are configured.
var DefaultTagColors = []string{
	"42", "214", "39", "196", "170", "208",
	"141", "81", "111", "118", "226", "201",
}

// Styles contains all TUI styles derived from a kit/cli.Theme.
type Styles struct {
	Title      lipgloss.Style
	Label      lipgloss.Style
	Muted      lipgloss.Style
	Warning    lipgloss.Style
	Error      lipgloss.Style
	Todo       lipgloss.Style
	InProgress lipgloss.Style
	Done       lipgloss.Style
	Skipped    lipgloss.Style
	Box        lipgloss.Style
	TagColors  []string

	// Raw colors for components that need them directly.
	PrimaryColor color.Color
	MutedColor   color.Color
}

// NewFromTheme creates Styles from a kit/cli.Theme.
func NewFromTheme(t kitcli.Theme) *Styles {
	return &Styles{
		Title:      lipgloss.NewStyle().Foreground(t.Accent).Bold(true),
		Label:      lipgloss.NewStyle().Foreground(t.Muted).Width(12),
		Muted:      lipgloss.NewStyle().Foreground(t.Muted),
		Warning:    lipgloss.NewStyle().Foreground(t.Secondary).Bold(true),
		Error:      lipgloss.NewStyle().Foreground(t.Error).Bold(true),
		Todo:       lipgloss.NewStyle(),
		InProgress: lipgloss.NewStyle().Foreground(t.Accent),
		Done:       lipgloss.NewStyle().Foreground(t.Success).Bold(true),
		Skipped:    lipgloss.NewStyle().Foreground(t.Secondary),
		Box: lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(t.Secondary).
			Padding(0, 1),
		TagColors:    DefaultTagColors,
		PrimaryColor: t.Accent,
		MutedColor:   t.Muted,
	}
}

// DefaultKitTheme returns the default kit theme for use in tests and
// as a fallback when no theme is provided.
func DefaultKitTheme() kitcli.Theme {
	return kitcli.Theme{
		Accent:    lipgloss.Color("39"),
		Secondary: lipgloss.Color("240"),
		Success:   lipgloss.Color("42"),
		Error:     lipgloss.Color("196"),
		Muted:     lipgloss.Color("241"),
		Title:     lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")),
		Subtle:    lipgloss.NewStyle().Foreground(lipgloss.Color("241")),
		Bold:      lipgloss.NewStyle().Bold(true),
	}
}
