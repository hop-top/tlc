package cli

// task.todo_file is an ABSOLUTE path on the happy path: root.go seeds the
// default as filepath.Join(UserDataDir(), "todo.txt"), and users who point
// the key at a shared store write an absolute one by hand.
//
// The local projection writer joined it onto the project's config
// directory unconditionally. filepath.Join("/p/.tlc", "/abs/todo.txt")
// does not yield "/abs/todo.txt" — Join cleans the leading separator away
// and NESTS, giving "/p/.tlc/abs/todo.txt". So every task write mirrored
// the absolute path's whole directory chain inside the config directory,
// and the file the user named was never written by the local path at all.
//
// These run the real binary: the defect is a directory appearing on disk,
// which is a claim about what the process did, not about what a function
// returned.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// absTodoFixture builds a project whose config sets task.todo_file to
// todoFile verbatim, and returns the binary, the project dir and the env.
func absTodoFixture(t *testing.T, todoFile string) (bin, proj string, env []string) {
	t.Helper()

	bin = buildTLCBinary(t)
	home := t.TempDir()
	dbPath := filepath.Join(home, "tasks.db")

	proj = filepath.Join(home, "proj")
	tlcDir := filepath.Join(proj, ".tlc")
	if err := os.MkdirAll(tlcDir, 0o750); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}

	cfg := "project:\n  id: absproj\nstorage:\n  db_path: " + dbPath +
		"\ntask:\n  todo_file: " + todoFile + "\n"
	if err := os.WriteFile(filepath.Join(tlcDir, "config.yaml"), []byte(cfg), 0o600); err != nil {
		t.Fatalf("write project config: %v", err)
	}

	// No TLC_CONFIG: the project's own .tlc/config.yaml must be what is
	// discovered, since that discovery is what puts DetectProject in
	// project context in the first place.
	return bin, proj, e2eEnv(t, home, dbPath)
}

// strayEntries returns the names directly under root that tlc does not
// legitimately own there. An absolute path mirrored into the config
// directory shows up as exactly one such name: its leading component.
func strayEntries(t *testing.T, root string, allowed map[string]bool) []string {
	t.Helper()

	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read %s: %v", root, err)
	}
	var stray []string
	for _, e := range entries {
		if allowed[e.Name()] {
			continue
		}
		stray = append(stray, e.Name())
	}
	return stray
}

// TestAbsoluteTodoFileNotNestedUnderConfigDir is the headline regression.
// An absolute task.todo_file must be written at the path it names, and
// must not mirror its directory chain inside the project config dir.
func TestAbsoluteTodoFileNotNestedUnderConfigDir(t *testing.T) {
	home := t.TempDir()
	// A deliberately deep absolute path: the defect reproduces the WHOLE
	// chain under the config dir, so the first stray component is the
	// one furthest from the leaf and is what the assertion catches.
	todoFile := filepath.Join(home, "shared", "store", "tlc", "todo.txt")
	if err := os.MkdirAll(filepath.Dir(todoFile), 0o750); err != nil {
		t.Fatalf("mkdir todo dir: %v", err)
	}

	bin, proj, env := absTodoFixture(t, todoFile)
	runTLCOK(t, bin, proj, env, "task", "create", "abs path probe")

	tlcDir := filepath.Join(proj, ".tlc")

	// The absolute path the user named must hold the projection.
	data, err := os.ReadFile(todoFile)
	if err != nil {
		t.Fatalf("absolute todo_file was not written at %s: %v", todoFile, err)
	}
	if !strings.Contains(string(data), "abs path probe") {
		t.Errorf("absolute todo_file at %s lacks the task:\n%s", todoFile, data)
	}

	// Nothing path-shaped may appear under the config dir. tlc owns
	// config.yaml and its projection directories there; an absolute
	// path's leading component is none of those.
	allowed := map[string]bool{
		"config.yaml": true,
		"tasks":       true,
		"tracks":      true,
		"recipes":     true,
		"flows":       true,
		"db.sqlite":   true,
	}
	if stray := strayEntries(t, tlcDir, allowed); len(stray) > 0 {
		t.Errorf("absolute todo_file mirrored its path under %s: stray entries %v", tlcDir, stray)
	}

	// Stated the other way, so the assertion names the actual defect:
	// the joined path must not exist.
	nested := filepath.Join(tlcDir, strings.TrimPrefix(todoFile, string(os.PathSeparator)))
	if _, err := os.Stat(nested); err == nil {
		t.Errorf("nested mirror of the absolute todo_file exists at %s", nested)
	}
}

// TestRelativeTodoFileStillResolvesAgainstConfigDir guards the obvious
// wrong fix: treating every value as absolute. A relative task.todo_file
// is the documented project-local form and must keep landing next to the
// config that named it.
func TestRelativeTodoFileStillResolvesAgainstConfigDir(t *testing.T) {
	bin, proj, env := absTodoFixture(t, "todo.txt")
	runTLCOK(t, bin, proj, env, "task", "create", "relative path probe")

	local := filepath.Join(proj, ".tlc", "todo.txt")
	data, err := os.ReadFile(local)
	if err != nil {
		t.Fatalf("relative todo_file was not written at %s: %v", local, err)
	}
	if !strings.Contains(string(data), "relative path probe") {
		t.Errorf("relative todo_file at %s lacks the task:\n%s", local, data)
	}
}

// TestRelativeSubdirTodoFileResolvesAgainstConfigDir pins the
// subdirectory form, which is the case a naive filepath.IsAbs check
// could not have broken but a "use the base name" fix would.
func TestRelativeSubdirTodoFileResolvesAgainstConfigDir(t *testing.T) {
	bin, proj, env := absTodoFixture(t, filepath.Join("notes", "todo.md"))
	runTLCOK(t, bin, proj, env, "task", "create", "subdir path probe")

	local := filepath.Join(proj, ".tlc", "notes", "todo.md")
	data, err := os.ReadFile(local)
	if err != nil {
		t.Fatalf("relative subdir todo_file was not written at %s: %v", local, err)
	}
	if !strings.Contains(string(data), "subdir path probe") {
		t.Errorf("relative subdir todo_file at %s lacks the task:\n%s", local, data)
	}
}
