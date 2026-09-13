package core

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// RenderSteps renders an expanded step list against scope, in an order
// where every steps.<id>.<field> reference and due.after already has its
// target rendered. The result keeps the input order. scope.Steps is
// filled as it goes; a caller that passes a map keeps the values.
func RenderSteps(steps []RecipeStep, scope Scope) ([]RecipeStep, error) {
	order, err := renderOrder(steps)
	if err != nil {
		return nil, err
	}
	if scope.Steps == nil {
		scope.Steps = map[string]map[string]any{}
	}
	out := make([]RecipeStep, len(steps))
	for _, i := range order {
		r, err := RenderStep(steps[i], scope)
		if err != nil {
			return nil, err
		}
		if err := resolveDue(&r, scope); err != nil {
			return nil, fmt.Errorf("step %s: due: %w", r.ID, err)
		}
		scope.Steps[r.ID] = stepFieldValues(r)
		out[i] = r
	}
	return out, nil
}

// renderOrder returns step indices in a topological order over depends_on
// plus the implicit edges of steps.* references and due.after. Ties keep
// recipe order. A cycle through implicit edges is an error, like a
// depends_on cycle; references to unknown ids are left for RenderStep to
// report.
func renderOrder(steps []RecipeStep) ([]int, error) {
	index := make(map[string]int, len(steps))
	for i := range steps {
		index[steps[i].ID] = i
	}
	deps := make([][]int, len(steps))
	for i := range steps {
		for _, id := range renderDeps(steps[i]) {
			if j, ok := index[id]; ok && j != i {
				deps[i] = append(deps[i], j)
			}
		}
	}
	done := make([]bool, len(steps))
	order := make([]int, 0, len(steps))
	for len(order) < len(steps) {
		progressed := false
		for i := range steps {
			if done[i] || !allDone(deps[i], done) {
				continue
			}
			done[i] = true
			order = append(order, i)
			progressed = true
		}
		if !progressed {
			return nil, fmt.Errorf("render order cycle through steps.* references: %s", strings.Join(pendingIDs(steps, done), ", "))
		}
	}
	return order, nil
}

func allDone(deps []int, done []bool) bool {
	for _, d := range deps {
		if !done[d] {
			return false
		}
	}
	return true
}

func pendingIDs(steps []RecipeStep, done []bool) []string {
	var ids []string
	for i := range steps {
		if !done[i] {
			ids = append(ids, steps[i].ID)
		}
	}
	return ids
}

// renderDeps lists the step ids a step must be rendered after.
func renderDeps(s RecipeStep) []string {
	ids := append([]string(nil), s.DependsOn...)
	if s.Due.After != "" {
		ids = append(ids, s.Due.After)
	}
	for _, ref := range stepRefs(s) {
		if rest, ok := strings.CutPrefix(ref, nsSteps+"."); ok {
			id, _, _ := strings.Cut(rest, ".")
			ids = append(ids, id)
		}
	}
	return ids
}

func isRecipeDuration(s string) bool {
	_, err := ParseRecipeDuration(s)
	return err == nil
}

// stepFieldValues is what steps.<id>.<field> exposes of a rendered step.
func stepFieldValues(s RecipeStep) map[string]any {
	return map[string]any{
		fieldID:          s.ID,
		fieldTitle:       s.Title,
		fieldDescription: s.Description,
		fieldDue:         s.Due.Raw,
		fieldAssignee:    s.Assignee,
		fieldWhen:        s.When,
		fieldAgent:       s.Agent,
		fieldKind:        string(s.EffectiveKind()),
	}
}

// dueArithmetic matches "<base> + <offset>" / "<base> - <offset>" with
// spaces around the operator, so "+2d" and "2026-10-01" alone never match.
var dueArithmetic = regexp.MustCompile(`^(.+?)\s+([+-])\s+(\S+)$`)

// dueLayouts are the absolute forms due arithmetic understands; a base in
// any other form (natural language) is passed through for the due parser.
var dueLayouts = []string{"2006-01-02", time.RFC3339, "2006-01-02T15:04", "2006-01-02 15:04"}

// resolveDue collapses either due form into Raw: the structured form reads
// its base from the referenced step, the string form from its own text.
func resolveDue(s *RecipeStep, scope Scope) error {
	if s.Due.After != "" {
		fields, ok := scope.Steps[s.Due.After]
		if !ok {
			return fmt.Errorf("after: step %q is not rendered", s.Due.After)
		}
		base, isText := fields[fieldDue].(string)
		if !isText || base == "" {
			return fmt.Errorf("after: step %q has no due", s.Due.After)
		}
		resolved, err := applyDueOffset(base, s.Due.Offset)
		if err != nil {
			return err
		}
		s.Due = DueSpec{Raw: resolved}
		return nil
	}
	m := dueArithmetic.FindStringSubmatch(s.Due.Raw)
	if m == nil || !isRecipeDuration(m[3]) {
		return nil // no offset; leave the text to the due parser
	}
	resolved, err := applyDueOffset(m[1], m[2]+m[3])
	if err != nil {
		return err
	}
	s.Due.Raw = resolved
	return nil
}

// applyDueOffset adds a signed recipe duration to an absolute base and
// formats it back in the base's layout. A base that is not absolute is
// returned as "<base> +|- <offset>" for the materializer's parser.
func applyDueOffset(base, offset string) (string, error) {
	if offset == "" {
		return base, nil
	}
	sign, magnitude := "+", offset
	if magnitude[0] == '+' || magnitude[0] == '-' {
		sign, magnitude = magnitude[:1], magnitude[1:]
	}
	d, err := ParseRecipeDuration(magnitude)
	if err != nil {
		return "", fmt.Errorf("offset: %w", err)
	}
	if sign == "-" {
		d = -d
	}
	for _, layout := range dueLayouts {
		if t, err := time.Parse(layout, base); err == nil {
			return t.Add(d).Format(layout), nil
		}
	}
	return base + " " + sign + " " + magnitude, nil
}
