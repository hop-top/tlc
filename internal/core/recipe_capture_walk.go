package core

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
)

// Step-walking helpers capture needs that hop.top/flo/spec keeps
// unexported. They mirror its walkStepStrings, copyStep, rewriteStepRefs
// and templateValue.

var (
	// stepIDRe bounds authored step ids; "/" is reserved for expansion.
	stepIDRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	// recipePlaceholder matches {{ref}}: a bare var name or a dotted path.
	recipePlaceholder = regexp.MustCompile(`\{\{\s*([A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z0-9_/-]+)*)\s*\}\}`)
	// recipeStepRef matches a results.<id>. or steps.<id>. reference.
	recipeStepRef = regexp.MustCompile(`\b(results|steps)\.([A-Za-z0-9_/-]+)\.`)
)

// templateValue renders a bound value the way a task field expects it.
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

// copyStep deep-copies the parts of a step that rewriting mutates.
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
	if s.Check != nil {
		g := *s.Check
		out.Check = &g
	}
	out.With = cloneAnyMap(s.With)
	out.Outputs = cloneAnyMap(s.Outputs)
	return out
}

// walkStepStrings visits every templatable string of a step in place.
func walkStepStrings(s *RecipeStep, fn func(*string)) {
	for _, p := range []*string{&s.Title, &s.Description, &s.Assignee, &s.Due.Raw, &s.When, &s.Until, &s.Agent} {
		fn(p)
	}
	if s.Exec != nil {
		for i := range s.Exec.Argv {
			fn(&s.Exec.Argv[i])
		}
		fn(&s.Exec.Cwd)
		for k, v := range s.Exec.Env {
			fn(&v)
			s.Exec.Env[k] = v
		}
	}
	if s.Human != nil {
		fn(&s.Human.Assignee)
	}
	if s.Check != nil {
		fn(&s.Check.Contract)
	}
	walkAnyMap(s.With, fn)
	walkAnyMap(s.Outputs, fn)
	fn(&s.ForEach)
}

func walkAnyMap(m map[string]any, fn func(*string)) {
	for k, v := range m {
		m[k] = walkAny(v, fn)
	}
}

func walkAny(v any, fn func(*string)) any {
	switch t := v.(type) {
	case string:
		fn(&t)
		return t
	case map[string]any:
		walkAnyMap(t, fn)
		return t
	case []any:
		for i := range t {
			t[i] = walkAny(t[i], fn)
		}
		return t
	}
	return v
}

func cloneAnyMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = cloneAny(v)
	}
	return out
}

func cloneAny(v any) any {
	switch t := v.(type) {
	case map[string]any:
		return cloneAnyMap(t)
	case []any:
		out := make([]any, len(t))
		for i := range t {
			out[i] = cloneAny(t[i])
		}
		return out
	}
	return v
}

// rewriteStepRefs replaces results.<id>. and steps.<id>. references whose
// id is in rename, and renames a structured due's After the same way.
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
