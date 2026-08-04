package cli

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"hop.top/kit/go/core/util"
	"hop.top/kit/go/runtime/domain"
	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/storage"
)

// TaskFieldChanges describes an optional-field update to a task, decoupled
// from cobra's flag-changed tracking so both the CLI (task update) and the
// HTTP API (serve's PATCH /tasks/{id}) can build one from their own input
// source and apply it identically via applyTaskFieldChanges. A nil pointer
// means "no change"; the per-field sentinel conventions below mirror
// exactly what task_update.go's RunE used to do inline, field by field —
// they are deliberately not unified into one scheme.
type TaskFieldChanges struct {
	// Title, Description: nil = no change. Empty string is a valid value
	// (matches the CLI, which never treated "" as a clear sentinel here).
	Title       *string
	Description *string

	// AssignedTo: nil = no change; "" or "-" or "null" clears the
	// assignee (matches taskUpdateAssignedTo handling in RunE).
	AssignedTo *string

	// Status: nil = no change. StatusNote falls back to "Manual update"
	// (CLI) — callers that want a different fallback (e.g. the HTTP API's
	// "Updated via serve API") must set StatusNote explicitly since empty
	// string always means "use the fallback", matching existing behavior.
	Status      *string
	StatusNote  string
	StatusForce bool

	// Effort, Priority: nil = no change; "" or "-" clears the field.
	Effort   *string
	Priority *string

	// Tags: set-diff, applied add-then-remove against the existing tag
	// set (matches RunE's tagMap logic exactly, including dedup via map).
	AddTags    []string
	RemoveTags []string

	// BlockedBy: ClearBlockedBy wins outright (SetBlockedBy(nil)); add/
	// remove are independent set operations layered on top, matching the
	// CLI where --clear-blocked-by, --add-blocked-by and
	// --remove-blocked-by are independent flags that can combine in a
	// single invocation.
	ClearBlockedBy  bool
	AddBlockedBy    []string
	RemoveBlockedBy []string

	// BlockedReason / Unblock: independent boolean-ish flags, not a
	// shared clear sentinel — matches --blocked / --unblock in RunE.
	BlockedReason *string
	Unblock       bool

	// Timeout: nil = no change. Parsed with time.ParseDuration; there is
	// no clear sentinel here because the CLI never supported clearing
	// --timeout via update (only setting it).
	Timeout *string

	// Track: nil = no change; "" or "-" unlinks. A non-empty value that
	// doesn't resolve to an existing track triggers auto-create (see
	// AutoCreateTrack below), matching --track in RunE.
	Track *string

	// Due, RemindAt, RRule: nil = no change; "" or "-" clears the field.
	Due      *string
	RemindAt *string
	RRule    *string

	// NoAutoRemind: nil = no change (matches Changed("no-auto-remind")).
	NoAutoRemind *bool

	// Note is the caller's reason for this edit, recorded on an UPDATED
	// log row so it survives updates that are not status transitions.
	// Before this existed, a note supplied alongside a plain field edit
	// was read only by the Status branch and silently discarded, losing
	// commit-SHA linkage notes. See docs/state-change-notes-design.md
	// §189-195.
	//
	// When Status is also set, StatusNote carries the same text onto the
	// transition row; both rows keep it so neither view of the history
	// loses the reason. Empty = record nothing.
	//
	// NoteFields optionally names the fields this edit touched, stored
	// under meta.fields for audit. Callers that don't track field names
	// may leave it nil.
	Note       string
	NoteFields []string

	// Eva: nil slices = no change. Included for parity with the CLI's
	// --add-eva/--remove-eva/--clear-eva; the HTTP API is free to leave
	// these unset if eva isn't in scope for a given request shape.
	ClearEva  bool
	AddEva    []string
	RemoveEva []string

	// AutoCreateTrack, when Track resolves to ErrTrackNotFound, creates a
	// track named Track and links it instead of failing. Callers that
	// don't want auto-create (e.g. a strict HTTP API) can leave this nil;
	// the shared function then surfaces ErrTrackNotFound directly.
	AutoCreateTrack func(ctx context.Context, s *storage.SQLiteStorage, input string) (string, error)
}

// resolveOrCreateTrackID resolves a non-empty, non-clear-sentinel track
// reference to its canonical track ID. If ref doesn't resolve to an
// existing track (ErrTrackNotFound) and autoCreate is non-nil, it
// creates a track named ref and returns that instead of failing —
// matching --track's CLI ergonomics. With autoCreate nil (e.g. the
// strict HTTP API), an unresolvable ref surfaces ErrTrackNotFound
// directly. Any other resolution error is returned unchanged.
func resolveOrCreateTrackID(
	ctx context.Context, taskStorage *storage.SQLiteStorage, ref string,
	autoCreate func(ctx context.Context, s *storage.SQLiteStorage, input string) (string, error),
) (string, error) {
	resolvedID, err := resolveTrackID(ctx, taskStorage, ref)
	if err == nil {
		return resolvedID, nil
	}
	if !errors.Is(err, ErrTrackNotFound) {
		return "", err
	}
	if autoCreate == nil {
		return "", err
	}
	return autoCreate(ctx, taskStorage, ref)
}

// applyTaskFieldChanges mutates task in place per the given changes,
// running the same validation/normalization path TaskUpdateCmd's RunE used
// to run inline, so the CLI and the HTTP API can never drift apart on task
// field semantics. registryStorage is used to resolve cross-project
// blocked-by refs (mirrors validateBlockedByRefs' registryStorage/
// localStorage split); pass the same value as taskStorage when there is no
// separate registry handle.
//
// Returns whether anything changed and the first error encountered. Field
// application stops at the first error, matching RunE's per-task
// continue-on-error loop (the caller is expected to record the error
// against the task and move on to the next one, exactly as RunE's errs
// slice does).
func applyTaskFieldChanges(ctx context.Context, registryStorage, taskStorage *storage.SQLiteStorage, task *core.Task, changes TaskFieldChanges) (changed bool, err error) {
	if changes.Title != nil {
		task.Title = *changes.Title
		changed = true
	}
	if changes.Description != nil {
		task.Description = unescapeMarkdown(*changes.Description)
		changed = true
	}
	if changes.AssignedTo != nil {
		if *changes.AssignedTo == "null" || *changes.AssignedTo == "-" || *changes.AssignedTo == "" {
			task.AssignedTo = nil
		} else {
			task.AssignedTo = changes.AssignedTo
		}
		changed = true
	}

	if changes.Status != nil {
		normalized, ok := NormalizeStatus(*changes.Status)
		if !ok {
			return changed, fmt.Errorf("%w: unknown status %q; valid values: TODO, IN_PROGRESS, DONE, SKIPPED", domain.ErrValidation, *changes.Status)
		}
		nextStatus := core.TaskStatus(normalized)
		wm := core.DefaultWorkflow()
		transitionNote := changes.StatusNote
		if transitionNote == "" {
			transitionNote = "Manual update"
		}
		log, transErr := task.TransitionWithWorkflow(
			nextStatus, core.GetCurrentUser(), transitionNote, wm, changes.StatusForce,
		)
		if transErr != nil {
			return changed, fmt.Errorf("%w: failed to transition: %v", domain.ErrInvalidTransition, transErr)
		}
		if err := taskStorage.AddLog(ctx, log); err != nil {
			fmt.Printf("Warning: failed to write log for %s: %v\n", formatTaskAlias(task), err)
		}
		changed = true
	}

	if changes.Effort != nil {
		// "" and "-" clear the field, matching the existing pattern for
		// --due, --remind-at, --rrule, --assigned-to.
		if *changes.Effort == "" || *changes.Effort == "-" {
			task.Effort = ""
		} else {
			normalized, ok := NormalizeEffort(*changes.Effort)
			if !ok {
				return changed, fmt.Errorf("%w: invalid effort %q: must be one of XS, S, M, L, XL", domain.ErrValidation, *changes.Effort)
			}
			task.Effort = core.Effort(normalized)
		}
		changed = true
	}

	if changes.Priority != nil {
		// "" and "-" clear the field, matching the existing pattern for
		// --due, --remind-at, --rrule, --assigned-to.
		if *changes.Priority == "" || *changes.Priority == "-" {
			task.Priority = ""
		} else {
			normalized, ok := NormalizePriority(*changes.Priority)
			if !ok {
				return changed, fmt.Errorf("%w: invalid priority %q: must be one of P0, P1, P2, P3", domain.ErrValidation, *changes.Priority)
			}
			task.Priority = core.Priority(normalized)
		}
		changed = true
	}

	if changes.ClearBlockedBy {
		task.SetBlockedBy(nil)
		changed = true
	}

	if changes.ClearEva {
		task.SetEva(nil)
		changed = true
	}

	if len(changes.AddEva) > 0 {
		task.AddEva(changes.AddEva)
		changed = true
	}

	if len(changes.RemoveEva) > 0 {
		task.RemoveEva(changes.RemoveEva)
		changed = true
	}

	if len(changes.AddBlockedBy) > 0 {
		validated, err := validateBlockedByRefs(ctx, registryStorage, taskStorage, changes.AddBlockedBy)
		if err != nil {
			return changed, fmt.Errorf("%w: %v", domain.ErrValidation, err)
		}
		task.AddBlockedBy(validated)
		changed = true
	}

	if len(changes.RemoveBlockedBy) > 0 {
		// Translate display aliases and bare seq into typeid form before
		// set-diff so callers can remove blockers by the same ID they
		// see in `task show`. parseTaskRefForCLI returns the input
		// unchanged for cross-project refs and on resolution failure, so
		// the translation is safe; empty/whitespace inputs are skipped
		// to keep intent explicit.
		translated := make([]string, 0, len(changes.RemoveBlockedBy))
		for _, raw := range changes.RemoveBlockedBy {
			if strings.TrimSpace(raw) == "" {
				continue
			}
			ref, refErr := parseTaskRefForCLI(ctx, taskStorage, raw)
			if refErr != nil {
				// parseTaskRefForCLI returns the input unchanged on
				// resolution failure (see comment above), so fall back
				// to the raw value rather than dropping it.
				ref = raw
			}
			translated = append(translated, ref)
		}
		task.RemoveBlockedBy(translated)
		changed = true
	}

	if len(changes.AddTags) > 0 || len(changes.RemoveTags) > 0 {
		tagMap := make(map[string]bool)
		for _, t := range task.Tags {
			tagMap[t] = true
		}
		for _, t := range changes.AddTags {
			tagMap[t] = true
		}
		for _, t := range changes.RemoveTags {
			delete(tagMap, t)
		}
		newTags := []string{}
		for t := range tagMap {
			newTags = append(newTags, t)
		}
		task.Tags = newTags
		changed = true
	}

	if changes.BlockedReason != nil {
		task.BlockedReason = changes.BlockedReason
		changed = true
	}
	if changes.Unblock {
		task.BlockedReason = nil
		changed = true
	}
	if changes.Timeout != nil {
		d, err := time.ParseDuration(*changes.Timeout)
		if err != nil {
			return changed, fmt.Errorf("%w: invalid timeout %q: %v", domain.ErrValidation, *changes.Timeout, err)
		}
		task.StaleTimeout = &d
		changed = true
	}

	if changes.Track != nil {
		if *changes.Track == "-" || *changes.Track == "" {
			task.TrackID = nil
		} else {
			resolvedID, err := resolveOrCreateTrackID(ctx, taskStorage, *changes.Track, changes.AutoCreateTrack)
			if err != nil {
				return changed, err
			}
			task.TrackID = &resolvedID
		}
		changed = true
	}

	if changes.Due != nil {
		if *changes.Due == "-" || *changes.Due == "" {
			task.DueAt = nil
		} else {
			t, err := util.ParseUntil(*changes.Due)
			if err != nil {
				return changed, fmt.Errorf("%w: invalid due date: %v", domain.ErrValidation, err)
			}
			task.DueAt = &t
		}
		changed = true
	}
	if changes.RemindAt != nil {
		if *changes.RemindAt == "-" || *changes.RemindAt == "" {
			task.RemindAt = nil
		} else {
			t, err := util.ParseUntil(*changes.RemindAt)
			if err != nil {
				return changed, fmt.Errorf("%w: invalid remind-at: %v", domain.ErrValidation, err)
			}
			task.RemindAt = &t
		}
		changed = true
	}
	if changes.RRule != nil {
		if *changes.RRule == "-" || *changes.RRule == "" {
			task.RRule = ""
		} else {
			if err := core.ValidateRRule(*changes.RRule); err != nil {
				return changed, fmt.Errorf("%w: invalid rrule: %v", domain.ErrValidation, err)
			}
			task.RRule = *changes.RRule
		}
		changed = true
	}
	if changes.NoAutoRemind != nil {
		task.NoAutoRemind = *changes.NoAutoRemind
		changed = true
	}

	// A note is itself a reason to persist: it records why an edit
	// happened. A bare note with no field edit is a valid annotation,
	// not a no-op, so it must not be swallowed by the !changed return.
	if !changed {
		if changes.Note != "" {
			addUpdateNoteLog(ctx, taskStorage, task, changes)
			return true, nil
		}
		return false, nil
	}

	task.StaleFiredAt = nil // reset stale crossing state on any change
	var assignedTo string
	if task.AssignedTo != nil {
		assignedTo = *task.AssignedTo
	}
	valCfg := getValidationConfig()
	if err := valCfg.ValidateTaskOp(config.ValidationOpUpdate, config.TaskFields{
		Title:       task.Title,
		Description: task.Description,
		Status:      string(task.Status),
		AssignedTo:  assignedTo,
		Effort:      string(task.Effort),
		Priority:    string(task.Priority),
		Tags:        task.Tags,
		Reference:   task.Reference,
	}); err != nil {
		return changed, fmt.Errorf("%w: %v", domain.ErrValidation, err)
	}

	task.UpdatedAt = time.Now().UTC()

	// Local DB is the source of truth; commit it first.
	if err := taskStorage.UpdateTask(ctx, task); err != nil {
		return changed, fmt.Errorf("failed to update: %w", err)
	}

	// Record the note for the non-status part of the edit. A status-only
	// edit is already fully described by its transition row above, so
	// adding an UPDATED row there would just duplicate it.
	if changes.Note != "" && !statusOnlyEdit(changes) {
		addUpdateNoteLog(ctx, taskStorage, task, changes)
	}

	return changed, nil
}

// statusOnlyEdit reports whether the sole field this edit touches is
// status, in which case TransitionWithWorkflow's log row already carries
// the note via StatusNote and an extra UPDATED row would duplicate it.
//
// Enumerated explicitly rather than via reflection: TaskFieldChanges
// holds a func field (AutoCreateTrack) that is not comparable, and a new
// field added here should force a deliberate decision rather than
// silently defaulting either way.
func statusOnlyEdit(changes TaskFieldChanges) bool {
	if changes.Status == nil {
		return false
	}
	touchesOther := changes.Title != nil ||
		changes.Description != nil ||
		changes.AssignedTo != nil ||
		changes.Effort != nil ||
		changes.Priority != nil ||
		changes.BlockedReason != nil ||
		changes.Timeout != nil ||
		changes.Track != nil ||
		changes.Due != nil ||
		changes.RemindAt != nil ||
		changes.RRule != nil ||
		changes.NoAutoRemind != nil ||
		changes.Unblock ||
		changes.ClearBlockedBy ||
		changes.ClearEva ||
		len(changes.AddTags) > 0 ||
		len(changes.RemoveTags) > 0 ||
		len(changes.AddBlockedBy) > 0 ||
		len(changes.RemoveBlockedBy) > 0 ||
		len(changes.AddEva) > 0 ||
		len(changes.RemoveEva) > 0
	return !touchesOther
}

// addUpdateNoteLog appends an UPDATED log row carrying the caller's note,
// with the edited field names under meta.fields when known. A failure to
// write the note is reported as a warning rather than an error: the task
// row is already committed, and losing the audit note must not make a
// successful edit look failed.
func addUpdateNoteLog(
	ctx context.Context, taskStorage *storage.SQLiteStorage,
	task *core.Task, changes TaskFieldChanges,
) {
	entry := &core.LogEntry{
		TaskID:    task.ID,
		Timestamp: time.Now().UTC(),
		By:        core.GetCurrentUser(),
		Action:    core.ActionUpdated,
		Note:      changes.Note,
	}
	if len(changes.NoteFields) > 0 {
		fields := append([]string(nil), changes.NoteFields...)
		sort.Strings(fields)
		entry.Meta = map[string]any{"fields": fields}
	}
	if err := taskStorage.AddLog(ctx, entry); err != nil {
		fmt.Printf("Warning: failed to write note for %s: %v\n", formatTaskAlias(task), err)
	}
}
