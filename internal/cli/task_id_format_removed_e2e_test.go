package cli

// End-to-end coverage for the REMOVAL of task.id_format.
//
// task.id_format was declared, defaulted, advertised in the interactive
// config hints, and documented — but never read. Task identity is a
// TypeID assigned at creation; the familiar "T-0042" is a display alias
// rendered by core.FormatTaskSeq from the per-project sequence number
// and parsed back by several independent readers with a hardcoded
// grammar. The key was therefore removed rather than implemented.
//
// Two properties need guarding against regression:
//
//  1. Stores in the wild still carry the key. Decoding must ignore it,
//     not fail — removing a field from a struct is only safe while the
//     config decoder is non-strict, and a future switch to strict
//     decoding would turn every such config into a hard error.
//  2. Alias rendering stays fixed at "T-" plus at-least-4-digit
//     zero-padding, since the cross-project ref grammar, the todo.txt
//     round trip and URI normalisation all parse that shape back.
//
// The same two hazards that shape config_layer_precedence_e2e_test.go
// apply: storage.db_path is pinned in every config below (the DB
// resolves through the global project registry, not the cwd), and each
// case writes a fresh config file because running tlc rewrites it in
// place.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeIDFormatConfig materializes a project config at
// <dir>/.tlc/config.yaml with the DB pinned, optionally carrying a
// leftover task.id_format key.
func writeIDFormatConfig(t *testing.T, dir, dbPath, idFormat string) {
	t.Helper()

	body := `storage:
  db_path: ` + dbPath + `
task:
  default_status: TODO
  require_reference: false
`
	if idFormat != "" {
		body += `  id_format: "` + idFormat + `"` + "\n"
	}

	path := filepath.Join(dir, ".tlc", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// TestRemovedIDFormatKeyIsIgnored proves a config that still sets
// task.id_format is accepted, and that the key has no effect: the alias
// of the first task is "T-0001" whether the key is absent, set to the
// old default, or set to something that would be unmistakable if it
// were ever honored.
func TestRemovedIDFormatKeyIsIgnored(t *testing.T) {
	bin := buildTLCBinary(t)

	cases := []struct {
		name     string
		idFormat string
	}{
		{"absent", ""},
		{"old default", "T-{seq:04d}"},
		{"custom template", "TASK-{seq:08d}"},
		{"unparseable template", "T-{seq:!!bogus"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			proj := t.TempDir()
			dbPath := filepath.Join(t.TempDir(), "db.sqlite")
			writeIDFormatConfig(t, proj, dbPath, tc.idFormat)
			env := e2eEnv(t, home, dbPath)

			// Config must load without error even with the removed key
			// present, including an unparseable template value: nothing
			// parses it, so nothing can reject it.
			out, code := runTLC(t, bin, proj, env, "task", "create", "id format probe")
			if code != 0 {
				t.Fatalf("task create exit = %d, want 0\n%s", code, out)
			}

			// Default alias generation is unchanged in every case.
			if !strings.Contains(out, "T-0001") {
				t.Errorf("task create output missing alias T-0001 "+
					"(id_format=%q):\n%s", tc.idFormat, out)
			}

			// The alias resolves back — references still work.
			show := runTLCOK(t, bin, proj, env, "task", "show", "T-0001")
			if !strings.Contains(show, "id format probe") {
				t.Errorf("task show T-0001 missing title "+
					"(id_format=%q):\n%s", tc.idFormat, show)
			}
		})
	}
}

// TestRemovedIDFormatNotAdvertisedInHints proves the interactive config
// flow no longer offers id_format. The hint table is the surface that
// invited users to set the key in the first place, so it is the surface
// that must stop naming it. A live sibling key anchors the assertion so
// an empty or restructured table cannot make this pass vacuously.
func TestRemovedIDFormatNotAdvertisedInHints(t *testing.T) {
	hints := defaultKeyHints()

	if _, ok := hints["task.default_status"]; !ok {
		t.Fatal("hint table missing task.default_status; " +
			"anchor key absent, assertion would be vacuous")
	}
	if h, ok := hints["task.id_format"]; ok {
		t.Errorf("hint table still advertises task.id_format: %+v", h)
	}

	// Nothing in any hint's text should mention the key either — the
	// description used to read "Task ID format template (e.g.
	// T-{seq:04d})".
	for key, h := range hints {
		if strings.Contains(h.Description, "id_format") ||
			strings.Contains(h.Description, "{seq:") {
			t.Errorf("hint %q description still references the removed "+
				"id_format template: %q", key, h.Description)
		}
	}
}
