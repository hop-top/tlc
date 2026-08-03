package cli

// End-to-end tests for the `tlc serve` HTTP API: task list/show/create/
// update/claim/complete and track list/show/create. Exercises the
// router wiring (registerTaskRoutes/registerTrackRoutes) directly via
// httptest rather than binding a real TCP listener, mirroring the
// storage-level setup used by track_health_e2e_test.go and friends.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"hop.top/kit/go/transport/api"
	"hop.top/tlc/internal/core"
)

// newTestServeRouter builds an api.Router with tlc's task/track routes
// registered against a fresh test storage instance, with no auth
// middleware and no bus (publisher is nil, matching a process where the
// bus hasn't been initialized — publishDomainEvent no-ops in that case).
func newTestServeRouter(t *testing.T) *api.Router {
	t.Helper()
	s, err := getStorageRaw()
	if err != nil {
		t.Fatalf("getStorageRaw: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	deps := &serveDeps{storage: s}
	router := api.NewRouter()
	registerTaskRoutes(router, deps)
	registerTrackRoutes(router, deps)
	return router
}

// testAuthToken is the fixed bearer token used by newTestServeRouterWithAuth,
// standing in for the randomly-minted token runServe generates via
// secret.Mint. Fixed here so tests can assert the exact success/failure
// paths without needing to read the token back out of a startup line.
const testAuthToken = "test-serve-token"

// newTestServeRouterWithAuth builds the same router shape runServe wires
// in production: task/track routes, requireAuth(token) in the middleware
// chain, and the /health and /shutdown endpoints. This lets auth-gating
// and the health/shutdown handlers be exercised via httptest without
// binding a real net.Listener or going through the cobra command path.
//
// cancelCh receives a signal when /shutdown succeeds (mirroring runServe's
// `go cancel()`), so callers can assert the handler triggered a shutdown
// without standing up a real context.CancelFunc-driven server loop.
func newTestServeRouterWithAuth(t *testing.T) (router *api.Router, canceled <-chan struct{}) {
	t.Helper()
	s, err := getStorageRaw()
	if err != nil {
		t.Fatalf("getStorageRaw: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	done := make(chan struct{}, 1)
	token := testAuthToken

	router = api.NewRouter(api.WithMiddleware(requireAuth(token)))
	deps := &serveDeps{storage: s}
	registerTaskRoutes(router, deps)
	registerTrackRoutes(router, deps)

	startedAt := time.Now()
	router.Handle("GET", "/health", func(w http.ResponseWriter, _ *http.Request) {
		api.JSON(w, http.StatusOK, map[string]any{
			"status":         "ok",
			"pid":            os.Getpid(),
			"uptime_seconds": int(time.Since(startedAt).Seconds()),
		})
	})
	router.Handle("POST", "/shutdown", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			api.Error(w, http.StatusUnauthorized, &api.APIError{
				Status: http.StatusUnauthorized, Code: "unauthorized", Message: "invalid token",
			})
			return
		}
		w.WriteHeader(http.StatusNoContent)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		select {
		case done <- struct{}{}:
		default:
		}
	})

	return router, done
}

func doServeRequest(t *testing.T, router *api.Router, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reqBody *bytes.Buffer
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request body: %v", err)
		}
		reqBody = bytes.NewBuffer(b)
	} else {
		reqBody = bytes.NewBuffer(nil)
	}
	req := httptest.NewRequestWithContext(context.Background(), method, path, reqBody)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// doServeRequestWithToken is doServeRequest plus an optional bearer token
// (empty string omits the Authorization header entirely), for exercising
// requireAuth's accept/reject paths.
func doServeRequestWithToken(t *testing.T, router *api.Router, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reqBody *bytes.Buffer
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request body: %v", err)
		}
		reqBody = bytes.NewBuffer(b)
	} else {
		reqBody = bytes.NewBuffer(nil)
	}
	req := httptest.NewRequestWithContext(context.Background(), method, path, reqBody)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestServe_E2E_TaskCreateAndShow(t *testing.T) {
	withTestLock(func() {
		_, cleanup := setupTestDir(t)
		defer cleanup()

		router := newTestServeRouter(t)

		rec := doServeRequest(t, router, "POST", "/tasks", taskCreateRequest{
			Title:       "Serve-created task",
			Description: "created via HTTP",
			Priority:    "P1",
		})
		if rec.Code != http.StatusCreated {
			t.Fatalf("POST /tasks: expected 201, got %d: %s", rec.Code, rec.Body.String())
		}

		var created core.Task
		if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
			t.Fatalf("unmarshal created task: %v", err)
		}
		if created.Title != "Serve-created task" {
			t.Errorf("expected title 'Serve-created task', got %q", created.Title)
		}
		if created.Priority != "P1" {
			t.Errorf("expected priority P1, got %q", created.Priority)
		}
		if created.Status != core.StatusTodo {
			t.Errorf("expected default status TODO, got %q", created.Status)
		}

		rec = doServeRequest(t, router, "GET", "/tasks/"+created.ID, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /tasks/{id}: expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var shown core.Task
		if err := json.Unmarshal(rec.Body.Bytes(), &shown); err != nil {
			t.Fatalf("unmarshal shown task: %v", err)
		}
		if shown.ID != created.ID {
			t.Errorf("expected id %q, got %q", created.ID, shown.ID)
		}
	})
}

func TestServe_E2E_TaskCreateValidationError(t *testing.T) {
	withTestLock(func() {
		_, cleanup := setupTestDir(t)
		defer cleanup()

		router := newTestServeRouter(t)

		rec := doServeRequest(t, router, "POST", "/tasks", taskCreateRequest{
			Title: "",
		})
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422 for empty title, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

func TestServe_E2E_TaskShowNotFound(t *testing.T) {
	withTestLock(func() {
		_, cleanup := setupTestDir(t)
		defer cleanup()

		router := newTestServeRouter(t)

		rec := doServeRequest(t, router, "GET", "/tasks/T-9999", nil)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for missing task, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

func TestServe_E2E_TaskList(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		for _, title := range []string{"Task A", "Task B"} {
			if err := s.CreateTask(ctx, &core.Task{
				ID: core.NewTaskID(), Title: title, Status: core.StatusTodo,
			}); err != nil {
				t.Fatalf("CreateTask: %v", err)
			}
		}

		router := newTestServeRouter(t)
		rec := doServeRequest(t, router, "GET", "/tasks?status=TODO", nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /tasks: expected 200, got %d: %s", rec.Code, rec.Body.String())
		}

		var resp taskListResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal list response: %v", err)
		}
		if resp.Total != 2 {
			t.Errorf("expected 2 tasks, got %d", resp.Total)
		}
	})
}

func TestServe_E2E_TaskUpdateStatusTransition(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		task := &core.Task{ID: core.NewTaskID(), Title: "To update", Status: core.StatusTodo}
		if err := s.CreateTask(ctx, task); err != nil {
			t.Fatalf("CreateTask: %v", err)
		}

		router := newTestServeRouter(t)

		newStatus := "IN_PROGRESS"
		rec := doServeRequest(t, router, "PATCH", "/tasks/"+task.ID, taskUpdateRequest{
			Status: &newStatus,
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("PATCH /tasks/{id}: expected 200, got %d: %s", rec.Code, rec.Body.String())
		}

		var updated core.Task
		if err := json.Unmarshal(rec.Body.Bytes(), &updated); err != nil {
			t.Fatalf("unmarshal updated task: %v", err)
		}
		if updated.Status != core.StatusInProgress {
			t.Errorf("expected status IN_PROGRESS, got %q", updated.Status)
		}

		// Invalid transition: IN_PROGRESS is not terminal, so re-claiming
		// via an unsupported status should be rejected by the workflow.
		bogusStatus := "NOT_A_REAL_STATUS"
		rec = doServeRequest(t, router, "PATCH", "/tasks/"+task.ID, taskUpdateRequest{
			Status: &bogusStatus,
		})
		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("expected 422 for unknown status, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

// TestServe_E2E_TaskUpdateExtendedFields exercises the fields that
// PATCH /tasks/{id} gained once handleTaskUpdate started sharing
// applyTaskFieldChanges with the CLI's `task update`: due/remind-at/
// rrule, track (with auto-create disabled over HTTP), and blocked-by
// add/remove. These were previously CLI-only.
func TestServe_E2E_TaskUpdateExtendedFields(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		blocker := &core.Task{ID: core.NewTaskID(), Title: "Blocker", Status: core.StatusTodo}
		if err := s.CreateTask(ctx, blocker); err != nil {
			t.Fatalf("CreateTask blocker: %v", err)
		}
		task := &core.Task{ID: core.NewTaskID(), Title: "Schedulable", Status: core.StatusTodo}
		if err := s.CreateTask(ctx, task); err != nil {
			t.Fatalf("CreateTask: %v", err)
		}

		router := newTestServeRouter(t)

		// POST /tracks so track_id resolves to something real; the HTTP
		// update route does not auto-create tracks (unlike the CLI).
		trackRec := doServeRequest(t, router, "POST", "/tracks", trackCreateRequest{
			Slug:  "serve-update-track",
			Title: "Serve Update Track",
			Type:  "feature",
		})
		if trackRec.Code != http.StatusCreated {
			t.Fatalf("POST /tracks: expected 201, got %d: %s", trackRec.Code, trackRec.Body.String())
		}
		var track core.Track
		if err := json.Unmarshal(trackRec.Body.Bytes(), &track); err != nil {
			t.Fatalf("unmarshal created track: %v", err)
		}

		due := "2099-01-01"
		remindAt := "2099-01-01"
		rrule := "FREQ=DAILY"
		rec := doServeRequest(t, router, "PATCH", "/tasks/"+task.ID, taskUpdateRequest{
			Due:          &due,
			RemindAt:     &remindAt,
			RRule:        &rrule,
			Track:        &track.ID,
			AddBlockedBy: []string{blocker.ID},
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("PATCH /tasks/{id} extended fields: expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var updated core.Task
		if err := json.Unmarshal(rec.Body.Bytes(), &updated); err != nil {
			t.Fatalf("unmarshal updated task: %v", err)
		}
		if updated.DueAt == nil {
			t.Error("expected DueAt to be set")
		}
		if updated.RemindAt == nil {
			t.Error("expected RemindAt to be set")
		}
		if updated.RRule != rrule {
			t.Errorf("expected rrule %q, got %q", rrule, updated.RRule)
		}
		if updated.TrackID == nil || *updated.TrackID != track.ID {
			t.Errorf("expected track_id %q, got %v", track.ID, updated.TrackID)
		}
		blockedBy := updated.BlockedBy()
		found := false
		for _, b := range blockedBy {
			if b == blocker.ID {
				found = true
			}
		}
		if !found {
			t.Errorf("expected blocked_by to contain %q, got %v", blocker.ID, blockedBy)
		}

		// Unlinking the track ("-") and clearing rrule/due/remind-at via
		// "-" mirrors the CLI's clear-sentinel convention exactly.
		dash := "-"
		rec = doServeRequest(t, router, "PATCH", "/tasks/"+task.ID, taskUpdateRequest{
			Due:      &dash,
			RemindAt: &dash,
			RRule:    &dash,
			Track:    &dash,
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("PATCH /tasks/{id} clear extended fields: expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var cleared core.Task
		if err := json.Unmarshal(rec.Body.Bytes(), &cleared); err != nil {
			t.Fatalf("unmarshal cleared task: %v", err)
		}
		if cleared.DueAt != nil {
			t.Error("expected DueAt cleared")
		}
		if cleared.RemindAt != nil {
			t.Error("expected RemindAt cleared")
		}
		if cleared.RRule != "" {
			t.Errorf("expected rrule cleared, got %q", cleared.RRule)
		}
		if cleared.TrackID != nil {
			t.Errorf("expected track unlinked, got %v", cleared.TrackID)
		}

		// An unresolvable track_id is a 422 over HTTP (no CLI-style
		// auto-create), the deliberate HTTP-vs-CLI behavior difference
		// noted on TaskFieldChanges.AutoCreateTrack.
		bogusTrack := "does-not-exist"
		rec = doServeRequest(t, router, "PATCH", "/tasks/"+task.ID, taskUpdateRequest{
			Track: &bogusTrack,
		})
		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("expected 422 for unresolvable track_id, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

func TestServe_E2E_TaskClaimAndComplete(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		task := &core.Task{ID: core.NewTaskID(), Title: "Claim me", Status: core.StatusTodo}
		if err := s.CreateTask(ctx, task); err != nil {
			t.Fatalf("CreateTask: %v", err)
		}

		router := newTestServeRouter(t)

		rec := doServeRequest(t, router, "POST", "/tasks/"+task.ID+"/claim", nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("POST /tasks/{id}/claim: expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var claimed core.Task
		if err := json.Unmarshal(rec.Body.Bytes(), &claimed); err != nil {
			t.Fatalf("unmarshal claimed task: %v", err)
		}
		if claimed.Status != core.StatusInProgress {
			t.Errorf("expected status IN_PROGRESS after claim, got %q", claimed.Status)
		}
		if claimed.AssignedTo == nil || *claimed.AssignedTo == "" {
			t.Error("expected assignee set after claim")
		}

		rec = doServeRequest(t, router, "POST", "/tasks/"+task.ID+"/complete", nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("POST /tasks/{id}/complete: expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var completed core.Task
		if err := json.Unmarshal(rec.Body.Bytes(), &completed); err != nil {
			t.Fatalf("unmarshal completed task: %v", err)
		}
		if completed.Status != core.StatusDone {
			t.Errorf("expected status DONE after complete, got %q", completed.Status)
		}

		// Re-completing an already-DONE task converges (same-status
		// transitions are always valid per WorkflowManager.ValidateTransition),
		// matching the CLI's documented idempotent `task complete` behavior.
		rec = doServeRequest(t, router, "POST", "/tasks/"+task.ID+"/complete", nil)
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200 (idempotent) re-completing a DONE task, got %d: %s", rec.Code, rec.Body.String())
		}

		// A genuinely invalid transition (DONE is terminal) should fail.
		claimRec := doServeRequest(t, router, "POST", "/tasks/"+task.ID+"/claim", nil)
		if claimRec.Code != http.StatusConflict {
			t.Errorf("expected 409 claiming a terminal DONE task, got %d: %s", claimRec.Code, claimRec.Body.String())
		}
	})
}

func TestServe_E2E_TaskCreateWithInvalidBlockedBy(t *testing.T) {
	withTestLock(func() {
		_, cleanup := setupTestDir(t)
		defer cleanup()

		router := newTestServeRouter(t)

		rec := doServeRequest(t, router, "POST", "/tasks", taskCreateRequest{
			Title:     "Blocked",
			BlockedBy: []string{"T-9999"},
		})
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422 for unresolvable blocked_by, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

func TestServe_E2E_TrackCreateListShow(t *testing.T) {
	withTestLock(func() {
		_, cleanup := setupTestDir(t)
		defer cleanup()

		router := newTestServeRouter(t)

		rec := doServeRequest(t, router, "POST", "/tracks", trackCreateRequest{
			Slug:  "serve-track",
			Title: "Serve Track",
			Type:  "feature",
		})
		if rec.Code != http.StatusCreated {
			t.Fatalf("POST /tracks: expected 201, got %d: %s", rec.Code, rec.Body.String())
		}
		var created core.Track
		if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
			t.Fatalf("unmarshal created track: %v", err)
		}
		if created.Slug != "serve-track" {
			t.Errorf("expected slug 'serve-track', got %q", created.Slug)
		}
		if created.Status != core.TrackStatusPending {
			t.Errorf("expected default status pending, got %q", created.Status)
		}

		rec = doServeRequest(t, router, "GET", "/tracks/"+created.ID, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /tracks/{id}: expected 200, got %d: %s", rec.Code, rec.Body.String())
		}

		rec = doServeRequest(t, router, "GET", "/tracks", nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /tracks: expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var listResp trackListResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &listResp); err != nil {
			t.Fatalf("unmarshal track list: %v", err)
		}
		if listResp.Total != 1 {
			t.Errorf("expected 1 track, got %d", listResp.Total)
		}
	})
}

func TestServe_E2E_TrackCreateInvalidSlug(t *testing.T) {
	withTestLock(func() {
		_, cleanup := setupTestDir(t)
		defer cleanup()

		router := newTestServeRouter(t)

		rec := doServeRequest(t, router, "POST", "/tracks", trackCreateRequest{
			Slug: "AB", // too short / uppercase — fails ValidateTrackSlug
		})
		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("expected 422 for invalid slug, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

// TestServe_E2E_HealthEndpoint asserts GET /health's response shape:
// 200 with status/pid/uptime_seconds keys, matching the handler runServe
// registers directly (see runServe's inline /health handler in serve.go).
func TestServe_E2E_HealthEndpoint(t *testing.T) {
	withTestLock(func() {
		_, cleanup := setupTestDir(t)
		defer cleanup()

		router, _ := newTestServeRouterWithAuth(t)

		rec := doServeRequest(t, router, "GET", "/health", nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /health: expected 200, got %d: %s", rec.Code, rec.Body.String())
		}

		var body map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("unmarshal health body: %v", err)
		}
		status, ok := body["status"].(string)
		if !ok || status != "ok" {
			t.Errorf("expected status \"ok\", got %v", body["status"])
		}
		pid, ok := body["pid"].(float64)
		if !ok || pid <= 0 {
			t.Errorf("expected pid > 0, got %v", body["pid"])
		}
		uptime, ok := body["uptime_seconds"].(float64)
		if !ok || uptime < 0 {
			t.Errorf("expected uptime_seconds >= 0, got %v", body["uptime_seconds"])
		}
	})
}

// TestServe_E2E_AuthRequiredForWrites confirms requireAuth rejects a
// protected (non-GET/HEAD) route with 401 when no bearer token is
// supplied and auth is enabled.
func TestServe_E2E_AuthRequiredForWrites(t *testing.T) {
	withTestLock(func() {
		_, cleanup := setupTestDir(t)
		defer cleanup()

		router, _ := newTestServeRouterWithAuth(t)

		rec := doServeRequestWithToken(t, router, "POST", "/tasks", "", taskCreateRequest{Title: "No token"})
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("POST /tasks without token: expected 401, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

// TestServe_E2E_AuthBypassedForReads confirms requireAuth's documented
// GET/HEAD bypass (see requireAuth's doc comment in serve.go: "accepts
// the supplied bearer token for non-GET/HEAD requests") — GET routes
// succeed with no token even when auth is enabled.
func TestServe_E2E_AuthBypassedForReads(t *testing.T) {
	withTestLock(func() {
		_, cleanup := setupTestDir(t)
		defer cleanup()

		router, _ := newTestServeRouterWithAuth(t)

		rec := doServeRequestWithToken(t, router, "GET", "/tasks", "", nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /tasks without token: expected 200, got %d: %s", rec.Code, rec.Body.String())
		}

		rec = doServeRequestWithToken(t, router, "GET", "/health", "", nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /health without token: expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

// TestServe_E2E_AuthCorrectTokenSucceeds confirms the correct bearer
// token is accepted on a protected route.
func TestServe_E2E_AuthCorrectTokenSucceeds(t *testing.T) {
	withTestLock(func() {
		_, cleanup := setupTestDir(t)
		defer cleanup()

		router, _ := newTestServeRouterWithAuth(t)

		rec := doServeRequestWithToken(t, router, "POST", "/tasks", testAuthToken, taskCreateRequest{Title: "Authed create"})
		if rec.Code != http.StatusCreated {
			t.Fatalf("POST /tasks with correct token: expected 201, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

// TestServe_E2E_ShutdownRequiresAuth covers POST /shutdown's own inline
// auth gate (serve.go's /shutdown handler checks the Authorization
// header directly rather than going through requireAuth, since /shutdown
// is itself the "control plane" route — see runServe): missing or wrong
// token is rejected with 401, and the shutdown signal never fires.
func TestServe_E2E_ShutdownRequiresAuth(t *testing.T) {
	withTestLock(func() {
		_, cleanup := setupTestDir(t)
		defer cleanup()

		router, canceled := newTestServeRouterWithAuth(t)

		rec := doServeRequestWithToken(t, router, "POST", "/shutdown", "", nil)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("POST /shutdown without token: expected 401, got %d: %s", rec.Code, rec.Body.String())
		}

		rec = doServeRequestWithToken(t, router, "POST", "/shutdown", "wrong-token", nil)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("POST /shutdown with wrong token: expected 401, got %d: %s", rec.Code, rec.Body.String())
		}

		select {
		case <-canceled:
			t.Fatal("shutdown signal fired despite unauthorized requests")
		default:
		}
	})
}

// TestServe_E2E_ShutdownWithCorrectToken covers the success path: the
// correct bearer token returns 204 and triggers the shutdown signal
// (mirroring runServe's `go cancel()` after writing the response).
func TestServe_E2E_ShutdownWithCorrectToken(t *testing.T) {
	withTestLock(func() {
		_, cleanup := setupTestDir(t)
		defer cleanup()

		router, canceled := newTestServeRouterWithAuth(t)

		rec := doServeRequestWithToken(t, router, "POST", "/shutdown", testAuthToken, nil)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("POST /shutdown with correct token: expected 204, got %d: %s", rec.Code, rec.Body.String())
		}

		select {
		case <-canceled:
		case <-time.After(time.Second):
			t.Fatal("expected shutdown signal after successful /shutdown")
		}
	})
}

// TestServe_AwaitShutdown_ServerErrorPropagates covers awaitServeShutdown's
// errCh-fires-first branch for a genuine (non-ErrServerClosed) error: it
// must propagate rather than being swallowed.
func TestServe_AwaitShutdown_ServerErrorPropagates(t *testing.T) {
	srv := &http.Server{}
	errCh := make(chan error, 1)
	wantErr := errors.New("boom: listener died")
	errCh <- wantErr

	err := awaitServeShutdown(context.Background(), srv, errCh)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected wrapped/equal %v, got %v", wantErr, err)
	}
}

// TestServe_AwaitShutdown_ErrServerClosedSwallowed covers awaitServeShutdown's
// errCh-fires-first branch for the expected http.ErrServerClosed sentinel
// (the signal srv.Serve returns after a concurrent Shutdown call): it must
// be swallowed, returning nil rather than propagating as a real error.
func TestServe_AwaitShutdown_ErrServerClosedSwallowed(t *testing.T) {
	srv := &http.Server{}
	errCh := make(chan error, 1)
	errCh <- http.ErrServerClosed

	err := awaitServeShutdown(context.Background(), srv, errCh)
	if err != nil {
		t.Fatalf("expected nil (ErrServerClosed swallowed), got %v", err)
	}
}

// TestServe_AwaitShutdown_CtxDoneShutsDownServer covers awaitServeShutdown's
// ctx.Done()-fires-first branch: with no listener ever bound, srv.Shutdown
// is still safe to call and returns nil (nothing to drain), so
// awaitServeShutdown returns nil.
func TestServe_AwaitShutdown_CtxDoneShutsDownServer(t *testing.T) {
	srv := &http.Server{}
	errCh := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := awaitServeShutdown(ctx, srv, errCh)
	if err != nil {
		t.Fatalf("expected nil after clean shutdown, got %v", err)
	}
}

// TestServe_E2E_TrackShowInvalidID covers the 422 side of the
// 404-vs-422 track-resolve distinction: resolveTrackID returns a plain
// (non-ErrTrackNotFound) error — routed to 422 by writeTrackResolveError
// — whenever the input is well-formed slug-shaped text that resolves to
// more than one candidate (ambiguous prefix match), as opposed to a
// well-formed input with zero matches (404, covered below).
func TestServe_E2E_TrackShowInvalidID(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		createTrack(t, ctx, s, "feat-alpha", "Alpha", core.TrackStatusActive)
		createTrack(t, ctx, s, "feat-beta", "Beta", core.TrackStatusActive)

		router := newTestServeRouter(t)

		rec := doServeRequest(t, router, "GET", "/tracks/feat", nil)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("GET /tracks/feat (ambiguous prefix): expected 422, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

// TestServe_E2E_TrackShowNotFound covers the 404 side of the
// 404-vs-422 distinction: a well-formed-but-nonexistent slug (no
// tracks at all, so resolveTrackID's zero-candidates path returns the
// typed ErrTrackNotFound) resolves to 404 via writeTrackResolveError.
func TestServe_E2E_TrackShowNotFound(t *testing.T) {
	withTestLock(func() {
		_, cleanup := setupTestDir(t)
		defer cleanup()

		router := newTestServeRouter(t)

		rec := doServeRequest(t, router, "GET", "/tracks/zzz-nonexistent", nil)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("GET /tracks/zzz-nonexistent: expected 404, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

// TestServe_E2E_BusPublishing verifies that task lifecycle events
// publish under tlc's own topic namespace (internal/events/topics.go)
// when a bus publisher is wired, exercising the same code path
// runServe wires via GetBusPublisher().
func TestServe_E2E_BusPublishing(t *testing.T) {
	withTestLock(func() {
		_, cleanup := setupTestDir(t)
		defer cleanup()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		var gotTopics []string
		fake := fakePublisher(func(_ context.Context, topic, _ string, _ any) error {
			gotTopics = append(gotTopics, topic)
			return nil
		})

		deps := &serveDeps{storage: s, publisher: fake}
		router := api.NewRouter()
		registerTaskRoutes(router, deps)

		rec := doServeRequest(t, router, "POST", "/tasks", taskCreateRequest{Title: "Bus test"})
		if rec.Code != http.StatusCreated {
			t.Fatalf("POST /tasks: expected 201, got %d: %s", rec.Code, rec.Body.String())
		}

		found := false
		for _, topic := range gotTopics {
			if topic == "tlc.task.created" {
				found = true
			}
		}
		if !found {
			t.Errorf("expected tlc.task.created to be published; got %v", gotTopics)
		}
	})
}

// fakePublisher adapts a plain func to api.EventPublisher for tests.
type fakePublisher func(ctx context.Context, topic, source string, payload any) error

func (f fakePublisher) Publish(ctx context.Context, topic, source string, payload any) error {
	return f(ctx, topic, source, payload)
}
