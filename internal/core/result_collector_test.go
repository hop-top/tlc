package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeResultFile(t *testing.T, dir string, r AgentResult) string {
	t.Helper()
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	path := filepath.Join(dir, "results.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write results: %v", err)
	}
	return path
}

func validStdoutJSON(status AgentResultStatus, summary string) string {
	r := AgentResult{
		Version:  1,
		Status:   status,
		ExitCode: 0,
		Summary:  summary,
	}
	data, _ := json.Marshal(r)
	return string(data)
}

func TestResultCollector_StdoutSucceeded_VolumeExists(t *testing.T) {
	dir := t.TempDir()
	volResult := AgentResult{
		Version:  1,
		Status:   AgentStatusSucceeded,
		ExitCode: 0,
		Summary:  "volume summary",
		Agent:    "claude",
		Artifacts: []AgentArtifact{
			{Path: "main.go", Type: "modified"},
		},
	}
	path := writeResultFile(t, dir, volResult)
	stdout := validStdoutJSON(AgentStatusSucceeded, "stdout summary")

	c := NewResultCollector()
	got, err := c.CollectFromExec(stdout, path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Stdout is authoritative for status/summary; volume provides artifacts.
	if got.Summary != "stdout summary" {
		t.Errorf("summary = %q, want stdout summary", got.Summary)
	}
	if len(got.Artifacts) != 1 {
		t.Errorf("artifacts len = %d, want 1", len(got.Artifacts))
	}
}

func TestResultCollector_StdoutSucceeded_NoVolume(t *testing.T) {
	stdout := validStdoutJSON(AgentStatusSucceeded, "stdout only")

	c := NewResultCollector()
	got, err := c.CollectFromExec(stdout, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Summary != "stdout only" {
		t.Errorf("summary = %q", got.Summary)
	}
}

func TestResultCollector_StdoutSucceeded_VolumeMissing(t *testing.T) {
	stdout := validStdoutJSON(AgentStatusSucceeded, "stdout only")

	c := NewResultCollector()
	got, err := c.CollectFromExec(stdout, "/nonexistent/results.json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Summary != "stdout only" {
		t.Errorf("summary = %q", got.Summary)
	}
}

func TestResultCollector_StdoutFailed_VolumDiscarded(t *testing.T) {
	dir := t.TempDir()
	volResult := AgentResult{
		Version:  1,
		Status:   AgentStatusSucceeded,
		ExitCode: 0,
		Summary:  "volume says success",
	}
	path := writeResultFile(t, dir, volResult)

	stdout := validStdoutJSON(AgentStatusFailed, "stdout says failed")

	c := NewResultCollector()
	got, err := c.CollectFromExec(stdout, path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// stdout failed -> discard volume.
	if got.Status != AgentStatusFailed {
		t.Errorf("status = %q, want failed", got.Status)
	}
	if got.Summary != "stdout says failed" {
		t.Errorf("summary = %q", got.Summary)
	}
}

func TestResultCollector_Neither(t *testing.T) {
	c := NewResultCollector()
	_, err := c.CollectFromExec("no json here", "/nonexistent/path")
	if err == nil {
		t.Fatal("expected error when neither source available")
	}
	if !strings.Contains(err.Error(), "no results") {
		t.Errorf("error = %q", err)
	}
}

func TestResultCollector_MalformedStdout_VolumeExists(t *testing.T) {
	dir := t.TempDir()
	volResult := AgentResult{
		Version:  1,
		Status:   AgentStatusSucceeded,
		ExitCode: 0,
		Summary:  "volume result",
	}
	path := writeResultFile(t, dir, volResult)

	c := NewResultCollector()
	got, err := c.CollectFromExec("garbage output", path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Falls back to volume when stdout unparseable.
	if got.Summary != "volume result" {
		t.Errorf("summary = %q, want 'volume result'", got.Summary)
	}
}

func TestResultCollector_MalformedVolume(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "results.json")
	if err := os.WriteFile(path, []byte("not-json{"), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout := validStdoutJSON(AgentStatusSucceeded, "stdout ok")

	c := NewResultCollector()
	// stdout succeeded but volume is malformed -> should still return
	// stdout result since volume parse fails.
	got, err := c.CollectFromExec(stdout, path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Summary != "stdout ok" {
		t.Errorf("summary = %q", got.Summary)
	}
}

func TestResultCollector_OversizedVolume(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "results.json")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	// Sparse file > 50MB.
	if _, err := f.Seek(51*1024*1024, 0); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if _, err := f.Write([]byte{0}); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()

	stdout := validStdoutJSON(AgentStatusSucceeded, "stdout ok")

	c := NewResultCollector()
	// Volume too large -> falls back to stdout.
	got, err := c.CollectFromExec(stdout, path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Summary != "stdout ok" {
		t.Errorf("summary = %q", got.Summary)
	}
}

func TestResultCollector_OversizedVolume_NoStdout(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "results.json")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Seek(51*1024*1024, 0); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if _, err := f.Write([]byte{0}); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()

	c := NewResultCollector()
	_, err = c.CollectFromExec("no json", path)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "no results") {
		t.Errorf("error = %q", err)
	}
}

func TestResultCollector_PartialStatus(t *testing.T) {
	stdout := validStdoutJSON(AgentStatusPartial, "2 of 3 done")

	c := NewResultCollector()
	got, err := c.CollectFromExec(stdout, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Status != AgentStatusPartial {
		t.Errorf("status = %q, want partial", got.Status)
	}
}

func TestResultCollector_TimeoutStatus(t *testing.T) {
	stdout := validStdoutJSON(AgentStatusTimeout, "timed out")

	c := NewResultCollector()
	got, err := c.CollectFromExec(stdout, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Status != AgentStatusTimeout {
		t.Errorf("status = %q, want timeout", got.Status)
	}
}

func TestResultCollector_StdoutOnly_InvalidVersion(t *testing.T) {
	r := AgentResult{Version: 99, Status: AgentStatusSucceeded, ExitCode: 0, Summary: "done"}
	data, _ := json.Marshal(r)

	c := NewResultCollector()
	_, err := c.CollectFromExec(string(data), "")
	if err == nil {
		t.Fatal("expected error for invalid version")
	}
	if !strings.Contains(err.Error(), "unsupported") {
		t.Errorf("error = %q", err)
	}
}

func TestResultCollector_VolumeOnly(t *testing.T) {
	dir := t.TempDir()
	volResult := AgentResult{
		Version:  1,
		Status:   AgentStatusSucceeded,
		ExitCode: 0,
		Summary:  "volume only",
	}
	path := writeResultFile(t, dir, volResult)

	c := NewResultCollector()
	got, err := c.CollectFromExec("no json output here", path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Summary != "volume only" {
		t.Errorf("summary = %q", got.Summary)
	}
}

func TestResultCollector_ConflictingStatus(t *testing.T) {
	dir := t.TempDir()
	// Volume says succeeded.
	volResult := AgentResult{
		Version: 1, Status: AgentStatusSucceeded,
		ExitCode: 0, Summary: "volume succeeded",
	}
	path := writeResultFile(t, dir, volResult)

	// stdout says failed -> should use stdout (failed wins).
	stdout := validStdoutJSON(AgentStatusFailed, "stdout failed")

	c := NewResultCollector()
	got, err := c.CollectFromExec(stdout, path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Status != AgentStatusFailed {
		t.Errorf("status = %q, want failed (stdout overrides)", got.Status)
	}
}

func TestResultCollector_StdoutWithLogLines(t *testing.T) {
	stdout := "Starting agent...\nProcessing task T-0042...\n" +
		`{"version":1,"status":"succeeded","exit_code":0,"summary":"task done"}` +
		"\nCleanup complete."

	c := NewResultCollector()
	got, err := c.CollectFromExec(stdout, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Summary != "task done" {
		t.Errorf("summary = %q", got.Summary)
	}
}
