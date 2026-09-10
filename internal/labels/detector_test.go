package labels

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectProjectType(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "tlc-labels-test-*")
	defer os.RemoveAll(tmpDir)

	tests := []struct {
		name     string
		files    []string
		expected ProjectType
	}{
		{
			name:     "Go project",
			files:    []string{"go.mod", "main.go"},
			expected: TypeGoBinary,
		},
		{
			// The cmd/ branch used to return TypeGoBinary and then fall
			// through to returning TypeGoBinary again, on the way to a
			// go-socket split that never arrived. Both spellings must
			// keep answering the same, or removing the dead branch
			// changed behaviour.
			name:     "Go project with cmd",
			files:    []string{"go.mod", "cmd/tool/main.go"},
			expected: TypeGoBinary,
		},
		{
			name:     "Django project",
			files:    []string{"manage.py"},
			expected: TypePythonMVC,
		},
		{
			name:     "React project",
			files:    []string{"package.json", "src/App.tsx"},
			expected: TypeReactFrontend,
		},
		{
			name:     "Python project",
			files:    []string{"requirements.txt", "app.py"},
			expected: TypePythonMVC,
		},
		{
			name:     "Unknown project",
			files:    []string{"README.md"},
			expected: TypeGeneric,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			projDir := filepath.Join(tmpDir, tt.name)
			os.MkdirAll(projDir, 0o755)
			for _, f := range tt.files {
				fPath := filepath.Join(projDir, f)
				os.MkdirAll(filepath.Dir(fPath), 0o755)
				os.WriteFile(fPath, []byte(""), 0o644)
			}

			got := DetectProjectType(projDir)
			if got != tt.expected {
				t.Errorf("DetectProjectType() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// TestDetectionReachesEveryAdvertisedType states the rule the phantom
// types broke from the detection side.
//
// TypeGoSocket and TypeMicroservices were declared constants that no
// detector branch could return and no --type case handled. Requiring
// every advertised type to be reachable by detection keeps the two
// halves honest: a type worth listing is a type worth guessing.
//
// This is a deliberate policy choice rather than an accident of the
// current shapes. A type reachable ONLY through an explicit --type is
// defensible — it just has to be argued for, and the test is where that
// argument would have to be written down.
func TestDetectionReachesEveryAdvertisedType(t *testing.T) {
	fixtures := map[ProjectType][]string{
		TypeGoBinary:      {"go.mod"},
		TypeReactFrontend: {"package.json", "src/App.tsx"},
		TypePythonMVC:     {"manage.py"},
		TypeGeneric:       {"README.md"},
	}

	for _, pt := range AllProjectTypes() {
		files, ok := fixtures[pt]
		if !ok {
			t.Errorf("%s is advertised but no fixture shows detection reaching it", pt)
			continue
		}
		dir := t.TempDir()
		for _, f := range files {
			p := filepath.Join(dir, f)
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, nil, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		if got := DetectProjectType(dir); got != pt {
			t.Errorf("DetectProjectType(%v) = %v, want %v", files, got, pt)
		}
	}
}

// TestRemovedTypesHaveNoReferences proves the deleted constants left no
// live code behind.
//
// It checks the IDENTIFIERS, not the wire strings. Deleting a constant
// does fail the build at its use sites, so this test is not the primary
// guard — its job is to stay failing if someone reintroduces
// TypeGoSocket or TypeMicroservices as a declared-but-unhandled
// constant, which is the exact shape of the original defect and which
// the compiler is perfectly happy with.
//
// The hyphenated forms are deliberately NOT checked: "go-socket" and
// "microservices" legitimately appear in the comments explaining the
// removal, in the negative assertions in the CLI e2e test, and — for
// "microservices" — as ordinary task-description prose in
// prompt_e2e_test.go. Grepping those would only teach the next reader
// to add exclusions.
func TestRemovedTypesHaveNoReferences(t *testing.T) {
	for _, path := range goSourceFiles(t, filepath.Join("..", "..")) {
		// This file names the removed identifiers on purpose.
		if filepath.Base(path) == "detector_test.go" {
			continue
		}
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for _, gone := range []string{"TypeGoSocket", "TypeMicroservices"} {
			if strings.Contains(string(b), gone) {
				t.Errorf("%s still references removed type %q", path, gone)
			}
		}
	}
}

// goSourceFiles lists the repo's .go files, skipping trees that are not
// live source. docs/ carries the 0.1 design note this package was built
// from: it describes what was PROPOSED, and is history rather than a
// surface a stale reference could hide in.
func goSourceFiles(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case d.IsDir() && (d.Name() == ".git" || d.Name() == "docs" || d.Name() == "examples"):
			return fs.SkipDir
		case !d.IsDir() && filepath.Ext(path) == ".go":
			out = append(out, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	return out
}

func TestGetTemplates(t *testing.T) {
	templates := GetTemplates(TypeGoBinary)
	if len(templates) == 0 {
		t.Error("expected at least one template")
	}

	found := false
	for _, l := range templates {
		if l.Name == "domain:cli" {
			found = true
			break
		}
	}
	if !found {
		t.Error("missing domain:cli label in go-binary template")
	}
}
