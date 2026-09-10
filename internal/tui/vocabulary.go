package tui

// The TUI's status vocabulary, resolved from config rather than
// enumerated in source.
//
// Every function here answers a question the TUI used to answer with a
// literal list of the built-in four statuses. Those literals were not
// one constant standing in for a role — they were WHOLE-VOCABULARY
// assumptions, so a config declaring any other set left the TUI with
// no column, no rank, no glyph and, worst, a rotate ring targeting a
// status the workflow never declared.
//
// Declaration order IS rank order, the same convention the priority and
// effort vocabularies use, so `GetAllStatuses` is the single source for
// both grouping order and sort rank.

import (
	"charm.land/lipgloss/v2"

	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/tui/styles"
)

// configuredStatusOrder returns every configured status in declared
// order. Used for dashboard grouping and as the basis of the sort rank.
func configuredStatusOrder() []core.TaskStatus {
	names := core.ConfiguredTaskStatusStrings()
	order := make([]core.TaskStatus, 0, len(names))
	for _, name := range names {
		order = append(order, core.TaskStatus(name))
	}
	return order
}

// statusRanks maps each configured status to its declaration index, the
// sort key the task list groups on.
//
// A status absent from the map (a stale row written before a config
// edit) sorts as 0 by Go's zero value, which puts it alongside the
// initial status rather than dropping it. That is the desired failure
// mode: an unrecognized status still renders, at the top, where it is
// visible.
func statusRanks() map[core.TaskStatus]int {
	order := configuredStatusOrder()
	ranks := make(map[core.TaskStatus]int, len(order))
	for i, status := range order {
		ranks[status] = i
	}
	return ranks
}

// kanbanStatusOrder returns the statuses that earn a kanban column.
//
// Every configured status gets one EXCEPT the skip target: a kanban
// board tracks work moving toward completion, and the abandoned pile is
// not a stage of that. That is the one exclusion the built-in three-column
// board encoded (TODO/IN_PROGRESS/DONE, with SKIPPED left out), and it
// is expressed here as the role rather than the name so a vocabulary
// spelling it DROPPED or CANCELED is excluded for the same reason.
//
// The skip target is resolved by role only — never by the name-based
// fallback core.SkippedStatus also accepts. A board is a display
// choice, and silently dropping a column because a status happened to
// be named SKIPPED would be a worse outcome than showing one column too
// many.
func kanbanStatusOrder() []core.TaskStatus {
	wm := core.DefaultWorkflow()
	skipped, hasSkipped := skipRoleStatus(wm)

	order := configuredStatusOrder()
	cols := make([]core.TaskStatus, 0, len(order))
	for _, status := range order {
		if hasSkipped && status == skipped {
			continue
		}
		cols = append(cols, status)
	}
	return cols
}

// skipRoleStatus returns the status declaring role "skipped", if any.
func skipRoleStatus(wm *core.WorkflowManager) (core.TaskStatus, bool) {
	status, err := wm.StatusForRole(config.RoleSkipped)
	if err != nil {
		return "", false
	}
	return status, true
}

// nextRotationStatus answers what the rotate key should target for one
// task, or reports that it should do nothing.
//
// THE RING RULE: the next status is the first status, in declared
// order, that the task's own workflow accepts as a transition target
// from where the task is now — scanning from just past the current
// status and wrapping around, so a vocabulary whose rules form a cycle
// rotates forward through it rather than bouncing between two entries.
//
// The ring is driven off the STATE MACHINE, not off the status list
// alone, because those are different questions. Walking the list in
// order would happily offer BACKLOG -> SHIPPED on a workflow whose
// rules require passing through DOING; the service validates against
// the same workflow and would reject it, and the user would see a
// keypress that silently does nothing. Consulting ValidateTransition
// makes "what the TUI offers" and "what the workflow permits" the same
// set by construction, so that failure is not merely fixed but
// unreachable.
//
// Per-task workflow overrides are honored via WorkflowForTask, so a
// task carrying a tag with its own state machine rotates through that
// machine's transitions rather than the base one's.
//
// A status with no allowed target — every terminal status, since
// terminal states are immutable and `tlc task reopen` is their one
// sanctioned exit — returns ok=false, and the caller does nothing
// visible. Surfacing "terminal states are immutable" as an error banner
// on a rotate keypress would be noise: the user pressed a cycle key,
// not a reopen command.
func nextRotationStatus(task *core.Task) (core.TaskStatus, bool) {
	if task == nil {
		return "", false
	}

	wm := core.DefaultWorkflow()
	if tw, err := wm.WorkflowForTask(task); err == nil && tw != nil {
		wm = tw
	}

	names := wm.GetAllStatuses()
	start := 0
	for i, name := range names {
		if core.TaskStatus(name) == task.Status {
			start = i + 1
			break
		}
	}

	for i := range names {
		cand := core.TaskStatus(names[(start+i)%len(names)])
		if cand == task.Status {
			continue
		}
		if err := wm.ValidateTransition(task.Status, cand, false); err == nil {
			return cand, true
		}
	}
	return "", false
}

// formatStatusWithStyles renders a task status cell: the status's
// configured TLS marker in brackets, styled by the status's ROLE.
//
// The marker comes from the status DEFINITION rather than a switch over
// status names, which is what internal/cli's formatStatus already does
// for the same reason. That makes the TUI agree with every other
// surface on what glyph a status wears, custom vocabularies included.
//
// The COLOR, though, deliberately does NOT come from the definition the
// way internal/cli's formatter takes it. The TUI is themed: its palette
// is a kit/cli.Theme the user selects, and every other cell it renders
// resolves through that theme. A status definition's `color` is a fixed
// literal ("green", "#00af00") written without knowledge of which theme
// is active, so honoring it here would punch theme-blind colors into an
// otherwise themed surface — legible on the theme the author had in
// mind, and not on the others. internal/cli has no such theme to
// respect, which is why the same field is the right source there and
// the wrong one here.
//
// Role is what bridges them: the theme carries an emphasis per role
// (active, completed, skipped), so a custom vocabulary inherits the
// user's theme rather than either a fixed literal or flat text.
//
// Fallbacks, in order: a status with no marker renders its label (or
// name) rather than empty brackets, and a status the workflow does not
// know at all renders its raw name. The renderer stays total — a task
// row never disappears because its status is unrecognized.
func formatStatusWithStyles(status core.TaskStatus, s *styles.Styles) string {
	wm := core.DefaultWorkflow()
	def, err := wm.GetStatusDef(status)
	if err != nil {
		return string(status)
	}

	cell := "[" + def.TLSMarker + "]"
	if def.TLSMarker == "" {
		cell = def.Label
		if cell == "" {
			cell = def.Name
		}
	}

	return statusRoleStyle(def.Role, s).Render(cell)
}

// statusRoleStyle picks the theme style for a status, keyed on ROLE
// rather than name so a custom vocabulary inherits the theme's
// active/completed emphasis instead of rendering flat. A status
// declaring no role gets the neutral initial-status style.
func statusRoleStyle(role string, s *styles.Styles) lipgloss.Style {
	switch role {
	case config.RoleActive:
		return s.InProgress
	case config.RoleCompleted:
		return s.Done
	case config.RoleSkipped:
		return s.Skipped
	default:
		return s.Todo
	}
}
