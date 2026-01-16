package styles

import "github.com/charmbracelet/lipgloss"

var (
	PrimaryColor = lipgloss.Color("39")
	SuccessColor = lipgloss.Color("42")
	WarningColor = lipgloss.Color("214")
	ErrorColor   = lipgloss.Color("196")
	MutedColor   = lipgloss.Color("241")

	TitleStyle = lipgloss.NewStyle().Foreground(PrimaryColor).Bold(true)
	LabelStyle = lipgloss.NewStyle().Foreground(MutedColor).Width(12)
	MutedStyle = lipgloss.NewStyle().Foreground(MutedColor)
	ErrorStyle = lipgloss.NewStyle().Foreground(ErrorColor).Bold(true)

	TodoStyle       = lipgloss.NewStyle()
	InProgressStyle = lipgloss.NewStyle().Foreground(PrimaryColor)
	DoneStyle       = lipgloss.NewStyle().Foreground(SuccessColor).Bold(true)
	SkippedStyle    = lipgloss.NewStyle().Foreground(WarningColor)

	BoxStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1)

	TagColors = []string{
		"42",  // Green
		"214", // Orange
		"39",  // Blue
		"196", // Red
		"170", // Pink/Purple
		"208", // Dark Orange
		"141", // Purple
		"81",  // Light Blue
		"111", // Sky Blue
		"118", // Lime
		"226", // Yellow
		"201", // Magenta
	}
)
