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
			// changed behavior.
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
		{
			// The package.json contention, stated positively: a
			// package.json with no App entrypoint used to reach none of
			// the branches and fall out as generic.
			name:     "Node service",
			files:    []string{"package.json", "src/server.ts"},
			expected: TypeNodeBackend,
		},
		{
			// A Go module with neither root main.go nor cmd/.
			name:     "Go library",
			files:    []string{"go.mod", "parser.go"},
			expected: TypeLibrary,
		},
		{
			name:     "Go workspace",
			files:    []string{"go.work", "go.mod", "cmd/tool/main.go"},
			expected: TypeMonorepo,
		},
		{
			name:     "pnpm workspace",
			files:    []string{"pnpm-workspace.yaml", "package.json"},
			expected: TypeMonorepo,
		},
		{
			name:     "Turborepo",
			files:    []string{"turbo.json", "package.json"},
			expected: TypeMonorepo,
		},
		{
			name:     "Terraform module",
			files:    []string{"main.tf", "variables.tf"},
			expected: TypeInfra,
		},
		{
			name:     "Helm chart repo",
			files:    []string{"helm/app/Chart.yaml"},
			expected: TypeInfra,
		},
		{
			name:     "Kubernetes manifests",
			files:    []string{"k8s/deployment.yaml"},
			expected: TypeInfra,
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
//
// Every advertised type is currently reachable, so no exemption is
// claimed. Two fixtures below are worth reading as arguments rather than
// as data:
//
// TypeLibrary is reached through Go — a bare `go.mod` with no root
// `main.go` and no `cmd/`. It is NOT reachable through Node, and that
// is a limit of the signal rather than a gap in the policy: a Node
// library and a Node service declare the identical `package.json`, so
// there is nothing to detect on. `--type library` covers that repo. The
// distinction the rule cares about is whether the TYPE is reachable at
// all, and it is.
//
// TypeNodeBackend's fixture is a lone `package.json` — the catch-all
// arm — because that IS the signal. Adding a `src/server.ts` beside it
// would test a file the detector never reads and hide the fact that
// every non-React Node repo lands here.
func TestDetectionReachesEveryAdvertisedType(t *testing.T) {
	fixtures := map[ProjectType][]string{
		TypeGoBinary:      {"go.mod", "main.go"},
		TypeNodeBackend:   {"package.json"},
		TypeReactFrontend: {"package.json", "src/App.tsx"},
		TypePythonMVC:     {"manage.py"},
		TypeLibrary:       {"go.mod"},
		TypeMonorepo:      {"go.work"},
		TypeInfra:         {"main.tf"},
		TypeGeneric:       {"README.md"},
	}

	for _, pt := range AllProjectTypes() {
		files, ok := fixtures[pt]
		if !ok {
			t.Errorf("%s is advertised but no fixture shows detection reaching it", pt)
			continue
		}
		dir := t.TempDir()
		writeFixture(t, dir, files)
		if got := DetectProjectType(dir); got != pt {
			t.Errorf("DetectProjectType(%v) = %v, want %v", files, got, pt)
		}
	}
}

// TestDetectionContention pins the cases where two types compete for the
// same marker file. These are the regressions a reordering of
// DetectProjectType would cause, and none of them is caught by the
// happy-path table: every case below is a directory where the WRONG
// answer is also a plausible one.
//
// Each case names the branch that must lose, so a failure says which
// ordering broke rather than only that something did.
func TestDetectionContention(t *testing.T) {
	cases := []struct {
		name    string
		files   []string
		want    ProjectType
		notWant ProjectType
		because string
	}{
		{
			name:    "react app is not a node backend",
			files:   []string{"package.json", "src/App.tsx", "src/index.ts"},
			want:    TypeReactFrontend,
			notWant: TypeNodeBackend,
			because: "src/App.tsx is the specific signal; package.json is the general one",
		},
		{
			name:    "react app with jsx entrypoint is not a node backend",
			files:   []string{"package.json", "src/App.jsx"},
			want:    TypeReactFrontend,
			notWant: TypeNodeBackend,
			because: "both App spellings must be tested before the catch-all",
		},
		{
			name:    "go binary is not a library",
			files:   []string{"go.mod", "main.go"},
			want:    TypeGoBinary,
			notWant: TypeLibrary,
			because: "a root main.go is an entrypoint",
		},
		{
			name:    "go binary under cmd is not a library",
			files:   []string{"go.mod", "cmd/tool/main.go"},
			want:    TypeGoBinary,
			notWant: TypeLibrary,
			because: "cmd/ is the other place an entrypoint lives",
		},
		{
			name:    "go binary is not a monorepo",
			files:   []string{"go.mod", "main.go"},
			want:    TypeGoBinary,
			notWant: TypeMonorepo,
			because: "no workspace file; go.mod alone never means monorepo",
		},
		{
			name:    "go workspace beats the go.mod branch",
			files:   []string{"go.work", "go.mod", "main.go"},
			want:    TypeMonorepo,
			notWant: TypeGoBinary,
			because: "a workspace root carries the same markers its members do",
		},
		{
			name:    "pnpm workspace beats the package.json branch",
			files:   []string{"pnpm-workspace.yaml", "package.json", "src/App.tsx"},
			want:    TypeMonorepo,
			notWant: TypeReactFrontend,
			because: "the root composes the apps; it is not one of them",
		},
		{
			name:    "terraform repo with a go.mod is infra",
			files:   []string{"main.tf", "go.mod", "main.go"},
			want:    TypeInfra,
			notWant: TypeGoBinary,
			because: "in a provider repo the infrastructure is the product",
		},
		{
			name:    "monorepo beats infra",
			files:   []string{"go.work", "main.tf"},
			want:    TypeMonorepo,
			notWant: TypeInfra,
			because: "an infra directory in a workspace is a member, not the root",
		},
		{
			name:    "python project is not a node backend",
			files:   []string{"manage.py", "requirements.txt"},
			want:    TypePythonMVC,
			notWant: TypeNodeBackend,
			because: "no package.json; the branches must not leak into each other",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFixture(t, dir, tc.files)
			got := DetectProjectType(dir)
			if got == tc.notWant {
				t.Errorf("DetectProjectType(%v) = %v, the contending type; want %v (%s)",
					tc.files, got, tc.want, tc.because)
			}
			if got != tc.want {
				t.Errorf("DetectProjectType(%v) = %v, want %v (%s)",
					tc.files, got, tc.want, tc.because)
			}
		})
	}
}

// TestBareGoModIsALibrary states the one behavior change detection
// makes to an already-detected shape, so it is a decision on the record
// rather than a side effect.
//
// Before library existed, `go.mod` alone answered go-binary. It now
// answers library, because a module with neither a root `main.go` nor a
// `cmd/` tree builds no binary — that is the definition of a Go library,
// not a near-miss. The two go-binary fixtures both carry an entrypoint
// and are unaffected; what changed is the answer for a directory that
// never described a binary in the first place.
func TestBareGoModIsALibrary(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, []string{"go.mod", "parser.go", "parser_test.go"})
	if got := DetectProjectType(dir); got != TypeLibrary {
		t.Errorf("DetectProjectType(bare go.mod) = %v, want %v", got, TypeLibrary)
	}
}

// writeFixture materializes a fixture file list under dir, creating
// parent directories so a case can name "cmd/tool/main.go" directly.
func writeFixture(t *testing.T, dir string, files []string) {
	t.Helper()
	for _, f := range files {
		p := filepath.Join(dir, f)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatal(err)
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
