package core

import (
	"os"
	"strings"
	"testing"
)

func TestRewritePlanBlockedByRefs_ReplacesQuotedForms(t *testing.T) {
	path := writeTempPlan(t, `---
title: Before
tasks:
  - title: "A"
    blocked-by: ["alpha#2", 'alpha#3']
---
# body
`)
	if err := RewritePlanBlockedByRefs(path, map[string]string{
		"alpha#2": "T-0002",
		"alpha#3": "T-0003",
	}); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	got, _ := os.ReadFile(path)
	s := string(got)
	for _, bad := range []string{"alpha#2", "alpha#3"} {
		if strings.Contains(s, bad) {
			t.Errorf("still contains %q:\n%s", bad, s)
		}
	}
	for _, good := range []string{`"T-0002"`, `"T-0003"`} {
		if !strings.Contains(s, good) {
			t.Errorf("missing %q in:\n%s", good, s)
		}
	}
}

func TestRewritePlanBlockedByRefs_EmptyMapIsNoop(t *testing.T) {
	path := writeTempPlan(t, "hello\n")
	if err := RewritePlanBlockedByRefs(path, nil); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "hello\n" {
		t.Errorf("file changed: %q", got)
	}
}

// TestRewritePlanBlockedByRefs_DoesNotTouchBody verifies that a
// quoted "<track>#<N>" in the markdown body (outside the YAML
// frontmatter) is left alone, even when the same string is being
// rewritten inside the frontmatter.
func TestRewritePlanBlockedByRefs_DoesNotTouchBody(t *testing.T) {
	path := writeTempPlan(t, `---
title: Mixed
tasks:
  - title: "A"
    blocked-by: ["alpha#2"]
---
# Body

Prose mentioning "alpha#2" for documentation reasons.
`)
	if err := RewritePlanBlockedByRefs(path, map[string]string{
		"alpha#2": "T-0002",
	}); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	got, _ := os.ReadFile(path)
	s := string(got)
	// Frontmatter: rewritten.
	if !strings.Contains(s, `blocked-by: ["T-0002"]`) {
		t.Errorf("frontmatter not rewritten:\n%s", s)
	}
	// Body: untouched.
	if !strings.Contains(s, `Prose mentioning "alpha#2"`) {
		t.Errorf("body was incorrectly rewritten:\n%s", s)
	}
}

// TestRewritePlanBlockedByRefs_PreservesFileMode verifies that a
// plan file with non-default mode (e.g., 0600) keeps that mode
// after rewriting.
func TestRewritePlanBlockedByRefs_PreservesFileMode(t *testing.T) {
	path := writeTempPlan(t, `---
title: Secret
tasks:
  - title: "A"
    blocked-by: ["alpha#1"]
---
`)
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	if err := RewritePlanBlockedByRefs(path, map[string]string{
		"alpha#1": "T-0001",
	}); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600", info.Mode().Perm())
	}
}

// TestRewritePlanBlockedByRefs_NoFrontmatterIsNoop verifies that a
// file without YAML frontmatter is never rewritten, even if it
// contains matching strings.
func TestRewritePlanBlockedByRefs_NoFrontmatterIsNoop(t *testing.T) {
	path := writeTempPlan(t, `Not frontmatter.

Has "alpha#1" in it, but no leading ---.
`)
	before, _ := os.ReadFile(path)
	if err := RewritePlanBlockedByRefs(path, map[string]string{
		"alpha#1": "T-0001",
	}); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Errorf("file without frontmatter was rewritten:\n%s", after)
	}
}
