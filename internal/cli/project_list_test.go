package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func TestProjectListEmpty(t *testing.T) {
	withTestLock(func() {
		resetTestDB(t)

		cmd := newTestCmd()
		cmd.AddCommand(ProjectCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"project", "list"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("project list (empty) failed: %v", err)
		}

		out := buf.String()
		if !contains(out, "No projects") {
			t.Errorf("expected empty-state message; got: %q", out)
		}
	})
}

func TestProjectListTable(t *testing.T) {
	withTestLock(func() {
		resetTestDB(t)
		ctx := context.Background()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		if err := s.RegisterProject(ctx, "proj/alpha", "/data/alpha.sqlite", "space:main", "Alpha"); err != nil {
			t.Fatalf("RegisterProject: %v", err)
		}
		if err := s.RegisterProject(ctx, "proj/beta", "/data/beta.sqlite", "space:dev", "Beta"); err != nil {
			t.Fatalf("RegisterProject: %v", err)
		}

		viper.Set("output.format", "table")
		cmd := newTestCmd()
		cmd.AddCommand(ProjectCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"project", "list"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("project list (table) failed: %v", err)
		}

		out := buf.String()
		for _, want := range []string{"proj/alpha", "Alpha", "/data/alpha.sqlite", "proj/beta", "Beta"} {
			if !contains(out, want) {
				t.Errorf("expected %q in output; got: %q", want, out)
			}
		}
	})
}

func TestProjectListJSON(t *testing.T) {
	withTestLock(func() {
		resetTestDB(t)
		ctx := context.Background()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		if err := s.RegisterProject(ctx, "proj/gamma", "/data/gamma.sqlite", "space:prod", "Gamma"); err != nil {
			t.Fatalf("RegisterProject: %v", err)
		}

		cmd := newTestCmd()
		cmd.AddCommand(ProjectCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"project", "list", "--format", "json"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("project list (json) failed: %v", err)
		}

		out := buf.String()
		for _, want := range []string{"proj/gamma", "Gamma", "/data/gamma.sqlite", "space:prod"} {
			if !contains(out, want) {
				t.Errorf("expected %q in JSON output; got: %q", want, out)
			}
		}
		if !strings.HasPrefix(strings.TrimSpace(out), "[") {
			t.Errorf("expected JSON array output; got: %q", out)
		}
	})
}

func TestProjectListYAML(t *testing.T) {
	withTestLock(func() {
		resetTestDB(t)
		ctx := context.Background()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		if err := s.RegisterProject(ctx, "proj/delta", "/data/delta.sqlite", "space:ci", "Delta"); err != nil {
			t.Fatalf("RegisterProject: %v", err)
		}

		cmd := newTestCmd()
		cmd.AddCommand(ProjectCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"project", "list", "--format", "yaml"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("project list (yaml) failed: %v", err)
		}

		out := buf.String()
		for _, want := range []string{"proj/delta", "Delta", "/data/delta.sqlite", "space:ci"} {
			if !contains(out, want) {
				t.Errorf("expected %q in YAML output; got: %q", want, out)
			}
		}
	})
}

func TestProjectListAlias(t *testing.T) {
	withTestLock(func() {
		resetTestDB(t)

		cmd := newTestCmd()
		cmd.AddCommand(ProjectCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"project", "ls"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("project ls (alias) failed: %v", err)
		}
	})
}
