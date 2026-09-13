package core

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
)

// recipePlaceholder matches {{ref}} where ref is a bare var name or a
// dotted path whose later segments may contain the "/" and "-" that
// expanded step ids carry: {{pr}}, {{vars.pr}}, {{subject.title}},
// {{run.iteration}}, {{steps.planning.due}}, {{results.sec/scan.stdout}}.
var recipePlaceholder = regexp.MustCompile(`\{\{\s*([A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z0-9_/-]+)*)\s*\}\}`)

// recipeStepRef matches a results.<id>. or steps.<id>. reference wherever
// it appears — inside a placeholder or bare in a condition — so expansion
// can rewrite ids in one pass.
var recipeStepRef = regexp.MustCompile(`\b(results|steps)\.([A-Za-z0-9_/-]+)\.`)

// Scope is the create-time template environment. Results are not in it:
// {{results.*}} stays verbatim for the executor to resolve at dispatch.
type Scope struct {
	Vars    map[string]any
	Subject map[string]any
	Run     map[string]any
	Steps   map[string]map[string]any
}

// Render substitutes every placeholder in s from scope. Unknown references
// are errors so a typo never ships as literal braces in a task.
func Render(s string, scope Scope) (string, error) {
	var firstErr error
	out := recipePlaceholder.ReplaceAllStringFunc(s, func(match string) string {
		ref := recipePlaceholder.FindStringSubmatch(match)[1]
		if strings.HasPrefix(ref, "results.") {
			return match
		}
		v, err := scope.lookup(ref)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			return match
		}
		return templateValue(v)
	})
	if firstErr != nil {
		return "", firstErr
	}
	return out, nil
}

func (sc Scope) lookup(ref string) (any, error) {
	ns, rest, dotted := strings.Cut(ref, ".")
	if !dotted {
		return scopeKey(sc.Vars, ref, "var")
	}
	switch ns {
	case nsVars:
		return scopeKey(sc.Vars, rest, "var")
	case nsSubject:
		return scopeKey(sc.Subject, rest, "subject field")
	case nsRun:
		return scopeKey(sc.Run, rest, "run field")
	case nsSteps:
		id, field, ok := strings.Cut(rest, ".")
		if !ok {
			return nil, fmt.Errorf("{{%s}}: want steps.<id>.<field>", ref)
		}
		fields, found := sc.Steps[id]
		if !found {
			return nil, fmt.Errorf("{{%s}}: step %q is not rendered yet or does not exist", ref, id)
		}
		return scopeKey(fields, field, "step field")
	}
	return nil, fmt.Errorf("{{%s}}: unknown namespace %q", ref, ns)
}

func scopeKey(m map[string]any, key, what string) (any, error) {
	v, ok := m[key]
	if !ok {
		return nil, fmt.Errorf("unknown %s %q", what, key)
	}
	return v, nil
}

// templateValue renders a bound value the way a task field expects it:
// lists join with commas, everything else prints plainly.
func templateValue(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case []string:
		return strings.Join(t, ",")
	case []any:
		parts := make([]string, len(t))
		for i, e := range t {
			parts[i] = fmt.Sprint(e)
		}
		return strings.Join(parts, ",")
	}
	return fmt.Sprint(v)
}

// RenderStep renders every templatable field of step against scope and
// returns the result as a copy; the input is never mutated.
func RenderStep(step RecipeStep, scope Scope) (RecipeStep, error) {
	out := copyStep(step)
	var firstErr error
	walkStepStrings(&out, func(s *string) {
		r, err := Render(*s, scope)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			return
		}
		*s = r
	})
	if firstErr != nil {
		return RecipeStep{}, fmt.Errorf("step %s: %w", step.ID, firstErr)
	}
	return out, nil
}

// copyStep deep-copies the parts of a step that rendering or expansion
// mutate. Nested Steps share their backing array; expansion never edits
// them in place.
func copyStep(s RecipeStep) RecipeStep {
	out := s
	out.DependsOn = slices.Clone(s.DependsOn)
	if s.Exec != nil {
		e := *s.Exec
		e.Argv = slices.Clone(s.Exec.Argv)
		e.Env = maps.Clone(s.Exec.Env)
		out.Exec = &e
	}
	if s.Human != nil {
		h := *s.Human
		out.Human = &h
	}
	if s.Retry != nil {
		r := *s.Retry
		out.Retry = &r
	}
	if s.Gate != nil {
		g := *s.Gate
		out.Gate = &g
	}
	out.With = maps.Clone(s.With)
	return out
}

// walkStepStrings visits every templatable string of a step in place.
// It is the single list of "fields a template may appear in", shared by
// rendering, validation and expansion-time rewriting.
func walkStepStrings(s *RecipeStep, fn func(*string)) {
	for _, p := range []*string{&s.Title, &s.Description, &s.Assignee, &s.Due.Raw, &s.When, &s.Until, &s.Agent} {
		fn(p)
	}
	if s.Exec != nil {
		for i := range s.Exec.Argv {
			fn(&s.Exec.Argv[i])
		}
		fn(&s.Exec.Cwd)
		walkMapStrings(s.Exec.Env, fn)
	}
	if s.Human != nil {
		fn(&s.Human.Assignee)
	}
	if s.Gate != nil {
		fn(&s.Gate.Contract)
	}
	for k, v := range s.With {
		if str, ok := v.(string); ok {
			fn(&str)
			s.With[k] = str
		}
	}
}

func walkMapStrings(m map[string]string, fn func(*string)) {
	for k, v := range m {
		fn(&v)
		m[k] = v
	}
}

// stepRefs lists every placeholder reference in a step's templatable
// fields, in field order, without duplicates.
func stepRefs(s RecipeStep) []string {
	var refs []string
	seen := map[string]bool{}
	walkStepStrings(&s, func(p *string) {
		for _, m := range recipePlaceholder.FindAllStringSubmatch(*p, -1) {
			if !seen[m[1]] {
				seen[m[1]] = true
				refs = append(refs, m[1])
			}
		}
	})
	return refs
}

// rewriteStepRefs replaces results.<id>. and steps.<id>. references whose
// id is in rename, in placeholders and bare conditions alike, and renames
// a structured due's After the same way.
func rewriteStepRefs(s *RecipeStep, rename map[string]string) {
	walkStepStrings(s, func(p *string) {
		*p = recipeStepRef.ReplaceAllStringFunc(*p, func(m string) string {
			sub := recipeStepRef.FindStringSubmatch(m)
			if to, ok := rename[sub[2]]; ok {
				return sub[1] + "." + to + "."
			}
			return m
		})
	})
	if to, ok := rename[s.Due.After]; ok {
		s.Due.After = to
	}
}

// substituteVars replaces {{name}} and {{vars.name}} for every bound name
// with its literal replacement text; a binding keyed by a full dotted
// reference (run.iteration) matches that reference exactly. Expansion
// uses it to splice a parent's `with` expressions, a child's defaults and
// the iteration number into block steps.
func substituteVars(s *RecipeStep, bindings map[string]string) {
	walkStepStrings(s, func(p *string) {
		*p = recipePlaceholder.ReplaceAllStringFunc(*p, func(m string) string {
			ref := recipePlaceholder.FindStringSubmatch(m)[1]
			if v, ok := bindings[ref]; ok {
				return v
			}
			name := strings.TrimPrefix(ref, "vars.")
			if strings.Contains(name, ".") {
				return m
			}
			if v, ok := bindings[name]; ok {
				return v
			}
			return m
		})
	})
}
