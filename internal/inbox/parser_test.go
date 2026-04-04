package inbox

import (
	"strings"
	"testing"
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

// --- helpers ---

func assertEquals(t *testing.T, field, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("%s: got %q, want %q", field, got, want)
	}
}
