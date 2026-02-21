package themepicker

import (
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Theme represents a color palette interface for the picker.
type Theme interface {
	DisplayName() string
	Desc() string

	// Colors for preview
	GetPrimary() lipgloss.Color
	GetSecondary() lipgloss.Color
	GetSuccess() lipgloss.Color
	GetWarning() lipgloss.Color
	GetError() lipgloss.Color
	GetMuted() lipgloss.Color
	GetBackground() lipgloss.Color
	GetForeground() lipgloss.Color
}

type ThemeSelectedMsg struct {
	Theme Theme
}

type ThemeLoadedMsg struct {
	Theme Theme
}

type ThemeFetcher func(name string) (Theme, error)

type Model struct {
	list     list.Model
	themes   []Theme
	selected Theme
	width    int
	height   int
	fetcher  ThemeFetcher
}

type item struct {
	theme Theme
}

func (i item) Title() string       { return i.theme.DisplayName() }
func (i item) Description() string { return i.theme.Desc() }
func (i item) FilterValue() string { return i.theme.DisplayName() }

func New(themes []Theme) Model {
	items := make([]list.Item, len(themes))
	for i, t := range themes {
		items[i] = item{theme: t}
	}

	l := list.New(items, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Select Theme"
	l.SetShowHelp(false)

	return Model{
		list:   l,
		themes: themes,
	}
}

func (m *Model) SetFetcher(f ThemeFetcher) {
	m.fetcher = f
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		// Split width: List gets 1/3, Preview gets 2/3
		listWidth := m.width / 3
		if listWidth < 30 {
			listWidth = 30
		}
		m.list.SetWidth(listWidth)
		m.list.SetHeight(m.height - 2) // Account for borders/padding

	case tea.KeyMsg:
		if m.list.FilterState() == list.Filtering {
			break
		}
		switch msg.String() {
		case "enter":
			if i, ok := m.list.SelectedItem().(item); ok {
				return m, func() tea.Msg { return ThemeSelectedMsg{Theme: i.theme} }
			}
		}

	case ThemeLoadedMsg:
		// Find and update the item
		items := m.list.Items()
		for idx, it := range items {
			if i, ok := it.(item); ok {
				if i.theme.DisplayName() == msg.Theme.DisplayName() {
					// Update item
					items[idx] = item{theme: msg.Theme}
					m.list.SetItems(items) // Trigger list update

					// If this was the selected item, update selection
					if m.selected != nil && m.selected.DisplayName() == msg.Theme.DisplayName() {
						m.selected = msg.Theme
					}
					return m, nil
				}
			}
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)

	// Update selected for preview and trigger lazy load
	if i, ok := m.list.SelectedItem().(item); ok {
		m.selected = i.theme

		// Lazy load if needed
		if _, isLazy := m.selected.(LazyTheme); isLazy && m.fetcher != nil {
			name := m.selected.DisplayName()
			return m, tea.Batch(cmd, func() tea.Msg {
				t, err := m.fetcher(name)
				if err != nil {
					// Optionally return error msg, for now ignore
					return nil
				}
				return ThemeLoadedMsg{Theme: t}
			})
		}
	}

	return m, cmd
}

func (m Model) View() string {
	if m.width == 0 {
		return "Initializing..."
	}

	listView := m.list.View()
	previewView := m.renderPreview()

	return lipgloss.JoinHorizontal(lipgloss.Top, listView, previewView)
}

func (m Model) renderPreview() string {
	if m.selected == nil {
		return ""
	}

	t := m.selected

	// Create styles based on the selected theme for preview
	primary := lipgloss.NewStyle().Foreground(t.GetPrimary())
	secondary := lipgloss.NewStyle().Foreground(t.GetSecondary())
	success := lipgloss.NewStyle().Foreground(t.GetSuccess())
	warning := lipgloss.NewStyle().Foreground(t.GetWarning())
	errStyle := lipgloss.NewStyle().Foreground(t.GetError())
	muted := lipgloss.NewStyle().Foreground(t.GetMuted())

	// Styles
	title := primary.Bold(true).Render
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.GetSecondary()).
		Padding(1).
		Width(m.width - m.list.Width() - 4). // Approx width
		Render

	var b strings.Builder

	b.WriteString(title("Preview: " + t.DisplayName()))
	b.WriteString("\n\n")

	// Color Palette
	b.WriteString("Palette:\n")
	b.WriteString(primary.Render("██ ") + "Primary\n")
	b.WriteString(secondary.Render("██ ") + "Secondary\n")
	b.WriteString(success.Render("██ ") + "Success\n")
	b.WriteString(warning.Render("██ ") + "Warning\n")
	b.WriteString(errStyle.Render("██ ") + "Error\n")
	b.WriteString(muted.Render("██ ") + "Muted\n")
	b.WriteString("\n")

	// Sample UI Elements
	b.WriteString(title("Task List Example"))
	b.WriteString("\n")
	b.WriteString(success.Render("[x]") + " Refactor authentication module\n")
	b.WriteString(primary.Render("[~]") + " Implement theme picker " + muted.Render("@jadb") + "\n")
	b.WriteString(lipgloss.NewStyle().Render("[ ]") + " Update documentation\n")
	b.WriteString(warning.Render("[-]") + " Fix race condition " + muted.Render("#bug") + "\n")

	b.WriteString("\n")
	b.WriteString(title("Logs Example"))
	b.WriteString("\n")
	b.WriteString(muted.Render("12:00:01") + " " + success.Render("SUCCESS") + " Build completed\n")
	b.WriteString(muted.Render("12:00:05") + " " + errStyle.Render("ERROR") + " Connection timeout\n")

	return box(b.String())
}
