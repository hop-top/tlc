package cli

import (
	"fmt"
	"testing"

	lipgloss "charm.land/lipgloss/v2"

	"hop.top/tlc/internal/config"
)

// TestDotColorForRole pins one distinct fill per default role; the
// graph reads status at a glance.
func TestDotColorForRole(t *testing.T) {
	seen := map[string]string{}
	for _, role := range []string{
		config.RoleBacklog, config.RoleInitial, config.RoleActive,
		config.RoleReview, config.RoleBlocked, config.RoleCompleted,
		config.RoleSkipped,
	} {
		c := dotColorForRole(role)
		if prev, dup := seen[c]; dup {
			t.Errorf("roles %s and %s share fill %s", prev, role, c)
		}
		seen[c] = role
	}
	if got := dotColorForRole("no-such-role"); got != "#eeeeee" {
		t.Errorf("unknown role fill = %s, want #eeeeee", got)
	}
}

// TestStatusColor_NamesResolve pins that declared color names resolve to
// ANSI codes rather than passing through as unusable bare names.
func TestStatusColor_NamesResolve(t *testing.T) {
	for _, name := range []string{"gray", "yellow", "blue", "magenta", "red", "green"} {
		if got := fmt.Sprint(statusColor(name)); got == name {
			t.Errorf("statusColor(%q) passed the name through", name)
		}
	}
	if got := fmt.Sprint(statusColor("#00ff00")); got != fmt.Sprint(lipgloss.Color("#00ff00")) {
		t.Errorf("hex color altered: %s", got)
	}
}

// TestRoleStyleFor_NewRolesAreStyled pins that backlog, review and
// blocked do not fall back to the neutral style.
func TestRoleStyleFor_NewRolesAreStyled(t *testing.T) {
	neutral := roleStyleFor("")
	for _, role := range []string{config.RoleBacklog, config.RoleReview, config.RoleBlocked} {
		if roleStyleFor(role).GetForeground() == neutral.GetForeground() {
			t.Errorf("role %s renders with the neutral style", role)
		}
	}
}

func TestIsTerminalRole(t *testing.T) {
	for role, want := range map[string]bool{
		config.RoleCompleted: true, config.RoleSkipped: true,
		config.RoleBlocked: false, config.RoleActive: false, "": false,
	} {
		if got := isTerminalRole(role); got != want {
			t.Errorf("isTerminalRole(%q) = %v, want %v", role, got, want)
		}
	}
}
