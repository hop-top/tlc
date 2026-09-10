package cli

// The HTTP create path must land a status-less task in the SAME status
// the CLI's `tlc task create` would choose. Both write to one store, so
// two answers is a bug regardless of which one is "right".
//
// Vehicle is a fully renamed vocabulary (BACKLOG/DOING/SHIPPED) with no
// TODO in it: a create that defaults to the BUILT-IN literal is then not
// merely odd, it is rejected outright by the configured-vocabulary
// normaliser, so the assertion cannot pass by accident.

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"hop.top/kit/go/transport/api"
	"hop.top/tlc/internal/core"
)

// renamedVocabYAML declares a status vocabulary containing none of the
// built-in names. The initial-role status is BACKLOG.
const renamedVocabYAML = `storage:
  backend: sqlite
  db_path: %DB%
task:
  statuses:
    - name: BACKLOG
      label: Backlog
      is_terminal: false
      role: initial
      tls_marker: " "
    - name: DOING
      label: Doing
      is_terminal: false
      role: active
      tls_marker: ">"
    - name: SHIPPED
      label: Shipped
      is_terminal: true
      role: completed
      tls_marker: "x"
  state_machine:
    rules:
      BACKLOG: [DOING]
      DOING: [SHIPPED, BACKLOG]
`

// loadServeVocabConfig points viper at a renamed-vocabulary config whose
// storage.db_path is a fresh temp DB, then clears the workflow singleton
// and the synonym cache so the next lookup rebuilds from it.
//
// Writes a real config file rather than only viper.Set calls: the status
// list is a nested structure that the config unmarshal path has to walk,
// and setting it in memory would skip exactly the code the HTTP handler
// depends on.
func loadServeVocabConfig(t *testing.T, yamlBody string) {
	t.Helper()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.sqlite")
	body := []byte(strings.ReplaceAll(yamlBody, "%DB%", dbPath))
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	viper.Reset()
	viper.SetConfigFile(path)
	if err := viper.ReadInConfig(); err != nil {
		t.Fatalf("read config: %v", err)
	}

	core.ResetDefaultWorkflow()
	ResetSynonymCache()
	t.Cleanup(func() {
		viper.Reset()
		core.ResetDefaultWorkflow()
		ResetSynonymCache()
	})
}

// TestServeTaskCreate_DefaultStatusFollowsConfiguredVocabulary is the
// regression guard: POST /tasks with no status under a renamed
// vocabulary must succeed and land in the configured initial status,
// not 422 on a built-in literal the config never declared.
func TestServeTaskCreate_DefaultStatusFollowsConfiguredVocabulary(t *testing.T) {
	loadServeVocabConfig(t, renamedVocabYAML)

	s, err := getStorageRaw()
	if err != nil {
		t.Fatalf("getStorageRaw: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	router := api.NewRouter()
	registerTaskRoutes(router, &serveDeps{storage: s})

	rec := doServeRequest(t, router, "POST", "/tasks", taskCreateRequest{Title: "no status supplied"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /tasks with no status: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var created core.Task
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if created.Status != core.TaskStatus("BACKLOG") {
		t.Errorf("created status = %q, want %q (configured initial-role status)", created.Status, "BACKLOG")
	}

	// The row that actually landed, not just the response echo: a
	// handler could shape the JSON correctly and still persist the
	// built-in literal.
	stored, err := s.GetTask(t.Context(), created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if stored == nil {
		t.Fatal("created task not found in storage")
	}
	if stored.Status != core.TaskStatus("BACKLOG") {
		t.Errorf("stored status = %q, want %q", stored.Status, "BACKLOG")
	}
}

// TestServeTaskCreate_DefaultStatusMatchesCLIResolution pins the two
// create paths to ONE answer rather than to a hardcoded expectation:
// whatever resolveInitialStatus gives the CLI is what the route must
// produce. A future change to the resolution order that touches only one
// path fails here even if both remain individually plausible.
func TestServeTaskCreate_DefaultStatusMatchesCLIResolution(t *testing.T) {
	loadServeVocabConfig(t, renamedVocabYAML)

	want, err := resolveInitialStatus("")
	if err != nil {
		t.Fatalf("resolveInitialStatus: %v", err)
	}

	s, err := getStorageRaw()
	if err != nil {
		t.Fatalf("getStorageRaw: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	router := api.NewRouter()
	registerTaskRoutes(router, &serveDeps{storage: s})

	rec := doServeRequest(t, router, "POST", "/tasks", taskCreateRequest{Title: "parity probe"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /tasks: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var created core.Task
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if string(created.Status) != want {
		t.Errorf("HTTP create status = %q, CLI resolveInitialStatus = %q; the two create paths disagree",
			created.Status, want)
	}
}

// TestServeTaskCreate_ExplicitStatusStillValidated guards the other
// half: routing the DEFAULT through config must not stop an explicitly
// supplied bogus status from being rejected.
func TestServeTaskCreate_ExplicitStatusStillValidated(t *testing.T) {
	loadServeVocabConfig(t, renamedVocabYAML)

	s, err := getStorageRaw()
	if err != nil {
		t.Fatalf("getStorageRaw: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	router := api.NewRouter()
	registerTaskRoutes(router, &serveDeps{storage: s})

	rec := doServeRequest(t, router, "POST", "/tasks",
		taskCreateRequest{Title: "bogus", Status: "NOT_A_STATUS"})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("POST /tasks with bogus status: expected 422, got %d: %s", rec.Code, rec.Body.String())
	}
}
