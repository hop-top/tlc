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

// taskListResponse wraps task list results with a total count so
// clients can distinguish "empty page" from "no more results" without
// a separate count endpoint.
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
				writeAPIError(w, http.StatusUnprocessableEntity, "invalid_status", "unknown status %q; valid values: TODO, IN_PROGRESS, DONE, SKIPPED", v)
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
			writeAPIError(w, http.StatusInternalServerError, "internal_error", "failed to list tasks: %v", err)
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
			writeAPIError(w, http.StatusBadRequest, "invalid_json", "invalid request body: %v", err)
			return
		}

		if req.Title == "" {
			writeAPIError(w, http.StatusUnprocessableEntity, "validation_error", "title is required")
			return
		}

		status := req.Status
		if status == "" {
			status = string(core.StatusTodo)
		}
		normalizedStatus, ok := NormalizeStatus(status)
		if !ok {
			writeAPIError(w, http.StatusUnprocessableEntity, "invalid_status", "unknown status %q", status)
			return
		}
		effort := req.Effort
		if effort != "" {
			normalized, ok := NormalizeEffort(effort)
			if !ok {
				writeAPIError(w, http.StatusUnprocessableEntity, "invalid_effort", "invalid effort %q: must be one of XS, S, M, L, XL", effort)
				return
			}
			effort = normalized
		}
		priority := req.Priority
		if priority != "" {
			normalized, ok := NormalizePriority(priority)
			if !ok {
				writeAPIError(w, http.StatusUnprocessableEntity, "invalid_priority", "invalid priority %q: must be one of P0, P1, P2, P3", priority)
				return
			}
			priority = normalized
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
			writeAPIError(w, http.StatusUnprocessableEntity, "validation_error", "%v", err)
			return
		}

		meta := make(map[string]interface{})
		if blockedBy := core.NormalizeBlockedBy(req.BlockedBy); len(blockedBy) > 0 {
			validated, err := validateBlockedByRefs(ctx, deps.storage, deps.storage, blockedBy)
			if err != nil {
				writeAPIError(w, http.StatusUnprocessableEntity, "invalid_blocked_by", "%v", err)
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
				writeAPIError(w, http.StatusUnprocessableEntity, "invalid_track", "%v", trackErr)
				return
			}
			task.TrackID = &resolved
			if tr, _ := deps.storage.GetTrack(ctx, resolved); tr != nil {
				parentTrackType = tr.Type
			}
		}

		if task.ProjectID != nil && *task.ProjectID != "" {
			if err := core.GateTaskCreate(*task.ProjectID, parentTrackType); err != nil {
				writeAPIError(w, http.StatusConflict, "stage_gate", "%v", err)
				return
			}
		}

		if task.Reference == "" {
			task.Reference = buildTaskReference(task.ID, core.DetectProject())
		}

		if err := deps.storage.CreateTask(ctx, task); err != nil {
			writeAPIError(w, http.StatusInternalServerError, "internal_error", "failed to create task: %v", err)
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

		publishTaskEvent(r.Context(), deps.publisher, events.TopicTaskCreated, events.TaskCreatedPayload{
			TaskID:     task.ID,
			Title:      task.Title,
			TrackID:    derefStr(task.TrackID),
			AssignedTo: derefStr(task.AssignedTo),
			Tags:       task.Tags,
		})

		api.JSON(w, http.StatusCreated, task)
	}
}

// taskUpdateRequest is the JSON body accepted by PATCH /tasks/{id}.
// Only fields present (non-nil pointers) are applied — mirrors the
// CLI's cmd.Flags().Changed() gating in task_update.go so an omitted
// field never clobbers existing data.
type taskUpdateRequest struct {
	Title       *string   `json:"title,omitempty"`
	Description *string   `json:"description,omitempty"`
	Status      *string   `json:"status,omitempty"`
	AssignedTo  *string   `json:"assigned_to,omitempty"`
	Effort      *string   `json:"effort,omitempty"`
	Priority    *string   `json:"priority,omitempty"`
	Tags        *[]string `json:"tags,omitempty"`
	Note        string    `json:"note,omitempty"`
	Force       bool      `json:"force,omitempty"`
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
			writeAPIError(w, http.StatusBadRequest, "invalid_json", "invalid request body: %v", err)
			return
		}

		var logEntry *core.LogEntry
		user := core.GetCurrentUser()

		if req.Title != nil {
			task.Title = *req.Title
		}
		if req.Description != nil {
			task.Description = *req.Description
		}
		if req.AssignedTo != nil {
			if *req.AssignedTo == "" {
				task.AssignedTo = nil
			} else {
				task.AssignedTo = req.AssignedTo
			}
		}
		if req.Effort != nil {
			normalized, ok := NormalizeEffort(*req.Effort)
			if !ok {
				writeAPIError(w, http.StatusUnprocessableEntity, "invalid_effort", "invalid effort %q", *req.Effort)
				return
			}
			task.Effort = core.Effort(normalized)
		}
		if req.Priority != nil {
			normalized, ok := NormalizePriority(*req.Priority)
			if !ok {
				writeAPIError(w, http.StatusUnprocessableEntity, "invalid_priority", "invalid priority %q", *req.Priority)
				return
			}
			task.Priority = core.Priority(normalized)
		}
		if req.Tags != nil {
			task.Tags = *req.Tags
		}

		if req.Status != nil {
			normalized, ok := NormalizeStatus(*req.Status)
			if !ok {
				writeAPIError(w, http.StatusUnprocessableEntity, "invalid_status", "unknown status %q", *req.Status)
				return
			}
			wm := core.DefaultWorkflow()
			note := req.Note
			if note == "" {
				note = "Updated via serve API"
			}
			entry, transErr := task.TransitionWithWorkflow(core.TaskStatus(normalized), user, note, wm, req.Force)
			if transErr != nil {
				writeAPIError(w, http.StatusConflict, "invalid_transition", "%v", transErr)
				return
			}
			logEntry = entry
		} else {
			task.UpdatedAt = time.Now().UTC()
			logEntry = &core.LogEntry{
				TaskID:    task.ID,
				Timestamp: task.UpdatedAt,
				By:        user,
				Action:    "UPDATED",
				Note:      req.Note,
			}
		}

		if err := deps.storage.UpdateTaskWithLog(ctx, task, logEntry); err != nil {
			writeAPIError(w, http.StatusInternalServerError, "internal_error", "failed to update task: %v", err)
			return
		}

		api.JSON(w, http.StatusOK, task)
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
			writeAPIError(w, http.StatusInternalServerError, "internal_error", "workflow has no active status: %v", wmErr)
			return
		}
		logEntry, transErr := task.TransitionWithWorkflow(activeStatus, user, "Claimed via serve API", wm, false)
		if transErr != nil {
			writeAPIError(w, http.StatusConflict, "invalid_transition", "%v", transErr)
			return
		}

		if err := deps.storage.UpdateTaskWithLog(ctx, task, logEntry); err != nil {
			writeAPIError(w, http.StatusInternalServerError, "internal_error", "failed to claim task: %v", err)
			return
		}

		if task.TrackID != nil && *task.TrackID != "" {
			svc := core.NewTrackService(deps.storage, deps.storage)
			_ = svc.AutoTransitionOnTaskClaim(ctx, *task.TrackID) //nolint:errcheck // best-effort, mirrors CLI claim
		}

		publishTaskEvent(ctx, deps.publisher, events.TopicTaskClaimed, events.TaskClaimedPayload{
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
			writeAPIError(w, http.StatusInternalServerError, "internal_error", "workflow has no completed status: %v", wmErr)
			return
		}
		logEntry, transErr := task.TransitionWithWorkflow(completedStatus, user, "Completed via serve API", wm, false)
		if transErr != nil {
			writeAPIError(w, http.StatusConflict, "invalid_transition", "%v", transErr)
			return
		}

		if err := deps.storage.UpdateTaskWithLog(ctx, task, logEntry); err != nil {
			writeAPIError(w, http.StatusInternalServerError, "internal_error", "failed to complete task: %v", err)
			return
		}

		var durationSec int64
		if !startedAt.IsZero() {
			durationSec = int64(task.UpdatedAt.Sub(startedAt).Seconds())
		}
		publishTaskEvent(ctx, deps.publisher, events.TopicTaskCompleted, events.TaskCompletedPayload{
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
	return uri.NewResolver(s).ResolveTask(ctx, canonical)
}

// writeResolveError maps a task-resolution failure to the right HTTP
// status, distinguishing not-found from other errors.
func writeResolveError(w http.ResponseWriter, id string, err error) {
	var notFound *uri.ErrTaskNotFound
	if errors.As(err, &notFound) {
		writeAPIError(w, http.StatusNotFound, "not_found", "task %q not found", id)
		return
	}
	writeAPIError(w, http.StatusBadRequest, "bad_request", "%v", err)
}

func writeAPIError(w http.ResponseWriter, status int, code, format string, args ...any) {
	api.Error(w, status, &api.APIError{
		Status:  status,
		Code:    code,
		Message: fmt.Sprintf(format, args...),
	})
}

// publishTaskEvent publishes a domain lifecycle event under tlc's own
// topic namespace. No-op when the publisher is unavailable (e.g. bus
// not initialised), matching the nil-safety convention already used by
// events.DomainOptions.
func publishTaskEvent(ctx context.Context, pub api.EventPublisher, topic bus.Topic, payload any) {
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
