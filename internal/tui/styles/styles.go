package styles

import "github.com/charmbracelet/lipgloss"

// Theme defines the color palette
type Theme struct {
	Name       string
	Primary    lipgloss.Color
	Secondary  lipgloss.Color
	Success    lipgloss.Color
	Warning    lipgloss.Color
	Error      lipgloss.Color
	Muted      lipgloss.Color
	Background lipgloss.Color
	Foreground lipgloss.Color
	TagColors  []string
}

// Methods to satisfy themepicker.Theme interface
func (t Theme) DisplayName() string       { return t.Name }
func (t Theme) Desc() string              { return "Internal Theme" }
func (t Theme) GetPrimary() lipgloss.Color    { return t.Primary }
func (t Theme) GetSecondary() lipgloss.Color  { return t.Secondary }
func (t Theme) GetSuccess() lipgloss.Color    { return t.Success }
func (t Theme) GetWarning() lipgloss.Color    { return t.Warning }
func (t Theme) GetError() lipgloss.Color      { return t.Error }
func (t Theme) GetMuted() lipgloss.Color      { return t.Muted }
func (t Theme) GetBackground() lipgloss.Color { return t.Background }
func (t Theme) GetForeground() lipgloss.Color { return t.Foreground }

// Styles contains all application styles derived from a Theme
type Styles struct {
	Title           lipgloss.Style
	Label           lipgloss.Style
	Muted           lipgloss.Style
	Error           lipgloss.Style
	Todo            lipgloss.Style
	InProgress      lipgloss.Style
	Done            lipgloss.Style
	Skipped         lipgloss.Style
	Box             lipgloss.Style
	TagColors       []string
}

// DefaultTheme returns the built-in default theme
func DefaultTheme() Theme {
	return Theme{
		Name:       "Default",
		Primary:    lipgloss.Color("39"),
		Secondary:  lipgloss.Color("240"),
		Success:    lipgloss.Color("42"),
		Warning:    lipgloss.Color("214"),
		Error:      lipgloss.Color("196"),
		Muted:      lipgloss.Color("241"),
		Background: lipgloss.Color(""), // Default terminal bg
		Foreground: lipgloss.Color(""), // Default terminal fg
		TagColors: []string{
			"42", "214", "39", "196", "170", "208", "141", "81", "111", "118", "226", "201",
		},
	}
}

// NewStyles creates a new Styles struct from a Theme
func NewStyles(t Theme) *Styles {
	return &Styles{
		Title:      lipgloss.NewStyle().Foreground(t.Primary).Bold(true),
		Label:      lipgloss.NewStyle().Foreground(t.Muted).Width(12),
		Muted:      lipgloss.NewStyle().Foreground(t.Muted),
		Error:      lipgloss.NewStyle().Foreground(t.Error).Bold(true),
		Todo:       lipgloss.NewStyle(), // Inherits default fg/bg
		InProgress: lipgloss.NewStyle().Foreground(t.Primary),
		Done:       lipgloss.NewStyle().Foreground(t.Success).Bold(true),
		Skipped:    lipgloss.NewStyle().Foreground(t.Warning),
		Box: lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(t.Secondary).
			Padding(0, 1),
		TagColors: t.TagColors,
	}
}

// Global instance for backward compatibility (optional, or we can refactor usage)
var Current = NewStyles(DefaultTheme())

// Deprecated: Use Current.Primary instead (kept for now to avoid breaking build immediately)
var (
	PrimaryColor = DefaultTheme().Primary
	SuccessColor = DefaultTheme().Success
	WarningColor = DefaultTheme().Warning
	ErrorColor   = DefaultTheme().Error
	MutedColor   = DefaultTheme().Muted

	TitleStyle      = Current.Title
	LabelStyle      = Current.Label
	MutedStyle      = Current.Muted
	ErrorStyle      = Current.Error
	TodoStyle       = Current.Todo
	InProgressStyle = Current.InProgress
	DoneStyle       = Current.Done
	SkippedStyle    = Current.Skipped
	BoxStyle        = Current.Box
	TagColors       = Current.TagColors
)

func ApplyTheme(t Theme) {
	Current = NewStyles(t)
	
	PrimaryColor = t.Primary
	SuccessColor = t.Success
	WarningColor = t.Warning
	ErrorColor = t.Error
	MutedColor = t.Muted
	
	TitleStyle = Current.Title
	LabelStyle = Current.Label
	MutedStyle = Current.Muted
	ErrorStyle = Current.Error
	TodoStyle = Current.Todo
	InProgressStyle = Current.InProgress
	DoneStyle = Current.Done
	SkippedStyle = Current.Skipped
	BoxStyle = Current.Box
	TagColors = Current.TagColors
}

