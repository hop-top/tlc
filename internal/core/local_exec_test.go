package core

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalExecManager_Exec(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		r := &mockRunner{results: []mockResult{
			{Stdout: `{"version":1,"status":"succeeded","exit_code":0,"summary":"done"}`, ExitCode: 0},
		}}
		m := NewLocalExecManager(r, "")
		stdout, _, code, err := m.Exec(context.Background(), LocalExecOpts{
			Binary: "claude",
			Args:   []string{"--context", "/tmp/ctx.json"},
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

	t.Run("missing binary", func(t *testing.T) {
		r := &mockRunner{}
		m := NewLocalExecManager(r, "")
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
