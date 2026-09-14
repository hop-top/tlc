package vtodo

import (
	"strings"

	vstar "hop.top/vstar"
	"hop.top/vstar/supersession"

	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/core"
)

// MetaEffectiveStatusKey is the LogEntry.Meta key under which the
// decoder keeps the X-VSTAR-EFFECTIVE-STATUS of an imported supersession
// journal when the entry's own action could not reproduce it: a
// foreign ledger entry with no X-TLC-LOG-ACTION, or one whose wire
// value disagrees with what the action derives. The encoder emits it
// verbatim, so a foreign ledger survives a tlc round trip. A derived
// key: never written into X-TLC-META (see derivedMetaKeys).
const MetaEffectiveStatusKey = "effective_status"

// transitionRoles maps the fixed transition constants to the status
// role each leaves the task in, which is what picks the RFC 5545 STATUS
// value written as X-VSTAR-EFFECTIVE-STATUS. The spec gives the property
// no vocabulary; tlc uses the VTODO STATUS set, the same projection the
// task builder makes, so a ledger and the VTODO it supersedes speak one
// language (docs/VSTAR-CONFORMANCE.md records the choice).
//
//   - CLAIMED takes the task into the active role.
//   - RELEASED, RETRY, REOPENED and UNBLOCKED return it to the initial
//     role.
//   - BLOCKED parks it: the executor's fail path has already returned
//     the task to the initial role when it logs BLOCKED, and the plain
//     block path leaves a task the executor will not pick up again
//     until a human acts. NEEDS-ACTION is the RFC value for both.
//   - DONE completes, SKIPPED cancels.
//
// A configured status name is not here: its role comes from the
// vocabulary, through statusToWire.
var transitionRoles = map[string]string{
	core.ActionClaimed:   config.RoleActive,
	core.ActionReleased:  config.RoleInitial,
	core.ActionRetry:     config.RoleInitial,
	actionReopened:       config.RoleInitial,
	core.ActionBlocked:   config.RoleInitial,
	core.ActionUnblocked: config.RoleInitial,
	core.ActionDone:      config.RoleCompleted,
	core.ActionSkipped:   config.RoleSkipped,
}

// derivedEffectiveStatus is the RFC 5545 STATUS value a status-class
// action lands the task in: the fixed table first, else the configured
// vocabulary's role for the name (NEEDS-ACTION for a name it does not
// declare, the same fallback STATUS itself takes).
func derivedEffectiveStatus(action string, defs []config.StatusDefinition) string {
	if role, ok := transitionRoles[strings.ToUpper(strings.TrimSpace(action))]; ok {
		return roleToWire(role)
	}
	return statusToWire(core.TaskStatus(strings.TrimSpace(action)), defs)
}

// effectiveStatus is the value written as X-VSTAR-EFFECTIVE-STATUS: a
// value preserved from the wire wins over one derived from the action,
// because the wire value is the ledger's fact and the derivation is
// tlc's reading of it.
func effectiveStatus(le *core.LogEntry, defs []config.StatusDefinition) string {
	if v := strings.TrimSpace(metaString(le.Meta, MetaEffectiveStatusKey)); v != "" {
		return v
	}
	return derivedEffectiveStatus(le.Action, defs)
}

// isSupersessionEntry reports whether a log entry is a status-change
// record in the ledger sense: its action is in the status class, or it
// carries an effective status preserved from an imported supersession
// journal (a foreign entry may have no action at all).
func isSupersessionEntry(le *core.LogEntry, defs []config.StatusDefinition) bool {
	if metaString(le.Meta, MetaEffectiveStatusKey) != "" {
		return true
	}
	return isStatusTransition(le.Action, defs)
}

// isTodoStatusWire reports whether s is one of the four RFC 5545
// VTODO STATUS values, compared case-insensitively because a ledger
// written by another producer is the input here.
func isTodoStatusWire(s string) bool {
	for _, want := range []vstar.TodoStatus{
		vstar.TodoNeedsAction, vstar.TodoInProcess, vstar.TodoCompleted, vstar.TodoCancelled,
	} {
		if strings.EqualFold(strings.TrimSpace(s), string(want)) {
			return true
		}
	}
	return false
}

// supersessionStatus reports whether j is a well-formed supersession
// journal (VJOURNAL, RELATED-TO, CATEGORIES:status-supersession,
// X-VSTAR-EFFECTIVE-STATUS) and returns its effective status. The check
// is delegated to supersession.Superseded against a stub of j's own
// target, so what counts as a supersession entry here is exactly what
// the library projects, not a second reading of the category list.
func supersessionStatus(j vstar.Component) (string, bool) {
	rel, ok := j.Get("RELATED-TO")
	if !ok || strings.TrimSpace(rel.Value) == "" {
		return "", false
	}
	return supersession.Superseded(supersessionTarget(rel.Value), []vstar.Component{j})
}

// supersessionTarget builds the minimal component supersession.Supersedes
// and Superseded need to address a target: a VTODO carrying only the
// UID. No X-VSTAR-HASH, so Supersedes runs no integrity check on it.
func supersessionTarget(uid string) vstar.Component {
	c := vstar.Component{Type: vstar.CompTodo}
	c.Set(vstar.Property{Name: "UID", Value: uid})
	return c
}
