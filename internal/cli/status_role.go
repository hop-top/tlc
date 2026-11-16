package cli

import (
	"strings"

	lipgloss "charm.land/lipgloss/v2"

	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/core"
)

// statusRole resolves a task status to the role its definition declares,
// "" when the vocabulary does not declare the status. Rendering decisions
// key on role, never on a status name, so a renamed or custom vocabulary
// renders like the default one.
func statusRole(status core.TaskStatus) string {
	def, err := core.DefaultWorkflow().GetStatusDef(status)
	if err != nil {
		return ""
	}
	return def.Role
}

// dotColorForRole is the GraphViz fill color for a status role.
func dotColorForRole(role string) string {
	switch role {
	case config.RoleInitial:
		return "#ffffff"
	case config.RoleBacklog:
		return "#e0e0e0"
	case config.RoleActive:
		return "#aee4f8"
	case config.RoleReview:
		return "#e3c4f0"
	case config.RoleBlocked:
		return "#f4a6a6"
	case config.RoleCompleted:
		return "#a8e6a3"
	case config.RoleSkipped:
		return "#f5dfa5"
	default:
		return "#eeeeee"
	}
}

// roleStyleFor picks the terminal style for a status role. Roles without
// a dedicated style render with the neutral one.
func roleStyleFor(role string) lipgloss.Style {
	switch role {
	case config.RoleActive:
		return inProgressStyle
	case config.RoleReview:
		return reviewStyle
	case config.RoleBlocked:
		return blockedStyle
	case config.RoleBacklog:
		return backlogStyle
	case config.RoleCompleted:
		return doneStyle
	case config.RoleSkipped:
		return skippedStyle
	default:
		return todoStyle
	}
}

// isTerminalRole reports whether a role names a finished status.
func isTerminalRole(role string) bool {
	return role == config.RoleCompleted || role == config.RoleSkipped
}

// statusColorNames translates the color names a status definition
// declares into ANSI-256 codes. A lipgloss color built from a bare name
// such as "yellow" is not a valid color and renders unstyled, so names
// resolve here; hex and numeric values pass through untouched.
var statusColorNames = map[string]string{
	"black":   "0",
	"red":     "196",
	"green":   "42",
	"yellow":  "220",
	"blue":    "39",
	"magenta": "170",
	"cyan":    "45",
	"white":   "15",
	"gray":    "245",
	"grey":    "245",
}

// statusColor resolves a status definition's color to a lipgloss color.
func statusColor(declared string) lipgloss.Color {
	if code, ok := statusColorNames[strings.ToLower(strings.TrimSpace(declared))]; ok {
		return lipgloss.Color(code)
	}
	return lipgloss.Color(declared)
}
