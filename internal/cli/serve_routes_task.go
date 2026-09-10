package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"hop.top/kit/go/runtime/bus"
	"hop.top/kit/go/runtime/domain"
	"hop.top/kit/go/transport/api"
	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/events"
	"hop.top/tlc/internal/storage"
	"hop.top/tlc/internal/uri"
)

// registerTaskRoutes wires task REST routes onto router under /tasks.
// Handlers call into the same storage repo (deps.storage) and core
// validation/state-machine logic (getValidationConfig, TransitionWithWorkflow)
// that internal/cli/task_*.go's cobra RunE functions use, so the HTTP
// surface enforces the same rules as the CLI — it is not a bypass.
//
// Covered: list, show, create, update (subset of fields), complete, claim.
// Deferred (see final report): delete, assign/unassign, reopen, unclaim,
// blocked-by graph endpoints, workspace-wide queries, vtodo/summary output
// formats. These are mechanical extensions of the same pattern and were
// left out of this first pass to keep the surface small and well-tested.
func registerTaskRoutes(router *api.Router, deps *serveDeps) {
	tasks := router.Group("/tasks")

	tasks.Handle("GET", "", handleTaskList(deps))
	tasks.Handle("POST", "", handleTaskCreate(deps))
	tasks.Handle("GET", "/{id}", handleTaskShow(deps))
	tasks.Handle("PATCH", "/{id}", handleTaskUpdate(deps))
	tasks.Handle("POST", "/{id}/claim", handleTaskClaim(deps))
	tasks.Handle("POST", "/{id}/complete", handleTaskComplete(deps))
}

// taskListResponse wraps task list results. Total is the count of
// items returned in this page (i.e. len(Tasks)), not a global total
// across all pages — there is no CountTasks-equivalent query in
// internal/storage, so a client cannot use Total to compute remaining
// pages; it only distinguishes "this page has items" from "this page
// is empty".
type taskListResponse struct {
	Tasks []*core.Task `json:"tasks"`
	Total int          `json:"total"`
}

func handleTaskList(deps *serveDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		q := core.Query{Limit: 100}

		query := r.URL.Query()
		if v := query.Get("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				q.Limit = n
			}
		}
		if v := query.Get("offset"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n >= 0 {
				q.Offset = n
			}
		}
		if v := query.Get("status"); v != "" {
			normalized, ok := NormalizeStatus(v)
			if !ok {
				writeAPIErrorf(w, http.StatusUnprocessableEntity, "invalid_status", "%v", unknownStatusError(v))
				return
			}
			q.Filters = append(q.Filters, core.FieldFilter{Field: "status", Value: normalized})
		}
		if v := query.Get("assigned_to"); v != "" {
			q.Filters = append(q.Filters, core.FieldFilter{Field: "assigned_to", Value: v})
		}
		if v := query.Get("track_id"); v != "" {
			q.Filters = append(q.Filters, core.FieldFilter{Field: "track_id", Operator: core.OpEq, Value: v})
		}
		if query.Get("all_projects") == "true" {
			q.AllProjects = true
		}
		if query.Get("archived") == "true" {
			q.IncludeArchived = true
		}

		tasks, err := deps.storage.ListTasks(ctx, q)
		if err != nil {
			writeAPIErrorf(w, http.StatusInternalServerError, "internal_error", "failed to list tasks: %v", err)
			return
		}
		api.JSON(w, http.StatusOK, taskListResponse{Tasks: tasks, Total: len(tasks)})
	}
}

func handleTaskShow(deps *serveDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		id := api.PathParam(r, "id")

		res, err := resolveTaskForServe(ctx, deps.storage, id)
		if err != nil {
			writeResolveError(w, id, err)
			return
		}
		if res.Storage != deps.storage {
			defer func() { _ = res.Storage.Close() }()
		}
		api.JSON(w, http.StatusOK, res.Task)
	}
}

// taskCreateRequest is the JSON body accepted by POST /tasks. Field
// names mirror the CLI's --flag names (snake_case) rather than
// core.Task's Go field names, since not every Task field is settable
// at create time (e.g. ID/Seq/CreatedAt are server-assigned).
type taskCreateRequest struct {
	Title       string   `json:"title"`
	Description string   `json:"description,omitempty"`
	Status      string   `json:"status,omitempty"`
	AssignedTo  string   `json:"assigned_to,omitempty"`
	Effort      string   `json:"effort,omitempty"`
	Priority    string   `json:"priority,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Reference   string   `json:"reference,omitempty"`
	BlockedBy   []string `json:"blocked_by,omitempty"`
	TrackID     string   `json:"track_id,omitempty"`
}

func handleTaskCreate(deps *serveDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		var req taskCreateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeAPIErrorf(w, http.StatusBadRequest, "invalid_json", "invalid request body: %v", err)
			return
		}

		if req.Title == "" {
			writeAPIErrorf(w, http.StatusUnprocessableEntity, "validation_error", "title is required")
			return
		}

		status := req.Status
		if status == "" {
			status = string(core.StatusTodo)
		}
		normalizedStatus, ok := NormalizeStatus(status)
		if !ok {
			writeAPIErrorf(w, http.StatusUnprocessableEntity, "invalid_status", "unknown status %q", status)
			return
		}
		effort := req.Effort
		if effort != "" {
			normalized, ok := NormalizeEffort(effort)
			if !ok {
				writeAPIErrorf(w, http.StatusUnprocessableEntity, "invalid_effort", "%v", unknownEffortError(effort))
				return
			}
			effort = normalized
		}
		priority := req.Priority
		if priority != "" {
			normalized, ok := NormalizePriority(priority)
			if !ok {
				writeAPIErrorf(w, http.StatusUnprocessableEntity, "invalid_priority", "%v", invalidPriorityError(priority))
				return
			}
			priority = normalized
		}

		// Same tag vocabulary gate the CLI's saveTask applies. A policy
		// enforced only on the CLI is not a policy: the HTTP surface
		// writes to the same store.
		if err := core.ValidateTags(req.Tags); err != nil {
			writeAPIErrorf(w, http.StatusUnprocessableEntity, "invalid_tags", "%v", err)
			return
		}

		// Same config-driven validation the CLI's saveTask applies.
		valCfg := getValidationConfig()
		if err := valCfg.ValidateTaskOp(config.ValidationOpCreate, config.TaskFields{
			Title:       req.Title,
			Description: req.Description,
			Status:      normalizedStatus,
			AssignedTo:  req.AssignedTo,
			Effort:      effort,
			Priority:    priority,
			Tags:        req.Tags,
			Reference:   req.Reference,
		}); err != nil {
			writeAPIErrorf(w, http.StatusUnprocessableEntity, "validation_error", "%v", err)
			return
		}

		meta := make(map[string]interface{})
		if blockedBy := core.NormalizeBlockedBy(req.BlockedBy); len(blockedBy) > 0 {
			validated, err := validateBlockedByRefs(ctx, deps.storage, deps.storage, blockedBy)
			if err != nil {
				writeAPIErrorf(w, http.StatusUnprocessableEntity, "invalid_blocked_by", "%v", err)
				return
			}
			meta["blocked_by"] = validated
		}

		now := time.Now().UTC()
		var assigneePtr *string
		if req.AssignedTo != "" {
			assigneePtr = &req.AssignedTo
		}

		task := &core.Task{
			ID:          core.NewTaskID(),
			Title:       req.Title,
			Description: req.Description,
			Status:      core.TaskStatus(normalizedStatus),
			AssignedTo:  assigneePtr,
			Effort:      core.Effort(effort),
			Priority:    core.Priority(priority),
			Tags:        req.Tags,
			Reference:   req.Reference,
			CreatedAt:   now,
			UpdatedAt:   now,
			Meta:        meta,
		}

		if proj := core.DetectProject(); proj != nil && proj.ProjectID != "" {
			task.ProjectID = &proj.ProjectID
		}

		var parentTrackType string
		if req.TrackID != "" {
			resolved, trackErr := resolveTrackID(ctx, deps.storage, req.TrackID)
			if trackErr != nil {
				writeAPIErrorf(w, http.StatusUnprocessableEntity, "invalid_track", "%v", trackErr)
				return
			}
			task.TrackID = &resolved
			if tr, getErr := deps.storage.GetTrack(ctx, resolved); getErr == nil && tr != nil {
				parentTrackType = tr.Type
			}
		}

		if task.ProjectID != nil && *task.ProjectID != "" {
			if err := core.GateTaskCreate(*task.ProjectID, parentTrackType); err != nil {
				writeAPIErrorf(w, http.StatusConflict, "stage_gate", "%v", err)
				return
			}
		}

		if task.Reference == "" {
			task.Reference = buildTaskReference(task.ID, core.DetectProject())
		}

		if err := deps.storage.CreateTask(ctx, task); err != nil {
			writeAPIErrorf(w, http.StatusInternalServerError, "internal_error", "failed to create task: %v", err)
			return
		}

		logEntry := &core.LogEntry{
			TaskID:    task.ID,
			Timestamp: now,
			By:        core.GetCurrentUser(),
			Action:    "CREATED",
			Note:      "Task created via serve API",
		}
		_ = deps.storage.AddLog(ctx, logEntry) //nolint:errcheck // best-effort audit log, mirrors saveTask

		publishDomainEvent(r.Context(), deps.publisher, events.TopicTaskCreated, events.TaskCreatedPayload{
			TaskID:     task.ID,
			Title:      task.Title,
			TrackID:    derefStr(task.TrackID),
			AssignedTo: derefStr(task.AssignedTo),
			Tags:       task.Tags,
		})

		api.JSON(w, http.StatusCreated, task)
	}
}

// taskUpdateRequest is the JSON body accepted by PATCH /tasks/{id}. Only
// fields present (non-nil pointers) are applied — mirrors the CLI's
// cmd.Flags().Changed() gating in task_update.go, now shared via
// applyTaskFieldChanges, so an omitted field never clobbers existing data.
//
// Route/verb choice: kit's generic api.ResourceRouter (hop.top/kit/go/
// transport/api/resource.go) documents PUT {prefix}/{id} → full-resource
// replace as its default REST convention for arbitrary entities. Tasks are
// deliberately kept on PATCH with partial-update semantics instead: a task
// has a dozen-plus independent optional fields (title, status, effort,
// priority, tags, due/remind-at/rrule, track, blocked-by, timeout, ...),
// and requiring a client to resend the entire resource on every edit (as
// true PUT replace would) risks silently clobbering fields the client
// never intended to touch — a functional regression, not an alignment
// win. PATCH with "apply only present fields" is the semantically correct
// verb here per RFC 5789, and it is what the CLI's own field-changed
// tracking already implements; the HTTP route just mirrors that model
// instead of forcing kit's generic-entity default onto a domain it
// doesn't fit. See applyTaskFieldChanges for the shared implementation.
type taskUpdateRequest struct {
	Title           *string   `json:"title,omitempty"`
	Description     *string   `json:"description,omitempty"`
	Status          *string   `json:"status,omitempty"`
	AssignedTo      *string   `json:"assigned_to,omitempty"`
	Effort          *string   `json:"effort,omitempty"`
	Priority        *string   `json:"priority,omitempty"`
	Tags            *[]string `json:"tags,omitempty"`
	AddTags         []string  `json:"add_tags,omitempty"`
	RemoveTags      []string  `json:"remove_tags,omitempty"`
	ClearBlockedBy  bool      `json:"clear_blocked_by,omitempty"`
	AddBlockedBy    []string  `json:"add_blocked_by,omitempty"`
	RemoveBlockedBy []string  `json:"remove_blocked_by,omitempty"`
	BlockedReason   *string   `json:"blocked_reason,omitempty"`
	Unblock         bool      `json:"unblock,omitempty"`
	Timeout         *string   `json:"timeout,omitempty"`
	Track           *string   `json:"track_id,omitempty"`
	Due             *string   `json:"due,omitempty"`
	RemindAt        *string   `json:"remind_at,omitempty"`
	RRule           *string   `json:"rrule,omitempty"`
	NoAutoRemind    *bool     `json:"no_auto_remind,omitempty"`
	Note            string    `json:"note,omitempty"`
	Force           bool      `json:"force,omitempty"`
}

func handleTaskUpdate(deps *serveDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		id := api.PathParam(r, "id")

		res, err := resolveTaskForServe(ctx, deps.storage, id)
		if err != nil {
			writeResolveError(w, id, err)
			return
		}
		if res.Storage != deps.storage {
			defer func() { _ = res.Storage.Close() }()
		}
		task := res.Task

		var req taskUpdateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeAPIErrorf(w, http.StatusBadRequest, "invalid_json", "invalid request body: %v", err)
			return
		}

		changes := TaskFieldChanges{
			Title:           req.Title,
			Description:     req.Description,
			AssignedTo:      req.AssignedTo,
			Status:          req.Status,
			StatusNote:      req.Note,
			StatusForce:     req.Force,
			Effort:          req.Effort,
			Priority:        req.Priority,
			AddTags:         req.AddTags,
			RemoveTags:      req.RemoveTags,
			ClearBlockedBy:  req.ClearBlockedBy,
			AddBlockedBy:    req.AddBlockedBy,
			RemoveBlockedBy: req.RemoveBlockedBy,
			BlockedReason:   req.BlockedReason,
			Unblock:         req.Unblock,
			Timeout:         req.Timeout,
			Track:           req.Track,
			Due:             req.Due,
			RemindAt:        req.RemindAt,
			RRule:           req.RRule,
			NoAutoRemind:    req.NoAutoRemind,
			// No AutoCreateTrack: the HTTP API treats an unresolvable
			// --track-equivalent as an error (422) rather than silently
			// creating a track, since a stray typo in a track_id over
			// HTTP shouldn't spawn new tracks the way CLI ergonomics
			// intentionally allow for interactive use.
		}
		if req.Tags != nil {
			// Bulk tags replace: apply as add-all/remove-none against the
			// current set so the shared set-diff logic in
			// applyTaskFieldChanges still owns the actual mutation.
			changes.AddTags = append(append([]string{}, *req.Tags...), changes.AddTags...)
			changes.RemoveTags = append(append([]string{}, task.Tags...), changes.RemoveTags...)
		}

		if _, err := applyTaskFieldChanges(ctx, deps.storage, res.Storage, task, changes); err != nil {
			writeTaskFieldChangeError(w, err)
			return
		}

		api.JSON(w, http.StatusOK, task)
	}
}

// writeTaskFieldChangeError maps an applyTaskFieldChanges error to the
// same HTTP status codes the previous inline implementation used:
// validation/normalization failures are 422, workflow transition
// conflicts are 409, an unresolvable track is 422, everything else (e.g.
// a storage write failure) is 500. applyTaskFieldChanges wraps its
// error returns with kit's domain sentinels (domain.ErrValidation,
// domain.ErrInvalidTransition) or tlc's own ErrTrackNotFound specifically
// so this classification can use errors.Is instead of matching on
// message text. This mirrors the existing writeAPIError convention used
// throughout this file rather than introducing api.MapError, since the
// underlying function is shared with the CLI, which has no concept of
// HTTP status codes.
func writeTaskFieldChangeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalidTransition):
		writeAPIErrorf(w, http.StatusConflict, "invalid_transition", "%v", err)
	case errors.Is(err, ErrTrackNotFound):
		writeAPIErrorf(w, http.StatusUnprocessableEntity, "invalid_track", "%v", err)
	case errors.Is(err, domain.ErrValidation):
		writeAPIErrorf(w, http.StatusUnprocessableEntity, "validation_error", "%v", err)
	default:
		writeAPIErrorf(w, http.StatusInternalServerError, "internal_error", "%v", err)
	}
}

func handleTaskClaim(deps *serveDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		id := api.PathParam(r, "id")

		res, err := resolveTaskForServe(ctx, deps.storage, id)
		if err != nil {
			writeResolveError(w, id, err)
			return
		}
		if res.Storage != deps.storage {
			defer func() { _ = res.Storage.Close() }()
		}
		task := res.Task

		user := core.GetCurrentUser()
		task.AssignedTo = &user

		wm := core.DefaultWorkflow()
		activeStatus, wmErr := wm.StatusForRole("active")
		if wmErr != nil {
			writeAPIErrorf(w, http.StatusInternalServerError, "internal_error", "workflow has no active status: %v", wmErr)
			return
		}
		logEntry, transErr := task.TransitionWithWorkflow(activeStatus, user, "Claimed via serve API", wm, false)
		if transErr != nil {
			writeAPIErrorf(w, http.StatusConflict, "invalid_transition", "%v", transErr)
			return
		}

		if err := deps.storage.UpdateTaskWithLog(ctx, task, logEntry); err != nil {
			writeAPIErrorf(w, http.StatusInternalServerError, "internal_error", "failed to claim task: %v", err)
			return
		}

		if task.TrackID != nil && *task.TrackID != "" {
			svc := core.NewTrackService(deps.storage, deps.storage)
			_ = svc.AutoTransitionOnTaskClaim(ctx, *task.TrackID) //nolint:errcheck // best-effort, mirrors CLI claim
		}

		publishDomainEvent(ctx, deps.publisher, events.TopicTaskClaimed, events.TaskClaimedPayload{
			TaskID:    task.ID,
			ClaimedBy: user,
			TrackID:   derefStr(task.TrackID),
		})

		api.JSON(w, http.StatusOK, task)
	}
}

func handleTaskComplete(deps *serveDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		id := api.PathParam(r, "id")

		res, err := resolveTaskForServe(ctx, deps.storage, id)
		if err != nil {
			writeResolveError(w, id, err)
			return
		}
		if res.Storage != deps.storage {
			defer func() { _ = res.Storage.Close() }()
		}
		task := res.Task

		user := core.GetCurrentUser()
		if task.AssignedTo == nil || *task.AssignedTo == "" {
			task.AssignedTo = &user
		}

		startedAt := task.UpdatedAt
		wm := core.DefaultWorkflow()
		completedStatus, wmErr := wm.StatusForRole("completed")
		if wmErr != nil {
			writeAPIErrorf(w, http.StatusInternalServerError, "internal_error", "workflow has no completed status: %v", wmErr)
			return
		}
		logEntry, transErr := task.TransitionWithWorkflow(completedStatus, user, "Completed via serve API", wm, false)
		if transErr != nil {
			writeAPIErrorf(w, http.StatusConflict, "invalid_transition", "%v", transErr)
			return
		}

		if err := deps.storage.UpdateTaskWithLog(ctx, task, logEntry); err != nil {
			writeAPIErrorf(w, http.StatusInternalServerError, "internal_error", "failed to complete task: %v", err)
			return
		}

		var durationSec int64
		if !startedAt.IsZero() {
			durationSec = int64(task.UpdatedAt.Sub(startedAt).Seconds())
		}
		publishDomainEvent(ctx, deps.publisher, events.TopicTaskCompleted, events.TaskCompletedPayload{
			TaskID:      task.ID,
			CompletedBy: user,
			TrackID:     derefStr(task.TrackID),
			DurationSec: durationSec,
		})

		api.JSON(w, http.StatusOK, task)
	}
}

// resolveTaskForServe resolves a path {id} (T-NNNN alias, durable
// TypeID, or cross-project URI) into a task, reusing the same
// resolution chain the CLI's task show/update commands go through.
func resolveTaskForServe(ctx context.Context, s *storage.SQLiteStorage, id string) (*uri.ResolvedTask, error) {
	canonical, err := parseTaskRefForCLI(ctx, s, id)
	if err != nil {
		return nil, err
	}
	res, err := uri.NewResolver(s).ResolveTask(ctx, canonical)
	if err != nil {
		return nil, fmt.Errorf("resolve task: %w", err)
	}
	return res, nil
}

// writeResolveError maps a task-resolution failure to the right HTTP
// status, distinguishing not-found from other errors.
func writeResolveError(w http.ResponseWriter, id string, err error) {
	var notFound *uri.ErrTaskNotFound
	if errors.As(err, &notFound) {
		writeAPIErrorf(w, http.StatusNotFound, "not_found", "task %q not found", id)
		return
	}
	writeAPIErrorf(w, http.StatusBadRequest, "bad_request", "%v", err)
}

func writeAPIErrorf(w http.ResponseWriter, status int, code, format string, args ...any) {
	api.Error(w, status, &api.APIError{
		Status:  status,
		Code:    code,
		Message: fmt.Sprintf(format, args...),
	})
}

// publishDomainEvent publishes a domain lifecycle event (task or track)
// under tlc's own topic namespace. No-op when the publisher is
// unavailable (e.g. bus not initialized), matching the nil-safety
// convention already used by events.DomainOptions.
func publishDomainEvent(ctx context.Context, pub api.EventPublisher, topic bus.Topic, payload any) {
	if pub == nil {
		return
	}
	_ = pub.Publish(ctx, string(topic), "tlc.serve", payload) //nolint:errcheck // best-effort event publish
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
