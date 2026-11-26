package tui

import (
	"testing"

	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/tui/styles"
)

// TestStatusRoleStyle_NewRolesWearTheirOwnStyle pins that backlog,
// review and blocked statuses resolve to their own theme styles instead
// of falling through to the neutral initial-status style.
func TestStatusRoleStyle_NewRolesWearTheirOwnStyle(t *testing.T) {
	s := styles.NewFromTheme(styles.DefaultKitTheme())
	if got := statusRoleStyle(config.RoleBlocked, s); got.GetForeground() != s.Blocked.GetForeground() {
		t.Errorf("blocked role did not use the Blocked style")
	}
	if got := statusRoleStyle(config.RoleReview, s); got.GetForeground() != s.Review.GetForeground() ||
		got.GetItalic() != s.Review.GetItalic() {
		t.Errorf("review role did not use the Review style")
	}
	if got := statusRoleStyle(config.RoleBacklog, s); got.GetForeground() != s.Backlog.GetForeground() {
		t.Errorf("backlog role did not use the Backlog style")
	}
	if statusRoleStyle(config.RoleBlocked, s).GetForeground() == s.Todo.GetForeground() {
		t.Errorf("blocked role fell through to the neutral style")
	}
}
