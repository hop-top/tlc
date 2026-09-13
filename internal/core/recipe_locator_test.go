package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeRecipeFile(t *testing.T, dir, file, name, version string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(dir, file)
	body := "recipe: " + name + "\nversion: " + version + "\nsteps:\n  - id: a\n    title: from " + file + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

// locatorFixture lays out a project dir shadowing a user dir: the same
// recipe at two versions in the user dir, an older one in the project
// dir, and an unrelated file that must be ignored.
func locatorFixture(t *testing.T) (loc *DirLocator, root, project, user string) {
	t.Helper()
	root = t.TempDir()
	project = filepath.Join(root, "project")
	user = filepath.Join(root, "user")
	writeRecipeFile(t, user, "code-review-1.yaml", "code-review", "1.0.0")
	writeRecipeFile(t, user, "code-review-2.yaml", "code-review", "1.2.0")
	writeRecipeFile(t, user, "release.yml", "release", "0.9.0")
	writeRecipeFile(t, project, "review.yaml", "code-review", "1.1.0")
	if err := os.WriteFile(filepath.Join(project, "notes.txt"), []byte("recipe: nope\n"), 0o644); err != nil {
		t.Fatalf("write notes: %v", err)
	}
	return &DirLocator{Dirs: []string{project, user}}, root, project, user
}

func TestDirLocator_SearchOrder(t *testing.T) {
	loc, _, project, user := locatorFixture(t)
	// Unpinned: the first directory that has the recipe wins, highest
	// version within it.
	r, err := loc.Locate("code-review")
	if err != nil {
		t.Fatalf("Locate: %v", err)
	}
	if r.Version != "1.1.0" || r.Source != project {
		t.Errorf("unpinned = %s from %s; want 1.1.0 from the project dir", r.Version, r.Source)
	}
	// Pinned: searched across all dirs.
	r, err = loc.Locate("code-review@1.2.0")
	if err != nil {
		t.Fatalf("Locate pinned: %v", err)
	}
	if r.Version != "1.2.0" || r.Source != user {
		t.Errorf("pinned = %s from %s; want 1.2.0 from the user dir", r.Version, r.Source)
	}
	if _, err := loc.Locate("code-review@9.9.9"); err == nil {
		t.Error("unknown pinned version resolved")
	}
	// Highest semver, not lexical: 1.10.0 beats 1.9.0.
	writeRecipeFile(t, user, "release-9.yaml", "release", "1.9.0")
	writeRecipeFile(t, user, "release-10.yaml", "release", "1.10.0")
	r, err = loc.Locate("release")
	if err != nil {
		t.Fatalf("Locate release: %v", err)
	}
	if r.Version != "1.10.0" {
		t.Errorf("highest version = %s; want 1.10.0", r.Version)
	}
}

func TestDirLocator_PathAndNotFound(t *testing.T) {
	loc, root, project, _ := locatorFixture(t)
	// A file path resolves directly, and Path/Hash are populated.
	p := writeRecipeFile(t, root, "adhoc.yaml", "adhoc", "0.1.0")
	r, err := loc.Locate(p)
	if err != nil || r.Name != "adhoc" || r.Path != p || r.Hash == "" {
		t.Errorf("file path: %v %+v", err, r)
	}
	// Unknown name: a typed error that names the ref and the search path.
	_, err = loc.Locate("nope")
	var nf *RecipeNotFoundError
	if !asRecipeNotFound(err, &nf) || nf.Ref != "nope" || !strings.Contains(err.Error(), project) {
		t.Errorf("missing recipe: err = %v", err)
	}
	if !IsRecipeNotFound(err) {
		t.Errorf("IsRecipeNotFound = false for %v", err)
	}
}

func TestDirLocator_List(t *testing.T) {
	root := t.TempDir()
	writeRecipeFile(t, filepath.Join(root, "a"), "x.yaml", "x", "1.0.0")
	writeRecipeFile(t, filepath.Join(root, "b"), "x-old.yaml", "x", "0.9.0")
	writeRecipeFile(t, filepath.Join(root, "b"), "y.yaml", "y", "2.0.0")
	loc := &DirLocator{Dirs: []string{filepath.Join(root, "a"), filepath.Join(root, "b"), filepath.Join(root, "missing")}}
	infos, err := loc.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	got := make([]string, 0, len(infos))
	for _, i := range infos {
		got = append(got, i.Name+"@"+i.Version+":"+filepath.Base(i.Source))
	}
	want := "x@1.0.0:a,x@0.9.0:b,y@2.0.0:b"
	if strings.Join(got, ",") != want {
		t.Errorf("List = %v; want %s (dir order, then name, version desc)", got, want)
	}
}

func TestCompareRecipeVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.0", 0},
		{"1.10.0", "1.9.0", 1},
		{"1.0.0", "1.0.0-alpha.1", 1},
		{"1.0.0-alpha.2", "1.0.0-alpha.10", -1},
		{"2", "1.9.9", 1},
		{"0.1", "0.1.0", 0},
	}
	for _, tc := range cases {
		if got := CompareRecipeVersions(tc.a, tc.b); got != tc.want {
			t.Errorf("Compare(%s, %s) = %d; want %d", tc.a, tc.b, got, tc.want)
		}
	}
}
