package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
	"hop.top/tlc/internal/core"
)

// TestAppendAuditLog_FirstEntry_WithNote verifies audit log with note on first entry.
func TestAppendAuditLog_FirstEntry_WithNote(t *testing.T) {
	task := &core.Task{Description: "Original description."}
	ts := time.Date(2026, 3, 8, 12, 54, 0, 0, time.UTC)

	appendAuditLog(task, "jad", "CLAIMED", "(TODO → IN_PROGRESS, assigned to @jad)", "Starting work", ts)

	if !contains(task.Description, "Original description.") {
		t.Errorf("original description lost, got: %s", task.Description)
	}
	if !contains(task.Description, "---") {
		t.Errorf("expected --- separator, got: %s", task.Description)
	}
	if !contains(task.Description, "2026-03-08 12:54") {
		t.Errorf("expected timestamp, got: %s", task.Description)
	}
	if !contains(task.Description, "@jad") {
		t.Errorf("expected author, got: %s", task.Description)
	}
	if !contains(task.Description, "CLAIMED") {
		t.Errorf("expected action, got: %s", task.Description)
	}
	if !contains(task.Description, "Starting work") {
		t.Errorf("expected note, got: %s", task.Description)
	}
}

// TestAppendAuditLog_FirstEntry_NoNote verifies audit log without note on first entry.
func TestAppendAuditLog_FirstEntry_NoNote(t *testing.T) {
	task := &core.Task{Description: "Original description."}
	ts := time.Date(2026, 3, 8, 12, 54, 0, 0, time.UTC)

	appendAuditLog(task, "jad", "CLAIMED", "(TODO → IN_PROGRESS, assigned to @jad)", "", ts)

	if !contains(task.Description, "2026-03-08 12:54") {
		t.Errorf("expected timestamp, got: %s", task.Description)
	}
	// Should NOT have trailing newline after header when no note
	lines := strings.Split(task.Description, "\n")
	lastLine := lines[len(lines)-1]
	if lastLine == "" {
		t.Errorf("should not have trailing blank line when no note, got: %q", task.Description)
	}
}

// TestAppendAuditLog_EmptyDescription verifies audit log with empty description.
func TestAppendAuditLog_EmptyDescription(t *testing.T) {
	task := &core.Task{Description: ""}
	ts := time.Date(2026, 3, 8, 12, 54, 0, 0, time.UTC)

	appendAuditLog(task, "jad", "CLAIMED", "(TODO → IN_PROGRESS)", "Starting work", ts)

	// Should still have --- before entry
	if !strings.HasPrefix(task.Description, "---") {
		t.Errorf("expected --- prefix for empty description, got: %s", task.Description)
	}
	if !contains(task.Description, "Starting work") {
		t.Errorf("expected note, got: %s", task.Description)
	}
}

// TestAppendAuditLog_MultipleEntries verifies multiple audit log entries.
func TestAppendAuditLog_MultipleEntries(t *testing.T) {
	task := &core.Task{Description: "Rate limiting endpoint."}
	ts1 := time.Date(2026, 3, 8, 12, 54, 0, 0, time.UTC)
	ts2 := time.Date(2026, 3, 8, 14, 0, 0, 0, time.UTC)
	ts3 := time.Date(2026, 3, 8, 14, 30, 0, 0, time.UTC)

	appendAuditLog(task, "jad", "CLAIMED", "(TODO → IN_PROGRESS, assigned to @jad)", "", ts1)
	appendAuditLog(task, "jad", "COMPLETED", "(IN_PROGRESS → DONE)", "First pass done", ts2)
	appendAuditLog(task, "lead", "REOPENED", "(DONE → TODO)", "Missing tests", ts3)

	// Count --- separators: 1 (desc/audit boundary) + 2 (between entries) = 3
	separatorCount := strings.Count(task.Description, "---")
	if separatorCount != 3 {
		t.Errorf("expected 3 --- separators, got %d in: %s", separatorCount, task.Description)
	}
	if !contains(task.Description, "Rate limiting endpoint.") {
		t.Error("original description lost")
	}
	if !contains(task.Description, "@lead") {
		t.Error("missing third entry author")
	}
	if !contains(task.Description, "Missing tests") {
		t.Error("missing third entry note")
	}
}

// TestAppendAuditLog_CustomSeparator_Empty verifies empty custom separator config.
func TestAppendAuditLog_CustomSeparator_Empty(t *testing.T) {
	viper.Set("audit_log.separator", "")
	defer viper.Set("audit_log.separator", nil)

	task := &core.Task{Description: "Original."}
	ts1 := time.Date(2026, 3, 8, 12, 0, 0, 0, time.UTC)
	ts2 := time.Date(2026, 3, 8, 13, 0, 0, 0, time.UTC)

	appendAuditLog(task, "jad", "CLAIMED", "(TODO → IN_PROGRESS)", "", ts1)
	appendAuditLog(task, "jad", "COMPLETED", "(IN_PROGRESS → DONE)", "Done", ts2)

	// First separator (desc/audit boundary) is always ---
	// But between entries there should be no ---
	parts := strings.SplitN(task.Description, "---", 2)
	if len(parts) < 2 {
		t.Fatalf("expected at least one --- (desc boundary), got: %s", task.Description)
	}
	auditBlock := parts[1]
	if strings.Contains(auditBlock, "---") {
		t.Errorf("empty separator should produce no --- between entries, got: %s", auditBlock)
	}
}

// TestAppendAuditLog_CustomTemplate verifies custom template and timestamp format.
func TestAppendAuditLog_CustomTemplate(t *testing.T) {
	viper.Set("audit_log.template", "[{{.Timestamp}}] {{.Author}}: {{.Action}}{{if .Note}} - {{.Note}}{{end}}")
	viper.Set("audit_log.timestamp_format", "Jan 02 15:04")
	defer func() {
		viper.Set("audit_log.template", nil)
		viper.Set("audit_log.timestamp_format", nil)
	}()

	task := &core.Task{Description: ""}
	ts := time.Date(2026, 3, 8, 12, 54, 0, 0, time.UTC)

	appendAuditLog(task, "jad", "CLAIMED", "(TODO → IN_PROGRESS)", "Starting", ts)

	if !contains(task.Description, "[Mar 08 12:54]") {
		t.Errorf("expected custom timestamp format, got: %s", task.Description)
	}
	if !contains(task.Description, "jad: CLAIMED") {
		t.Errorf("expected custom template output, got: %s", task.Description)
	}
}
