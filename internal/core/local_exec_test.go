package core

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLocalExecManager_Exec_ProtocolEnv: every local agent learns its
// context and results paths from TLC_CONTEXT_PATH / TLC_RESULTS_PATH.
func TestLocalExecManager_Exec_ProtocolEnv(t *testing.T) {
	dir := t.TempDir()
	script := []string{"-c", "printf '%s\\n%s\\n' \"$TLC_CONTEXT_PATH\" \"$TLC_RESULTS_PATH\""}

	t.Run("explicit paths", func(t *testing.T) {
		results := filepath.Join(dir, "run-1", "results.json")
		m := NewLocalExecManager(nil, results)
		stdout, _, code, err := m.Exec(context.Background(), LocalExecOpts{
			Binary:      "sh",
			Args:        script,
			RepoRoot:    dir,
			ContextPath: filepath.Join(dir, "run-1", "context.json"),
		})
		if err != nil || code != 0 {
			t.Fatalf("exec: code=%d err=%v", code, err)
		}
		want := filepath.Join(dir, "run-1", "context.json") + "\n" + results + "\n"
		if stdout != want {
			t.Errorf("stdout = %q; want %q", stdout, want)
		}
	})

	t.Run("defaults", func(t *testing.T) {
		m := NewLocalExecManager(nil, "")
		stdout, _, code, err := m.Exec(context.Background(), LocalExecOpts{
			Binary:   "sh",
			Args:     script,
			EnvVars:  map[string]string{"UNRELATED": "1"},
			RepoRoot: dir,
		})
		if err != nil || code != 0 {
			t.Fatalf("exec: code=%d err=%v", code, err)
		}
		want := "\n" + filepath.Join(dir, ".tlc", "results.json") + "\n"
		if stdout != want {
			t.Errorf("stdout = %q; want no context path and the default results path %q", stdout, want)
		}
	})
}

func TestLocalExecManager_Exec(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		m := NewLocalExecManager(nil, "")
		stdout, _, code, err := m.Exec(context.Background(), LocalExecOpts{
			Binary: "echo",
			Args:   []string{"hello"},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if code != 0 {
			t.Errorf("exit code = %d", code)
		}
		if stdout == "" {
			t.Error("expected stdout")
		}
	})

	t.Run("env and dir", func(t *testing.T) {
		dir := t.TempDir()
		m := NewLocalExecManager(nil, "")
		stdout, _, code, err := m.Exec(context.Background(), LocalExecOpts{
			Binary:   "sh",
			Args:     []string{"-c", "echo $MY_VAR && pwd"},
			EnvVars:  map[string]string{"MY_VAR": "test123"},
			RepoRoot: dir,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if code != 0 {
			t.Errorf("exit code = %d", code)
		}
		if !strings.Contains(stdout, "test123") {
			t.Errorf("env var not passed: stdout = %q", stdout)
		}
	})

	// T-0594: verify Exec passes EnvVars and RepoRoot to the subprocess.
	// The subprocess must see the injected env var and run inside the
	// specified directory (RepoRoot -> cmd.Dir, EnvVars -> cmd.Env).
	t.Run("env vars and repo root", func(t *testing.T) {
		dir := t.TempDir()
		m := NewLocalExecManager(nil, "")
		stdout, _, code, err := m.Exec(context.Background(), LocalExecOpts{
			Binary:   "sh",
			Args:     []string{"-c", "echo $TLC_TEST_VAR; pwd"},
			EnvVars:  map[string]string{"TLC_TEST_VAR": "regression-594"},
			RepoRoot: dir,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if code != 0 {
			t.Errorf("exit code = %d", code)
		}
		// Verify env var was passed.
		if !strings.Contains(stdout, "regression-594") {
			t.Errorf("env var not passed; stdout = %q", stdout)
		}
		// Verify working directory was set to RepoRoot.
		// filepath.EvalSymlinks normalises /private/tmp -> /tmp on macOS.
		resolvedDir, _ := filepath.EvalSymlinks(dir)
		if !strings.Contains(stdout, resolvedDir) {
			t.Errorf(
				"repo root not set as working dir; stdout = %q, want %q",
				stdout, resolvedDir,
			)
		}
	})

	// T-0594 (cont): multiple env vars are passed correctly.
	t.Run("multiple env vars", func(t *testing.T) {
		m := NewLocalExecManager(nil, "")
		stdout, _, code, err := m.Exec(context.Background(), LocalExecOpts{
			Binary: "sh",
			Args:   []string{"-c", "echo $VAR_A:$VAR_B"},
			EnvVars: map[string]string{
				"VAR_A": "alpha",
				"VAR_B": "bravo",
			},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if code != 0 {
			t.Errorf("exit code = %d", code)
		}
		if !strings.Contains(stdout, "alpha:bravo") {
			t.Errorf("multiple env vars not passed; stdout = %q", stdout)
		}
	})

	t.Run("missing binary", func(t *testing.T) {
		m := NewLocalExecManager(nil, "")
		_, _, _, err := m.Exec(context.Background(), LocalExecOpts{})
		if err == nil {
			t.Fatal("expected error for missing binary")
		}
	})
}

func TestLocalExecManager_ResultsFilePath(t *testing.T) {
	t.Run("default relative", func(t *testing.T) {
		m := NewLocalExecManager(nil, "")
		got := m.ResultsFilePath("/workspace")
		want := filepath.Join("/workspace", ".tlc", "results.json")
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("custom relative", func(t *testing.T) {
		m := NewLocalExecManager(nil, "output/results.json")
		got := m.ResultsFilePath("/workspace")
		want := filepath.Join("/workspace", "output", "results.json")
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("absolute path", func(t *testing.T) {
		m := NewLocalExecManager(nil, "/tmp/results.json")
		got := m.ResultsFilePath("/workspace")
		if got != "/tmp/results.json" {
			t.Errorf("got %q, want /tmp/results.json", got)
		}
	})
}

func TestLocalExecManager_ReadResult(t *testing.T) {
	t.Run("file exists and valid", func(t *testing.T) {
		dir := t.TempDir()
		tlcDir := filepath.Join(dir, ".tlc")
		if err := os.MkdirAll(tlcDir, 0o755); err != nil {
			t.Fatal(err)
		}
		result := AgentResult{
			Version:  1,
			Status:   AgentStatusSucceeded,
			ExitCode: 0,
			Summary:  "all good",
			Agent:    "claude",
		}
		data, _ := json.Marshal(result)
		if err := os.WriteFile(
			filepath.Join(tlcDir, "results.json"), data, 0o644,
		); err != nil {
			t.Fatal(err)
		}

		m := NewLocalExecManager(nil, "")
		got, err := m.ReadResult(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got == nil {
			t.Fatal("expected non-nil result")
		}
		if got.Status != AgentStatusSucceeded {
			t.Errorf("status = %q", got.Status)
		}
		if got.Agent != "claude" {
			t.Errorf("agent = %q", got.Agent)
		}
	})

	t.Run("file missing", func(t *testing.T) {
		dir := t.TempDir()
		m := NewLocalExecManager(nil, "")
		got, err := m.ReadResult(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != nil {
			t.Errorf("expected nil for missing file, got %+v", got)
		}
	})

	t.Run("malformed JSON", func(t *testing.T) {
		dir := t.TempDir()
		tlcDir := filepath.Join(dir, ".tlc")
		if err := os.MkdirAll(tlcDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(
			filepath.Join(tlcDir, "results.json"),
			[]byte("not-json{"), 0o644,
		); err != nil {
			t.Fatal(err)
		}

		m := NewLocalExecManager(nil, "")
		_, err := m.ReadResult(dir)
		if err == nil {
			t.Fatal("expected error for malformed JSON")
		}
	})

	t.Run("oversized file", func(t *testing.T) {
		dir := t.TempDir()
		tlcDir := filepath.Join(dir, ".tlc")
		if err := os.MkdirAll(tlcDir, 0o755); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(tlcDir, "results.json")
		// Create a file that reports as >50MB via sparse file.
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		// Seek to 51MB and write a byte to make stat report the size.
		if _, err := f.Seek(51*1024*1024, 0); err != nil {
			f.Close()
			t.Fatal(err)
		}
		if _, err := f.Write([]byte{0}); err != nil {
			f.Close()
			t.Fatal(err)
		}
		f.Close()

		m := NewLocalExecManager(nil, "")
		_, err = m.ReadResult(dir)
		if err == nil {
			t.Fatal("expected error for oversized file")
		}
	})
}

func TestParseStdoutResult(t *testing.T) {
	t.Run("valid last line", func(t *testing.T) {
		stdout := "some log output\nmore logs\n" +
			`{"version":1,"status":"succeeded","exit_code":0,"summary":"done"}`
		got, err := ParseStdoutResult(stdout)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Status != AgentStatusSucceeded {
			t.Errorf("status = %q", got.Status)
		}
	})

	t.Run("JSON not last line", func(t *testing.T) {
		stdout := `{"version":1,"status":"failed","exit_code":1,"summary":"oops"}` +
			"\nfinal log line"
		got, err := ParseStdoutResult(stdout)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Status != AgentStatusFailed {
			t.Errorf("status = %q, want failed", got.Status)
		}
	})

	t.Run("no JSON", func(t *testing.T) {
		_, err := ParseStdoutResult("just plain text\nno json here")
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("empty stdout", func(t *testing.T) {
		_, err := ParseStdoutResult("")
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("malformed JSON", func(t *testing.T) {
		_, err := ParseStdoutResult(`{bad json}`)
		if err == nil {
			t.Fatal("expected error")
		}
	})
}
