package core

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func writeScript(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	return path
}

func TestRunPlanExtractor_JSONOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell scripts not supported on Windows")
	}

	script := writeScript(t, "extract.sh", `#!/bin/sh
cat <<'JSONEOF'
[
  {"title": "Task A", "effort": "S", "priority": "P1"},
  {"title": "Task B", "blocked-by": [0]}
]
JSONEOF
`)

	specs, err := RunPlanExtractor(script, "/tmp/plan.md")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(specs) != 2 {
		t.Fatalf("specs len = %d, want 2", len(specs))
	}
	if specs[0].Title != "Task A" {
		t.Errorf("specs[0].Title = %q", specs[0].Title)
	}
	if specs[0].Effort != "S" {
		t.Errorf("specs[0].Effort = %q", specs[0].Effort)
	}
}

func TestRunPlanExtractor_YAMLOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell scripts not supported on Windows")
	}

	script := writeScript(t, "extract.sh", `#!/bin/sh
cat <<'YAMLEOF'
- title: "YAML Task"
  effort: M
  tags: [phase:1]
YAMLEOF
`)

	specs, err := RunPlanExtractor(script, "/tmp/plan.md")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("specs len = %d, want 1", len(specs))
	}
	if specs[0].Title != "YAML Task" {
		t.Errorf("specs[0].Title = %q", specs[0].Title)
	}
	if specs[0].Effort != "M" {
		t.Errorf("specs[0].Effort = %q", specs[0].Effort)
	}
}

func TestRunPlanExtractor_CommandNotFound(t *testing.T) {
	_, err := RunPlanExtractor(
		"/nonexistent/extractor", "/tmp/plan.md",
	)
	if err == nil {
		t.Fatal("expected error for nonexistent command")
	}
}

func TestRunPlanExtractor_EmptyOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell scripts not supported on Windows")
	}

	script := writeScript(t, "extract.sh", `#!/bin/sh
echo ""
`)

	specs, err := RunPlanExtractor(script, "/tmp/plan.md")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if specs != nil {
		t.Errorf("expected nil for empty output, got %v", specs)
	}
}

func TestRunPlanExtractor_EmptyCommand(t *testing.T) {
	specs, err := RunPlanExtractor("", "/tmp/plan.md")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if specs != nil {
		t.Errorf("expected nil for empty command, got %v", specs)
	}
}

func TestRunPlanExtractor_NullOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell scripts not supported on Windows")
	}

	script := writeScript(t, "extract.sh", `#!/bin/sh
echo "null"
`)

	specs, err := RunPlanExtractor(script, "/tmp/plan.md")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if specs != nil {
		t.Errorf("expected nil for null output, got %v", specs)
	}
}

func TestRunPlanExtractor_EmptyArrayOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell scripts not supported on Windows")
	}

	script := writeScript(t, "extract.sh", `#!/bin/sh
echo "[]"
`)

	specs, err := RunPlanExtractor(script, "/tmp/plan.md")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if specs != nil {
		t.Errorf("expected nil for empty array output, got %v", specs)
	}
}

func TestRunPlanExtractor_NonZeroExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell scripts not supported on Windows")
	}

	script := writeScript(t, "extract.sh", `#!/bin/sh
exit 1
`)

	_, err := RunPlanExtractor(script, "/tmp/plan.md")
	if err == nil {
		t.Fatal("expected error for non-zero exit")
	}
}
