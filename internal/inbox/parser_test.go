package inbox

import (
	"strings"
	"testing"

	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/core"
)

// --- ParseCreateJSON tests ---

func TestParseCreateJSON_AllFields(t *testing.T) {
	data := []byte(`{
		"title": "Wire inbox watcher",
		"description": "Implement fsnotify loop",
		"status": "IN_PROGRESS",
		"assigned_to": "exo",
		"tags": ["feat", "inbox"],
		"effort": "M",
		"priority": "P1",
		"track_id": "inbox-protocol"
	}`)

	r, err := ParseCreateJSON(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertEquals(t, "Title", r.Title, "Wire inbox watcher")
	assertEquals(t, "Description", r.Description, "Implement fsnotify loop")
	assertEquals(t, "Status", r.Status, "IN_PROGRESS")
	assertEquals(t, "AssignedTo", r.AssignedTo, "exo")
	assertEquals(t, "Effort", r.Effort, "M")
	assertEquals(t, "Priority", r.Priority, "P1")
	assertEquals(t, "TrackID", r.TrackID, "inbox-protocol")

	if len(r.Tags) != 2 || r.Tags[0] != "feat" || r.Tags[1] != "inbox" {
		t.Errorf("Tags: got %v, want [feat inbox]", r.Tags)
	}
}

func TestParseCreateJSON_TitleOnly(t *testing.T) {
	data := []byte(`{"title": "Quick task"}`)

	r, err := ParseCreateJSON(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertEquals(t, "Title", r.Title, "Quick task")
	assertEquals(t, "Status", r.Status, "TODO")
	assertEquals(t, "Description", r.Description, "")
	assertEquals(t, "AssignedTo", r.AssignedTo, "")
	assertEquals(t, "Effort", r.Effort, "")
	assertEquals(t, "Priority", r.Priority, "")

	if len(r.Tags) != 0 {
		t.Errorf("Tags: got %v, want empty", r.Tags)
	}
}

func TestParseCreateJSON_MissingTitle(t *testing.T) {
	data := []byte(`{"description": "no title here"}`)

	_, err := ParseCreateJSON(data)
	if err == nil {
		t.Fatal("expected error for missing title")
	}
	if !strings.Contains(err.Error(), "title is required") {
		t.Errorf("error should mention title: %v", err)
	}
}

func TestParseCreateJSON_EmptyTitle(t *testing.T) {
	data := []byte(`{"title": "  "}`)

	_, err := ParseCreateJSON(data)
	if err == nil {
		t.Fatal("expected error for empty title")
	}
	if !strings.Contains(err.Error(), "title is required") {
		t.Errorf("error should mention title: %v", err)
	}
}

func TestParseCreateJSON_InvalidJSON(t *testing.T) {
	data := []byte(`{not valid}`)

	_, err := ParseCreateJSON(data)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

// --- ParseCreateMarkdown tests ---

func TestParseCreateMarkdown_FrontmatterAndBody(t *testing.T) {
	data := []byte("---\ntitle: Build parser\nstatus: TODO\n" +
		"tags: [feat]\neffort: M\npriority: P1\n---\n" +
		"Description body here.\n\nWith multiple paragraphs.\n")

	r, err := ParseCreateMarkdown(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertEquals(t, "Title", r.Title, "Build parser")
	assertEquals(t, "Status", r.Status, "TODO")
	assertEquals(t, "Effort", r.Effort, "M")
	assertEquals(t, "Priority", r.Priority, "P1")
	assertEquals(
		t, "Description", r.Description,
		"Description body here.\n\nWith multiple paragraphs.",
	)

	if len(r.Tags) != 1 || r.Tags[0] != "feat" {
		t.Errorf("Tags: got %v, want [feat]", r.Tags)
	}
}

func TestParseCreateMarkdown_TitleOnlyFrontmatter(t *testing.T) {
	data := []byte("---\ntitle: Minimal task\n---\n")

	r, err := ParseCreateMarkdown(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertEquals(t, "Title", r.Title, "Minimal task")
	assertEquals(t, "Status", r.Status, "TODO")
	assertEquals(t, "Description", r.Description, "")
}

func TestParseCreateMarkdown_MissingTitle(t *testing.T) {
	data := []byte("---\nstatus: TODO\n---\nSome body.\n")

	_, err := ParseCreateMarkdown(data)
	if err == nil {
		t.Fatal("expected error for missing title")
	}
	if !strings.Contains(err.Error(), "title is required") {
		t.Errorf("error should mention title: %v", err)
	}
}

func TestParseCreateMarkdown_NoFrontmatter(t *testing.T) {
	data := []byte("Just a plain markdown file.\n")

	_, err := ParseCreateMarkdown(data)
	if err == nil {
		t.Fatal("expected error for missing frontmatter")
	}
}

// --- ParseTransitionJSON tests ---

func TestParseTransitionJSON_Valid(t *testing.T) {
	data := []byte(`{
		"id": "T-0042",
		"status": "IN_PROGRESS",
		"note": "claiming",
		"by": "agent-x"
	}`)

	tr, err := ParseTransitionJSON(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertEquals(t, "ID", tr.ID, "T-0042")
	assertEquals(t, "Status", tr.Status, "IN_PROGRESS")
	assertEquals(t, "Note", tr.Note, "claiming")
	assertEquals(t, "By", tr.By, "agent-x")
}

func TestParseTransitionJSON_DefaultBy(t *testing.T) {
	data := []byte(`{"id": "T-0001", "status": "DONE"}`)

	tr, err := ParseTransitionJSON(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertEquals(t, "By", tr.By, "inbox")
}

func TestParseTransitionJSON_MissingID(t *testing.T) {
	data := []byte(`{"status": "DONE"}`)

	_, err := ParseTransitionJSON(data)
	if err == nil {
		t.Fatal("expected error for missing id")
	}
	if !strings.Contains(err.Error(), "id is required") {
		t.Errorf("error should mention id: %v", err)
	}
}

func TestParseTransitionJSON_MissingStatus(t *testing.T) {
	data := []byte(`{"id": "T-0042"}`)

	_, err := ParseTransitionJSON(data)
	if err == nil {
		t.Fatal("expected error for missing status")
	}
	if !strings.Contains(err.Error(), "status is required") {
		t.Errorf("error should mention status: %v", err)
	}
}

// --- effort vocabulary ---

// withEfforts declares an effort vocabulary for the duration of one test.
func withEfforts(t *testing.T, names ...string) {
	t.Helper()
	defs := make([]config.EffortDefinition, 0, len(names))
	for _, n := range names {
		defs = append(defs, config.EffortDefinition{Name: n})
	}
	core.SetTaskConfigProvider(func() *config.TaskConfig {
		return &config.TaskConfig{Efforts: defs}
	})
	core.ResetDefaultWorkflow()
	t.Cleanup(func() {
		core.SetTaskConfigProvider(nil)
		core.ResetDefaultWorkflow()
	})
}

// TestParseCreateJSON_ConfiguredEffortAccepted pins that intake honors
// the user's declared vocabulary. The inbox gate is a separate call site
// from the CLI's normaliser, so a size the CLI accepts must not be
// rejected on the way in from a file drop.
func TestParseCreateJSON_ConfiguredEffortAccepted(t *testing.T) {
	withEfforts(t, "TINY", "SMALL", "BIG")

	data := []byte(`{"title": "sized", "effort": "TINY"}`)
	r, err := ParseCreateJSON(data)
	if err != nil {
		t.Fatalf("declared effort rejected: %v", err)
	}
	assertEquals(t, "Effort", r.Effort, "TINY")
}

// TestParseCreateJSON_UnknownEffortNamesConfiguredSet pins the rejection
// message to the configured vocabulary. It used to retype "XS, S, M, L,
// XL" as a literal, which would tell a user with TINY declared that the
// sizes they replaced are the only legal ones.
func TestParseCreateJSON_UnknownEffortNamesConfiguredSet(t *testing.T) {
	withEfforts(t, "TINY", "SMALL", "BIG")

	data := []byte(`{"title": "sized", "effort": "XS"}`)
	_, err := ParseCreateJSON(data)
	if err == nil {
		t.Fatal("expected an undeclared effort to be rejected")
	}
	for _, want := range []string{"TINY", "SMALL", "BIG"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should name %q, got: %v", want, err)
		}
	}
	if strings.Contains(err.Error(), "XL") {
		t.Errorf("error leaked the built-in set: %v", err)
	}
}

// --- status vocabulary ---

// withStatuses declares a task-status vocabulary for one test.
func withStatuses(t *testing.T, names ...string) {
	t.Helper()
	defs := make([]config.StatusDefinition, 0, len(names))
	for _, n := range names {
		defs = append(defs, config.StatusDefinition{Name: n})
	}
	core.SetTaskConfigProvider(func() *config.TaskConfig {
		return &config.TaskConfig{Statuses: defs}
	})
	core.ResetDefaultWorkflow()
	t.Cleanup(func() {
		core.SetTaskConfigProvider(nil)
		core.ResetDefaultWorkflow()
	})
}

// TestParseCreateJSON_ConfiguredStatusAccepted pins the create gate to the
// declared vocabulary. The inbox kept its own hardcoded status map long
// after core.ValidTaskStatus became config-aware, so a project declaring
// IN_REVIEW had the CLI accept the status and the file drop reject it.
func TestParseCreateJSON_ConfiguredStatusAccepted(t *testing.T) {
	withStatuses(t, "TODO", "IN_REVIEW", "DONE")

	data := []byte(`{"title": "reviewed", "status": "IN_REVIEW"}`)
	r, err := ParseCreateJSON(data)
	if err != nil {
		t.Fatalf("declared status rejected: %v", err)
	}
	assertEquals(t, "Status", r.Status, "IN_REVIEW")
}

// TestParseCreateJSON_UnknownStatusNamesConfiguredSet pins the create
// rejection message to the configured vocabulary rather than the literal
// "TODO, IN_PROGRESS, DONE, SKIPPED" it used to retype.
func TestParseCreateJSON_UnknownStatusNamesConfiguredSet(t *testing.T) {
	withStatuses(t, "TODO", "IN_REVIEW", "DONE")

	data := []byte(`{"title": "x", "status": "SKIPPED"}`)
	_, err := ParseCreateJSON(data)
	if err == nil {
		t.Fatal("expected an undeclared status to be rejected")
	}
	if !strings.Contains(err.Error(), "IN_REVIEW") {
		t.Errorf("error should name the configured set, got: %v", err)
	}
	if strings.Contains(err.Error(), "IN_PROGRESS") {
		t.Errorf("error leaked the built-in set: %v", err)
	}
}

// TestParseCreateMarkdown_ConfiguredStatusAccepted covers the markdown
// intake, which reaches the same gate by a different route.
func TestParseCreateMarkdown_ConfiguredStatusAccepted(t *testing.T) {
	withStatuses(t, "TODO", "IN_REVIEW", "DONE")

	data := []byte("---\ntitle: reviewed\nstatus: IN_REVIEW\n---\nbody\n")
	r, err := ParseCreateMarkdown(data)
	if err != nil {
		t.Fatalf("declared status rejected: %v", err)
	}
	assertEquals(t, "Status", r.Status, "IN_REVIEW")
}

// TestParseTransitionJSON_ConfiguredStatusAccepted covers the SECOND
// status gate. Create and transition validated through separate copies of
// the same hardcoded map, so a fix applied to one of them would leave a
// declared status creatable but not transitional.
func TestParseTransitionJSON_ConfiguredStatusAccepted(t *testing.T) {
	withStatuses(t, "TODO", "IN_REVIEW", "DONE")

	data := []byte(`{"id": "T-0042", "status": "IN_REVIEW"}`)
	tr, err := ParseTransitionJSON(data)
	if err != nil {
		t.Fatalf("declared status rejected: %v", err)
	}
	assertEquals(t, "Status", tr.Status, "IN_REVIEW")
}

// TestParseTransitionJSON_UnknownStatusNamesConfiguredSet pins the
// transition rejection message to the configured vocabulary.
func TestParseTransitionJSON_UnknownStatusNamesConfiguredSet(t *testing.T) {
	withStatuses(t, "TODO", "IN_REVIEW", "DONE")

	data := []byte(`{"id": "T-0042", "status": "IN_PROGRESS"}`)
	_, err := ParseTransitionJSON(data)
	if err == nil {
		t.Fatal("expected an undeclared status to be rejected")
	}
	if !strings.Contains(err.Error(), "IN_REVIEW") {
		t.Errorf("error should name the configured set, got: %v", err)
	}
	if strings.Contains(err.Error(), "SKIPPED") {
		t.Errorf("error leaked the built-in set: %v", err)
	}
}

// TestStatusErrorsUnderDefaultConfig pins that a config declaring no
// statuses is indistinguishable from before this became config-driven:
// the built-in four accepted, and named verbatim in both rejections.
func TestStatusErrorsUnderDefaultConfig(t *testing.T) {
	const builtins = "TODO, IN_PROGRESS, DONE, SKIPPED"

	for _, s := range []string{"TODO", "IN_PROGRESS", "DONE", "SKIPPED"} {
		if _, err := ParseCreateJSON(
			[]byte(`{"title": "x", "status": "` + s + `"}`),
		); err != nil {
			t.Errorf("built-in status %q rejected: %v", s, err)
		}
		if _, err := ParseTransitionJSON(
			[]byte(`{"id": "T-1", "status": "` + s + `"}`),
		); err != nil {
			t.Errorf("built-in status %q rejected on transition: %v", s, err)
		}
	}

	_, err := ParseCreateJSON([]byte(`{"title": "x", "status": "NOPE"}`))
	if err == nil || !strings.Contains(err.Error(), builtins) {
		t.Errorf("create error should read %q, got: %v", builtins, err)
	}
	_, err = ParseTransitionJSON([]byte(`{"id": "T-1", "status": "NOPE"}`))
	if err == nil || !strings.Contains(err.Error(), builtins) {
		t.Errorf("transition error should read %q, got: %v", builtins, err)
	}
}

// --- priority vocabulary ---

// withPriorities declares a priority vocabulary for one test.
func withPriorities(t *testing.T, names ...string) {
	t.Helper()
	defs := make([]config.PriorityDefinition, 0, len(names))
	for _, n := range names {
		defs = append(defs, config.PriorityDefinition{Name: n})
	}
	core.SetTaskConfigProvider(func() *config.TaskConfig {
		return &config.TaskConfig{Priorities: defs}
	})
	core.ResetDefaultWorkflow()
	t.Cleanup(func() {
		core.SetTaskConfigProvider(nil)
		core.ResetDefaultWorkflow()
	})
}

// TestParseCreateJSON_UnknownPriorityNamesConfiguredSet pins the priority
// rejection to the configured vocabulary. Validation here was already
// correct — core.ValidPriority reads config — but the message retyped the
// set beside it, so the two could disagree.
func TestParseCreateJSON_UnknownPriorityNamesConfiguredSet(t *testing.T) {
	withPriorities(t, "URGENT", "NORMAL", "LATER")

	data := []byte(`{"title": "x", "priority": "P1"}`)
	_, err := ParseCreateJSON(data)
	if err == nil {
		t.Fatal("expected an undeclared priority to be rejected")
	}
	for _, want := range []string{"URGENT", "NORMAL", "LATER"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should name %q, got: %v", want, err)
		}
	}
}

// TestPriorityErrorNeverMentionsP4 pins the typo that shipped
// independently of any config work: the literal read "P0, P1, P2, P3, P4"
// and tlc has never had a P4.
func TestPriorityErrorNeverMentionsP4(t *testing.T) {
	_, err := ParseCreateJSON([]byte(`{"title": "x", "priority": "P9"}`))
	if err == nil {
		t.Fatal("expected an invalid priority to be rejected")
	}
	if strings.Contains(err.Error(), "P4") {
		t.Errorf("error still advertises the nonexistent P4: %v", err)
	}
	const builtins = "P0, P1, P2, P3"
	if !strings.Contains(err.Error(), builtins) {
		t.Errorf("error should read %q, got: %v", builtins, err)
	}
}

// --- helpers ---

func assertEquals(t *testing.T, field, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("%s: got %q, want %q", field, got, want)
	}
}

// TestParseCreateJSON_DefaultStatusIsConfigured pins the create DEFAULT
// to the same resolver `tlc task create` uses.
//
// The validation gate was moved onto core.ValidTaskStatus, but the
// default it feeds was left as a literal "TODO". A project declaring
// OPEN/DOING/SHIPPED therefore got a file drop that wrote a status
// outside its own vocabulary, which the state machine then had no
// transition out of — the same "CLI accepts, inbox disagrees" split the
// gate was fixed to close, one line lower.
func TestParseCreateJSON_DefaultStatusIsConfigured(t *testing.T) {
	withInitialStatus(t, "OPEN", "DOING", "SHIPPED")

	data := []byte(`{"title": "no status given"}`)
	r, err := ParseCreateJSON(data)
	if err != nil {
		t.Fatalf("create with no status rejected: %v", err)
	}
	assertEquals(t, "Status", r.Status, "OPEN")
}

// TestParseCreateMarkdown_DefaultStatusIsConfigured covers the second
// intake shape. Both parsers call applyCreateDefaults, so fixing one and
// not the other would leave a markdown drop writing the literal.
func TestParseCreateMarkdown_DefaultStatusIsConfigured(t *testing.T) {
	withInitialStatus(t, "OPEN", "DOING", "SHIPPED")

	data := []byte("---\ntitle: no status given\n---\nBody.\n")
	r, err := ParseCreateMarkdown(data)
	if err != nil {
		t.Fatalf("markdown create with no status rejected: %v", err)
	}
	assertEquals(t, "Status", r.Status, "OPEN")
}

// TestParseCreateJSON_DefaultStatusFallsBackToBuiltin pins the no-config
// path. A library consumer that registers no provider must still get a
// usable status rather than an empty one, so the built-in stays as the
// fallback rather than the literal being merely deleted.
func TestParseCreateJSON_DefaultStatusFallsBackToBuiltin(t *testing.T) {
	core.SetTaskConfigProvider(nil)
	core.ResetDefaultWorkflow()
	t.Cleanup(core.ResetDefaultWorkflow)

	data := []byte(`{"title": "no status, no config"}`)
	r, err := ParseCreateJSON(data)
	if err != nil {
		t.Fatalf("create with no status rejected: %v", err)
	}
	if r.Status == "" {
		t.Fatal("default status resolved to empty")
	}
	if !core.ValidTaskStatus(core.TaskStatus(r.Status)) {
		t.Errorf("default status %q is not in the vocabulary", r.Status)
	}
}

// withInitialStatus declares a status vocabulary whose FIRST entry
// carries the initial role, so the configured default is a name the
// built-in set does not contain.
func withInitialStatus(t *testing.T, names ...string) {
	t.Helper()
	defs := make([]config.StatusDefinition, 0, len(names))
	for i, n := range names {
		d := config.StatusDefinition{Name: n}
		if i == 0 {
			d.Role = config.RoleInitial
		}
		defs = append(defs, d)
	}
	core.SetTaskConfigProvider(func() *config.TaskConfig {
		return &config.TaskConfig{Statuses: defs}
	})
	core.ResetDefaultWorkflow()
	t.Cleanup(func() {
		core.SetTaskConfigProvider(nil)
		core.ResetDefaultWorkflow()
	})
}
