package config

// A config is PRESENT only when its file exists.
//
// localConfigPath answered "present" for a bare .tlc/ or .hop/tlc/
// DIRECTORY holding no config.yaml, and then reported the non-existent
// config.yaml inside it as the path it had found. Both halves are wrong
// together: CheckConfigConflict read an unreadable file, got the empty
// identity back, compared it against the real standalone config's
// identity, found them unequal, and refused every command with an
// ambiguous-config error naming a file that does not exist.
//
// A bare .hop/tlc/ is not hypothetical. Anything that mkdir -p's a path
// under the config directory creates one — which is exactly what the
// local projection writer did when it joined an absolute task.todo_file
// onto the config dir.

import (
	"os"
	"path/filepath"
	"testing"
)

// writeConfigAt writes a minimal config declaring id and dbPath at rel
// under root, creating parents.
func writeConfigAt(t *testing.T, root, rel, id, dbPath string) {
	t.Helper()

	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		t.Fatalf("mkdir for %s: %v", rel, err)
	}
	body := "project:\n  id: " + id + "\nstorage:\n  db_path: " + dbPath + "\n"
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

// TestBareHopConfigDirIsNotAConflict is the headline regression: a
// .hop/tlc/ directory with no config.yaml in it must not make a
// standalone project ambiguous.
func TestBareHopConfigDirIsNotAConflict(t *testing.T) {
	root := t.TempDir()
	writeConfigAt(t, root, filepath.Join(".tlc", "config.yaml"), "hop-top/tlc", "/data/db.sqlite")

	// A directory only. No .hop/tlc/config.yaml, no .hop/tlc.yaml.
	if err := os.MkdirAll(filepath.Join(root, ".hop", "tlc"), 0o750); err != nil {
		t.Fatalf("mkdir .hop/tlc: %v", err)
	}

	if err := CheckConfigConflict(root); err != nil {
		t.Errorf("bare .hop/tlc/ directory reported a conflict: %v", err)
	}
	if BothLayoutsPresent(root) {
		t.Error("BothLayoutsPresent() = true with only a bare .hop/tlc/ directory")
	}
}

// TestBareStandaloneConfigDirIsNotAConflict is the mirror case: the
// predicate is per layout, so an empty .tlc/ must be just as absent as
// an empty .hop/tlc/.
func TestBareStandaloneConfigDirIsNotAConflict(t *testing.T) {
	root := t.TempDir()
	writeConfigAt(t, root, filepath.Join(".hop", "tlc", "config.yaml"), "hop-top/tlc", "/data/db.sqlite")

	if err := os.MkdirAll(filepath.Join(root, ".tlc", "tasks"), 0o750); err != nil {
		t.Fatalf("mkdir .tlc/tasks: %v", err)
	}

	if err := CheckConfigConflict(root); err != nil {
		t.Errorf("bare .tlc/ directory reported a conflict: %v", err)
	}
	if BothLayoutsPresent(root) {
		t.Error("BothLayoutsPresent() = true with only a bare .tlc/ directory")
	}
}

// TestRealDisagreeingConfigsStillConflict is the behavior the fix must
// preserve. Two configs that both exist and name different projects are
// the case the ambiguity error was written for.
func TestRealDisagreeingConfigsStillConflict(t *testing.T) {
	root := t.TempDir()
	writeConfigAt(t, root, filepath.Join(".tlc", "config.yaml"), "hop-top/tlc", "/data/standalone.sqlite")
	writeConfigAt(t, root, filepath.Join(".hop", "tlc", "config.yaml"), "hop-top/other", "/data/hop.sqlite")

	err := CheckConfigConflict(root)
	if err == nil {
		t.Fatal("two disagreeing configs did not conflict")
	}
	var conflict *ConfigConflictError
	if !asConflict(err, &conflict) {
		t.Fatalf("error is not a *ConfigConflictError: %v", err)
	}
	if conflict.Standalone.ProjectID != "hop-top/tlc" || conflict.Hop.ProjectID != "hop-top/other" {
		t.Errorf("conflict identities wrong: %+v", conflict)
	}
	if !BothLayoutsPresent(root) {
		t.Error("BothLayoutsPresent() = false with two real configs")
	}
}

// TestFlatHopConfigStillConflicts pins the .hop/tlc.yaml form: the flat
// file is a legitimate hop config and a fix that only looked for
// .hop/tlc/config.yaml would stop seeing it.
func TestFlatHopConfigStillConflicts(t *testing.T) {
	root := t.TempDir()
	writeConfigAt(t, root, filepath.Join(".tlc", "config.yaml"), "hop-top/tlc", "/data/standalone.sqlite")
	writeConfigAt(t, root, filepath.Join(".hop", "tlc.yaml"), "hop-top/other", "/data/hop.sqlite")

	if err := CheckConfigConflict(root); err == nil {
		t.Fatal("flat .hop/tlc.yaml disagreeing with standalone did not conflict")
	}
	if !BothLayoutsPresent(root) {
		t.Error("BothLayoutsPresent() = false with a flat hop config")
	}
}

// TestBareHopConfigDirDoesNotShadowFlatFile covers the combination the
// stray directory creates in the wild: a .hop/tlc/ directory with no
// config.yaml SITTING BESIDE a real flat .hop/tlc.yaml. The directory
// form must not shadow the flat file into invisibility.
func TestBareHopConfigDirDoesNotShadowFlatFile(t *testing.T) {
	root := t.TempDir()
	writeConfigAt(t, root, filepath.Join(".tlc", "config.yaml"), "hop-top/tlc", "/data/standalone.sqlite")
	writeConfigAt(t, root, filepath.Join(".hop", "tlc.yaml"), "hop-top/other", "/data/hop.sqlite")
	if err := os.MkdirAll(filepath.Join(root, ".hop", "tlc"), 0o750); err != nil {
		t.Fatalf("mkdir .hop/tlc: %v", err)
	}

	if err := CheckConfigConflict(root); err == nil {
		t.Fatal("a bare .hop/tlc/ directory hid the real flat .hop/tlc.yaml")
	}
}

// TestAgreeingConfigsDoNotConflict is the documented mirror case: hubs
// that write one config into both layouts agree and must stay quiet.
func TestAgreeingConfigsDoNotConflict(t *testing.T) {
	root := t.TempDir()
	writeConfigAt(t, root, filepath.Join(".tlc", "config.yaml"), "hop-top/tlc", "/data/db.sqlite")
	writeConfigAt(t, root, filepath.Join(".hop", "tlc", "config.yaml"), "hop-top/tlc", "/data/db.sqlite")

	if err := CheckConfigConflict(root); err != nil {
		t.Errorf("agreeing configs conflicted: %v", err)
	}
}

// asConflict is errors.As specialized, kept local so the test file
// states its own dependency.
func asConflict(err error, target **ConfigConflictError) bool {
	c, ok := err.(*ConfigConflictError) //nolint:errorlint // CheckConfigConflict returns it unwrapped
	if ok {
		*target = c
	}
	return ok
}
