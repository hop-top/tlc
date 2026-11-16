package tui

import (
	"fmt"
	"strings"

	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/core"
)

// kanbanDropOrder is the order roles leave a board that is too narrow
// for its columns. The active role is never in it: the board always
// keeps the work that is moving.
var kanbanDropOrder = []string{
	config.RoleCompleted,
	config.RoleInitial,
	config.RoleBlocked,
	config.RoleReview,
}

// kanbanCapacity is how many columns fit at the minimum readable width.
// A width that is not yet known (zero before the first resize) fits
// everything, so nothing is hidden on a guess.
func kanbanCapacity(width int) int {
	if width <= 0 {
		return int(^uint(0) >> 1)
	}
	c := (width - 4) / kanbanMinColWidth
	if c < 1 {
		c = 1
	}
	return c
}

// kanbanLayout decides which columns a board of the given width shows.
//
// Fewer columns, never narrower ones and never a scroll. A board that
// fits shows everything. Otherwise columns with no tasks go first; if
// that is not enough, roles are dropped in kanbanDropOrder, with roles
// outside that order and the active role handled so a custom vocabulary
// loses its unknown roles before the well-known ones, and the active role
// last of all.
//
// shown keeps board order. hidden is in the order the columns were
// dropped, which is the order the hint names them in.
func kanbanLayout(
	columns []core.TaskStatus,
	counts map[core.TaskStatus]int,
	roleOf func(core.TaskStatus) string,
	width int,
) (shown, hidden []core.TaskStatus) {
	capacity := kanbanCapacity(width)
	if len(columns) <= capacity {
		return columns, nil
	}

	keep := make([]bool, len(columns))
	for i := range keep {
		keep[i] = true
	}
	remaining := len(columns)
	drop := func(i int) {
		keep[i] = false
		hidden = append(hidden, columns[i])
		remaining--
	}

	// Step 1: columns with no tasks. A board of nothing but empty
	// columns skips this step rather than render no board at all.
	empty := 0
	for _, s := range columns {
		if counts[s] == 0 {
			empty++
		}
	}
	if empty < len(columns) {
		for i, s := range columns {
			if counts[s] == 0 {
				drop(i)
			}
		}
	}

	// rightmost returns the rightmost kept column whose role satisfies
	// match, or -1.
	rightmost := func(match func(role string) bool) int {
		for i := len(columns) - 1; i >= 0; i-- {
			if keep[i] && match(roleOf(columns[i])) {
				return i
			}
		}
		return -1
	}
	known := func(role string) bool {
		if role == config.RoleActive {
			return true
		}
		for _, r := range kanbanDropOrder {
			if r == role {
				return true
			}
		}
		return false
	}

	// Step 2: by role. Roles outside the known set go first, then the
	// fixed order, then (still too wide) anything but the active role,
	// then active columns rightmost-first, keeping at least one.
	for remaining > capacity {
		i := rightmost(func(r string) bool { return !known(r) })
		for _, role := range kanbanDropOrder {
			if i >= 0 {
				break
			}
			i = rightmost(func(r string) bool { return r == role })
		}
		if i < 0 {
			i = rightmost(func(string) bool { return true })
		}
		if i < 0 || remaining == 1 {
			break
		}
		drop(i)
	}

	shown = make([]core.TaskStatus, 0, remaining)
	for i, s := range columns {
		if keep[i] {
			shown = append(shown, s)
		}
	}
	return shown, hidden
}

// kanbanHint is the one line naming hidden columns, or "" when none are.
func kanbanHint(hidden []core.TaskStatus, counts map[core.TaskStatus]int) string {
	if len(hidden) == 0 {
		return ""
	}
	parts := make([]string, len(hidden))
	for i, s := range hidden {
		parts[i] = fmt.Sprintf("%s (%d)", s, counts[s])
	}
	return "hidden: " + strings.Join(parts, ", ") + "; widen terminal to show"
}

// kanbanColumns is the board's visible columns plus the hint line for
// the current width and task set.
func (m Model) kanbanColumns() (shown []core.TaskStatus, hint string) {
	counts := make(map[core.TaskStatus]int)
	for _, t := range m.tasks {
		counts[t.Status]++
	}
	wm := core.DefaultWorkflow()
	roleOf := func(s core.TaskStatus) string {
		if def, err := wm.GetStatusDef(s); err == nil {
			return def.Role
		}
		return ""
	}
	shown, hidden := kanbanLayout(kanbanStatusOrder(), counts, roleOf, m.width)
	return shown, kanbanHint(hidden, counts)
}
