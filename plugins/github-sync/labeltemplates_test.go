package main

// Round-trip coverage between the seeded label templates and this
// plugin's own label classifier.
//
// The templates and the mapper are two hand-maintained vocabularies that
// must agree, and for two labels they did not: `label init` seeded bare
// `feat` and `fix`, which mapLabelsToTask discards at its colon check.
// Asserting the agreement by reading both files is exactly how that
// drift survived, so this drives the REAL classifier over the REAL
// template output instead.

import (
	"strings"
	"testing"

	"github.com/google/go-github/v69/github"
	"hop.top/tlc/internal/labels"
)

// templateGitHubLabels renders one project type's templates as the
// *github.Label values an issue would actually carry.
func templateGitHubLabels(pt labels.ProjectType) ([]*github.Label, []string) {
	tmpl := labels.GetTemplates(pt)
	out := make([]*github.Label, 0, len(tmpl))
	names := make([]string, 0, len(tmpl))
	for _, l := range tmpl {
		name := l.Name
		out = append(out, &github.Label{Name: &name})
		names = append(names, name)
	}
	return out, names
}

// TestSeededLabelsSurviveClassification is the acceptance property: every
// label `label init` seeds must be recognised by the pull-direction
// classifier as SOMETHING — a priority, an effort, a status, or a tag.
// A label that reaches none of those buckets was dropped by the colon
// check and can never come back from the forge.
func TestSeededLabelsSurviveClassification(t *testing.T) {
	for _, pt := range []labels.ProjectType{
		labels.TypeGoBinary,
		labels.TypeReactFrontend,
		labels.TypePythonMVC,
		labels.TypeGeneric,
	} {
		t.Run(string(pt), func(t *testing.T) {
			// One label at a time: feeding the whole set at once would
			// let a single recognised label mask every dropped one,
			// because priority/effort/status are scalar sinks.
			for _, name := range namesFor(pt) {
				task := &Task{Meta: map[string]interface{}{}}
				lbl := name
				in, hasBlocked := mapLabelsToTask([]*github.Label{{Name: &lbl}}, task)

				classified := task.Priority != "" ||
					task.Effort != "" ||
					len(task.Tags) > 0 ||
					in || hasBlocked

				if !classified {
					t.Errorf("label %q was discarded by mapLabelsToTask; "+
						"`label init` would seed a label no pull can read back", name)
				}
			}
		})
	}
}

// TestSeededPriorityLabelsMapToPriorities pins the priority axis to the
// plugin's own pull table, not to a name list retyped in the test: a
// renamed priority label that no longer maps is the same defect class as
// a bare label.
func TestSeededPriorityLabelsMapToPriorities(t *testing.T) {
	seen := map[string]bool{}
	for _, name := range namesFor(labels.TypeGeneric) {
		if !strings.HasPrefix(name, "priority:") {
			continue
		}
		task := &Task{Meta: map[string]interface{}{}}
		lbl := name
		mapLabelsToTask([]*github.Label{{Name: &lbl}}, task)
		if task.Priority == "" {
			t.Errorf("seeded %q maps to no priority", name)
			continue
		}
		seen[task.Priority] = true
	}
	// Every priority the push direction can emit must be reachable from
	// a seeded label, or a pushed issue comes back priority-less.
	for _, p := range priorityToLabel {
		reverse := labelToPriority[p]
		if !seen[reverse] {
			t.Errorf("no seeded label yields priority %q (label %q)", reverse, p)
		}
	}
}

// TestSeededEffortLabelsMapToEfforts is the effort counterpart: the
// plugin has emitted and consumed effort:* all along while no template
// defined them, so pushes created them with an arbitrary forge colour.
func TestSeededEffortLabelsMapToEfforts(t *testing.T) {
	seen := map[string]bool{}
	for _, name := range namesFor(labels.TypeGeneric) {
		if !strings.HasPrefix(name, "effort:") {
			continue
		}
		task := &Task{Meta: map[string]interface{}{}}
		lbl := name
		mapLabelsToTask([]*github.Label{{Name: &lbl}}, task)
		if task.Effort == "" {
			t.Errorf("seeded %q maps to no effort", name)
			continue
		}
		seen[task.Effort] = true
	}
	for effort := range effortToLabel {
		if !seen[effort] {
			t.Errorf("no seeded label yields effort %q", effort)
		}
	}
}

// TestPushedLabelsAreSeeded closes the loop the other way: every label
// buildPushLabels can put on an issue should be one `label init` already
// created, so the forge is not silently accumulating labels with default
// colours.
func TestPushedLabelsAreSeeded(t *testing.T) {
	seeded := map[string]bool{}
	for _, name := range namesFor(labels.TypeGeneric) {
		seeded[name] = true
	}

	blocked := "blocked by #1"
	cases := []*Task{
		{Status: "IN_PROGRESS", Priority: "P0", Effort: "XS"},
		{Status: "TODO", Priority: "P3", Effort: "XL"},
		{Status: "TODO", BlockedReason: &blocked},
	}
	for _, task := range cases {
		for _, l := range buildPushLabels(task) {
			if !seeded[l] {
				t.Errorf("push emits %q but `label init` never seeds it", l)
			}
		}
	}
}

func namesFor(pt labels.ProjectType) []string {
	_, names := templateGitHubLabels(pt)
	return names
}
