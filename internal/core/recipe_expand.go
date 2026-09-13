package core

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Locator resolves recipe references (name, name@version, or a path) and
// lists what is available.
type Locator interface {
	Locate(ref string) (*Recipe, error)
	List() ([]RecipeInfo, error)
}

// maxIncludeDepth bounds include nesting; cycles are refused earlier by
// the recipe-key stack.
const maxIncludeDepth = 8

// Expand flattens a recipe's include and repeat blocks into the ordered
// list of task steps that materialization creates, then validates that
// list as a whole. Included steps get ids <include>/<child>; unrolled
// steps get <block>/<i>/<child>. References and dependencies are rewritten
// to the expanded ids. The recipe itself is not mutated.
func Expand(r *Recipe, loc Locator) ([]RecipeStep, error) {
	ex := &expander{loc: loc, stack: []string{r.Key()}}
	steps, err := ex.expandList(r.Steps)
	if err != nil {
		return nil, fmt.Errorf("recipe %s: %w", r.Name, err)
	}
	for i := range steps {
		steps[i].Ordinal = i + 1
	}
	sc := refScope{vars: r.Vars, subject: r.Requires.Subject}
	if err := validateStepGraph(steps, sc); err != nil {
		return nil, fmt.Errorf("recipe %s (expanded): %w", r.Name, err)
	}
	if _, err := renderOrder(steps); err != nil {
		return nil, fmt.Errorf("recipe %s: %w", r.Name, err)
	}
	return steps, nil
}

type expander struct {
	loc   Locator
	stack []string // recipe keys being expanded, for the include cycle guard
}

// expandList expands one step list. Block ids are replaced by their leaf
// ids in every sibling's depends_on, so an edge on a block means "after
// the whole block".
func (ex *expander) expandList(steps []RecipeStep) ([]RecipeStep, error) {
	var out []RecipeStep
	leaves := map[string][]string{}
	for i := range steps {
		s := &steps[i]
		var (
			flat []RecipeStep
			lv   []string
			err  error
		)
		switch {
		case s.IsInclude():
			flat, lv, err = ex.expandInclude(s)
		case s.IsRepeat():
			flat, lv, err = ex.expandRepeat(s)
		default:
			out = append(out, copyStep(*s))
			continue
		}
		if err != nil {
			return nil, err
		}
		leaves[s.ID] = lv
		out = append(out, flat...)
	}
	for i := range out {
		out[i].DependsOn = replaceBlockDeps(out[i].DependsOn, leaves)
	}
	return out, nil
}

func replaceBlockDeps(deps []string, leaves map[string][]string) []string {
	var out []string
	for _, d := range deps {
		if lv, ok := leaves[d]; ok {
			out = append(out, lv...)
			continue
		}
		out = append(out, d)
	}
	return out
}

func (ex *expander) expandInclude(s *RecipeStep) ([]RecipeStep, []string, error) {
	child, err := ex.loc.Locate(s.Include)
	if err != nil {
		return nil, nil, fmt.Errorf("step %s: include %s: %w", s.ID, s.Include, err)
	}
	key := child.Key()
	if slices.Contains(ex.stack, key) {
		return nil, nil, fmt.Errorf("step %s: include cycle: %s -> %s", s.ID, strings.Join(ex.stack, " -> "), key)
	}
	if len(ex.stack) >= maxIncludeDepth {
		return nil, nil, fmt.Errorf("step %s: include nesting deeper than %d", s.ID, maxIncludeDepth)
	}
	bindings, err := bindIncludeVars(s, child)
	if err != nil {
		return nil, nil, err
	}
	ex.stack = append(ex.stack, key)
	flat, err := ex.expandList(child.Steps)
	ex.stack = ex.stack[:len(ex.stack)-1]
	if err != nil {
		return nil, nil, fmt.Errorf("step %s: include %s: %w", s.ID, key, err)
	}
	for i := range flat {
		substituteVars(&flat[i], bindings)
	}
	steps, leaves := prefixBlock(flat, s.ID, s.DependsOn)
	return steps, leaves, nil
}

// bindIncludeVars maps each child var to the literal text spliced in for
// it: the parent's `with` expression (rendered later in the parent's
// scope) or the child's default.
func bindIncludeVars(s *RecipeStep, child *Recipe) (map[string]string, error) {
	for k := range s.With {
		if _, ok := child.Vars[k]; !ok {
			return nil, fmt.Errorf("step %s: with.%s: recipe %s declares no such var", s.ID, k, child.Key())
		}
	}
	bindings := make(map[string]string, len(child.Vars))
	for name, def := range child.Vars {
		switch {
		case hasAnyKey(s.With, name):
			bindings[name] = templateValue(s.With[name])
		case def.Default != nil:
			bindings[name] = templateValue(def.Default)
		case def.Required:
			return nil, fmt.Errorf("step %s: include %s: required var %q is not bound; add it under with", s.ID, child.Key(), name)
		}
	}
	return bindings, nil
}

func hasAnyKey(m map[string]any, k string) bool {
	_, ok := m[k]
	return ok
}

func (ex *expander) expandRepeat(s *RecipeStep) ([]RecipeStep, []string, error) {
	var until *condition
	if s.Until != "" {
		c, err := parseCondition(s.Until)
		if err != nil {
			return nil, nil, fmt.Errorf("step %s: until: %w", s.ID, err)
		}
		until = &c
	}
	var out []RecipeStep
	prev := s.DependsOn
	prevPrefix := ""
	for i := 1; i <= s.Repeat; i++ {
		flat, err := ex.expandList(s.Steps)
		if err != nil {
			return nil, nil, fmt.Errorf("step %s: %w", s.ID, err)
		}
		iteration := map[string]string{"run.iteration": strconv.Itoa(i)}
		for j := range flat {
			substituteVars(&flat[j], iteration)
		}
		prefix := s.ID + "/" + strconv.Itoa(i)
		steps, leaves := prefixBlock(flat, prefix, prev)
		if until != nil && i > 1 {
			derived := derivedWhen(*until, s.Steps, prevPrefix)
			for j := range steps {
				steps[j].When = derived
			}
		}
		out = append(out, steps...)
		prev, prevPrefix = leaves, prefix
	}
	return out, prev, nil
}

// derivedWhen turns a block's until into the guard on iteration i: the
// negated condition, read against iteration i-1's results.
func derivedWhen(until condition, children []RecipeStep, prevPrefix string) string {
	rename := make(map[string]string, len(children))
	for i := range children {
		rename[children[i].ID] = prevPrefix + "/" + children[i].ID
	}
	probe := RecipeStep{When: until.lhs}
	rewriteStepRefs(&probe, rename)
	return condition{lhs: probe.When, op: until.op, rhs: until.rhs}.negate().String()
}

// prefixBlock namespaces a flattened block: ids and sibling references
// get "<prefix>/", roots inherit the block's own dependencies, and the
// block's leaves are returned for the edges that pointed at the block.
func prefixBlock(flat []RecipeStep, prefix string, inherit []string) ([]RecipeStep, []string) {
	rename := make(map[string]string, len(flat))
	for i := range flat {
		rename[flat[i].ID] = prefix + "/" + flat[i].ID
	}
	hasDependents := map[string]bool{}
	for i := range flat {
		st := &flat[i]
		for j, d := range st.DependsOn {
			hasDependents[d] = true
			if to, ok := rename[d]; ok {
				st.DependsOn[j] = to
			}
		}
		if len(st.DependsOn) == 0 {
			st.DependsOn = slices.Clone(inherit)
		}
		rewriteStepRefs(st, rename)
	}
	var leaves []string
	for i := range flat {
		if !hasDependents[flat[i].ID] {
			leaves = append(leaves, rename[flat[i].ID])
		}
		flat[i].ID = rename[flat[i].ID]
	}
	return flat, leaves
}
