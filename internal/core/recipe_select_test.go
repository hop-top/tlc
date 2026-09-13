package core

import (
	"reflect"
	"strings"
	"testing"
)

func selectFixture(t *testing.T) []RecipeStep {
	t.Helper()
	r := mustParse(t, `
recipe: release
version: 1
steps:
  - id: build
  - id: test
    depends_on: [build]
  - id: changelog
    depends_on: [test]
  - id: tag
    depends_on: [changelog]
  - id: publish
    depends_on: [tag]
`)
	steps, err := Expand(r, memLocator{})
	if err != nil {
		t.Fatalf("Expand: %v", err)
	}
	return steps
}

func TestParseTaskSelector(t *testing.T) {
	sel, err := ParseTaskSelector([]string{"1-2,5", "changelog", "3"})
	if err != nil {
		t.Fatalf("ParseTaskSelector: %v", err)
	}
	if !reflect.DeepEqual(sel.Ordinals, map[int]bool{1: true, 2: true, 3: true, 5: true}) || !sel.IDs["changelog"] {
		t.Errorf("selector = %+v", sel)
	}
	// A hyphenated id is an id, not a malformed range.
	if sel, err := ParseTaskSelector([]string{"sign-off"}); err != nil || !sel.IDs["sign-off"] {
		t.Errorf("hyphenated id: %v %+v", err, sel)
	}
	for _, bad := range []string{"0", "5-2", "", "-1", "1-", "Bad"} {
		if _, err := ParseTaskSelector([]string{bad}); err == nil {
			t.Errorf("%q accepted; want error", bad)
		}
	}
}

func TestSelect_DropsDepsOnUnselectedWithWarning(t *testing.T) {
	steps := selectFixture(t)
	sel, _ := ParseTaskSelector([]string{"1,3", "publish"})
	kept, dropped, err := Select(steps, sel, false)
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if got := stepIDs(kept); strings.Join(got, ",") != "build,changelog,publish" {
		t.Errorf("kept = %v", got)
	}
	// changelog lost its edge to test; publish lost its edge to tag.
	if len(kept[1].DependsOn) != 0 || len(kept[2].DependsOn) != 0 {
		t.Errorf("deps on unselected steps not dropped: %v %v", kept[1].DependsOn, kept[2].DependsOn)
	}
	want := map[string][]string{"changelog": {"test"}, "publish": {"tag"}}
	if !reflect.DeepEqual(dropped, want) {
		t.Errorf("dropped = %v; want %v", dropped, want)
	}
	// Ordinals stay the recipe's, so `--task 3` still means changelog later.
	if kept[1].Ordinal != 3 || kept[2].Ordinal != 5 {
		t.Errorf("ordinals renumbered: %d %d", kept[1].Ordinal, kept[2].Ordinal)
	}
}

func TestSelect_WithDepsPullsClosure(t *testing.T) {
	steps := selectFixture(t)
	sel, _ := ParseTaskSelector([]string{"tag"})
	kept, dropped, err := Select(steps, sel, true)
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if got := stepIDs(kept); strings.Join(got, ",") != "build,test,changelog,tag" {
		t.Errorf("kept = %v; want the closure in recipe order", got)
	}
	if len(dropped) != 0 || strings.Join(kept[3].DependsOn, ",") != "changelog" {
		t.Errorf("closure lost edges: dropped=%v tag.deps=%v", dropped, kept[3].DependsOn)
	}
}

func TestSelect_UnknownSelection(t *testing.T) {
	steps := selectFixture(t)
	for _, spec := range []string{"9", "deploy"} {
		sel, _ := ParseTaskSelector([]string{spec})
		if _, _, err := Select(steps, sel, false); err == nil || !strings.Contains(err.Error(), spec) {
			t.Errorf("selector %q: err = %v; want it to name the unknown selection", spec, err)
		}
	}
	// An empty selector keeps everything.
	kept, _, err := Select(steps, Selector{}, false)
	if err != nil || len(kept) != 5 {
		t.Errorf("empty selector: %v / %d", err, len(kept))
	}
}
