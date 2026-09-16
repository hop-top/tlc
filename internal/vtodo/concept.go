package vtodo

import (
	"strings"

	vstar "hop.top/vstar"

	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/core"
)

// Agentic concept tokens, the values of XPropConcept. Each exported
// component declares exactly one, so a consumer never has to infer
// from RELATED-TO edges whether a VTODO is a mission or an assignment,
// or from an action string what kind of journal a VJOURNAL is. The
// vocabulary and the mapping from tlc entities are recorded in
// docs/superpowers/specs/2026-09-14-agentic-concept-mapping-decision.md;
// docs/vtodo-sync-spec-0.1.md "Vocabulary" is the normative summary.
//
// Tokens are lowercase on the wire and compared case-insensitively on
// read.
const (
	// ConceptMission marks a goal held directly: a track, or a task
	// with no track (a standalone task carries no PARENT edge, so it
	// is the goal itself rather than a unit inside one).
	ConceptMission = "mission"
	// ConceptAssignment marks a scoped unit of work inside a mission:
	// a task with a track.
	ConceptAssignment = "assignment"
	// ConceptTurn marks a bounded execution window: one claim on an
	// assignment, open or closed.
	ConceptTurn = "turn"
	// ConceptPlaythrough marks one run through a mission or the world:
	// a recipe materialization.
	ConceptPlaythrough = "playthrough"

	// Journal sub-types. A LogEntry is one of these, derived from its
	// Action by journalConcept.
	ConceptStatus      = "status"
	ConceptDecision    = "decision"
	ConceptAction      = "action"
	ConceptObservation = "observation"
	// ConceptJournal is the umbrella for a log entry the classifier
	// cannot place: raw bus topics, foreign imports, anything else.
	ConceptJournal = "journal"
)

// actionReopened is written by the audit subscriber
// (internal/events/audit.go) rather than by core, so core exports no
// constant for it.
const actionReopened = "REOPENED"

// Action classes, per the decision document's sub-typing rule. The
// sets are keyed upper-case; journalConcept upper-cases the action
// before lookup so a hand-edited file with "claimed" still classifies.
var (
	// decisionActions record a human-gate verdict on the assignment.
	decisionActions = actionSet(core.ActionApproved, core.ActionRejected)

	// statusActions record a transition of Task.Status, or of the
	// readiness gate that decides its effective status (BLOCKED /
	// UNBLOCKED leave Task.Status alone but park and release the task).
	// Configured status NAMES are the other half of this class: a
	// transition writes the target name as its action, and the
	// vocabulary is open, so they are matched against the definitions
	// rather than listed here.
	statusActions = actionSet(
		core.ActionClaimed, core.ActionReleased, core.ActionDone,
		core.ActionSkipped, core.ActionRetry, actionReopened,
		core.ActionBlocked, core.ActionUnblocked,
	)

	// playerActions record something a player did to the assignment
	// other than move its status. RECLAIMED belongs here, not in
	// statusActions: the status stays active, only the owner and the
	// claim instant change.
	playerActions = actionSet(
		core.ActionCreated, core.ActionUpdated, core.ActionDeleted,
		core.ActionReassigned, core.ActionReclaimed, core.ActionAutoAssigned,
		core.ActionDelegated, core.ActionSplit, core.ActionMerged,
		core.ActionMigrated, core.ActionExecStart, core.ActionExecEnd,
		core.ActionExecAttempt, core.ActionSyncImported, core.ActionSyncPulled,
		core.ActionSyncPushed,
	)

	// observationActions record something noticed; no state change.
	observationActions = actionSet(
		core.ActionComment, core.ActionFailure,
		core.ActionSyncConflict, core.ActionSyncError,
	)
)

func actionSet(names ...string) map[string]bool {
	out := make(map[string]bool, len(names))
	for _, n := range names {
		out[strings.ToUpper(n)] = true
	}
	return out
}

// taskConcept declares what a task VTODO is: an assignment when it
// belongs to a track (it will carry a PARENT RELATED-TO to that
// mission), a mission when it stands alone. The token makes explicit
// what the edge structure already implies.
func taskConcept(t *core.Task) string {
	if t.TrackID != nil && *t.TrackID != "" {
		return ConceptAssignment
	}
	return ConceptMission
}

// journalConcept classifies a log entry's action into a journal
// sub-type. First match wins, in the order the decision document
// fixes:
//
//   - decision precedes status because APPROVED is produced on a
//     transition path (it completes the task) but what the entry
//     records is the verdict; the transition is its consequence. A
//     vocabulary that happens to declare a status named APPROVED does
//     not change that.
//   - status covers the fixed transition constants and any name in the
//     configured status vocabulary, because Task.TransitionWithWorkflow
//     writes the target status name as the action.
//   - the umbrella is required, not a nicety: the vocabulary is open on
//     two sides (configured names, raw bus topics from the audit
//     fallback) and the decoder mints LogEntry rows from foreign
//     VJOURNALs that carry no action at all.
func journalConcept(action string, defs []config.StatusDefinition) string {
	a := strings.ToUpper(strings.TrimSpace(action))
	switch {
	case a == "":
		return ConceptJournal
	case decisionActions[a]:
		return ConceptDecision
	case isStatusTransition(a, defs):
		return ConceptStatus
	case playerActions[a]:
		return ConceptAction
	case observationActions[a]:
		return ConceptObservation
	default:
		return ConceptJournal
	}
}

// isStatusTransition reports whether action records a change of the
// task's effective status: one of the fixed transition constants, or a
// name the configured vocabulary declares. Case-insensitive on both
// sides.
func isStatusTransition(action string, defs []config.StatusDefinition) bool {
	if statusActions[strings.ToUpper(strings.TrimSpace(action))] {
		return true
	}
	return isConfiguredStatus(action, defs)
}

// isConfiguredStatus reports whether name is declared in defs. It
// matches on the name alone, unlike statusRole, so a status declared
// without a role still counts as one.
func isConfiguredStatus(name string, defs []config.StatusDefinition) bool {
	for _, def := range defs {
		if strings.EqualFold(def.Name, strings.TrimSpace(name)) {
			return true
		}
	}
	return false
}

// declaredConcept reads the token a component carries, normalised to
// the lowercase form the encoder writes. Empty when the component
// declares nothing, which is the case for every calendar written
// before the property existed and for most foreign producers.
func declaredConcept(c vstar.Component) string {
	p, ok := c.Get(XPropConcept)
	if !ok {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(p.Value))
}

// collectConcepts indexes the declared concept of every component in
// cal, sub-components included, by wire UID. Nil when nothing declares
// one, so a foreign calendar leaves ParseResult.Concepts unset rather
// than empty.
func collectConcepts(cal vstar.Calendar) map[string]string {
	var out map[string]string
	var walk func(c vstar.Component)
	walk = func(c vstar.Component) {
		if token := declaredConcept(c); token != "" && c.UID() != "" {
			if out == nil {
				out = map[string]string{}
			}
			out[c.UID()] = token
		}
		for _, sub := range c.Sub {
			walk(sub)
		}
	}
	for _, c := range cal.Components {
		walk(c)
	}
	return out
}
