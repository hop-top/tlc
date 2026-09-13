package core

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Selector picks a subset of expanded steps by 1-based ordinal or by id.
// Ids are the stable form: ordinals shift when a recipe gains a step.
type Selector struct {
	Ordinals map[int]bool
	IDs      map[string]bool
}

// Empty reports whether the selector keeps everything.
func (s Selector) Empty() bool { return len(s.Ordinals) == 0 && len(s.IDs) == 0 }

var (
	selectorOrdinal = regexp.MustCompile(`^\d+$`)
	selectorRange   = regexp.MustCompile(`^(\d+)-(\d+)$`)
	selectorNumeric = regexp.MustCompile(`^[\d-]+$`)
)

// ParseTaskSelector parses repeated --task values: "1-5,7", "12", "lint",
// "sign-off". A piece made only of digits and dashes must be an ordinal or
// an ascending range; anything else must be a step id.
func ParseTaskSelector(args []string) (Selector, error) {
	sel := Selector{Ordinals: map[int]bool{}, IDs: map[string]bool{}}
	for _, arg := range args {
		for _, piece := range strings.Split(arg, ",") {
			if err := sel.addPiece(strings.TrimSpace(piece)); err != nil {
				return Selector{}, err
			}
		}
	}
	return sel, nil
}

func (s Selector) addPiece(piece string) error {
	switch {
	case piece == "":
		return fmt.Errorf("--task: empty selection")
	case selectorOrdinal.MatchString(piece):
		n, err := strconv.Atoi(piece)
		if err != nil || n < 1 {
			return fmt.Errorf("--task %s: ordinals start at 1", piece)
		}
		s.Ordinals[n] = true
	case selectorRange.MatchString(piece):
		m := selectorRange.FindStringSubmatch(piece)
		lo, errLo := strconv.Atoi(m[1])
		hi, errHi := strconv.Atoi(m[2])
		if errLo != nil || errHi != nil || lo < 1 || hi < lo {
			return fmt.Errorf("--task %s: want an ascending range starting at 1, like 1-5", piece)
		}
		for n := lo; n <= hi; n++ {
			s.Ordinals[n] = true
		}
	case selectorNumeric.MatchString(piece) || !stepIDRe.MatchString(piece):
		return fmt.Errorf("--task %s: want an ordinal (3), a range (1-5) or a step id (lint)", piece)
	default:
		s.IDs[piece] = true
	}
	return nil
}

// Select keeps the selected steps in recipe order. A dependency on an
// unselected step is dropped and reported under the dependent's id, or
// pulled in with its closure when withDeps is set. Ordinals are preserved
// so a later --task means the same step.
func Select(steps []RecipeStep, sel Selector, withDeps bool) ([]RecipeStep, map[string][]string, error) {
	dropped := map[string][]string{}
	if sel.Empty() {
		return cloneSteps(steps), dropped, nil
	}
	keep, err := selectedIDs(steps, sel)
	if err != nil {
		return nil, nil, err
	}
	if withDeps {
		addDependencyClosure(steps, keep)
	}
	var kept []RecipeStep
	for i := range steps {
		if !keep[steps[i].ID] {
			continue
		}
		c := copyStep(steps[i])
		c.DependsOn = nil
		for _, d := range steps[i].DependsOn {
			if keep[d] {
				c.DependsOn = append(c.DependsOn, d)
			} else {
				dropped[c.ID] = append(dropped[c.ID], d)
			}
		}
		kept = append(kept, c)
	}
	return kept, dropped, nil
}

func cloneSteps(steps []RecipeStep) []RecipeStep {
	out := make([]RecipeStep, len(steps))
	for i := range steps {
		out[i] = copyStep(steps[i])
	}
	return out
}

func selectedIDs(steps []RecipeStep, sel Selector) (map[string]bool, error) {
	byOrdinal := make(map[int]string, len(steps))
	byID := make(map[string]bool, len(steps))
	for i := range steps {
		ord := steps[i].Ordinal
		if ord == 0 {
			ord = i + 1
		}
		byOrdinal[ord] = steps[i].ID
		byID[steps[i].ID] = true
	}
	keep := map[string]bool{}
	for _, n := range sortedInts(sel.Ordinals) {
		id, ok := byOrdinal[n]
		if !ok {
			return nil, fmt.Errorf("--task %d: no step with ordinal %d (the recipe has %d steps)", n, n, len(steps))
		}
		keep[id] = true
	}
	for _, id := range sortedKeys(sel.IDs) {
		if !byID[id] {
			return nil, fmt.Errorf("--task %s: no step with id %q", id, id)
		}
		keep[id] = true
	}
	return keep, nil
}

// addDependencyClosure marks every transitive dependency of a kept step.
func addDependencyClosure(steps []RecipeStep, keep map[string]bool) {
	byID := indexSteps(steps)
	var walk func(string)
	walk = func(id string) {
		for _, d := range byID[id].DependsOn {
			if !keep[d] {
				keep[d] = true
				walk(d)
			}
		}
	}
	for _, id := range sortedKeys(keep) {
		walk(id)
	}
}

func sortedInts(m map[int]bool) []int {
	out := make([]int, 0, len(m))
	for n := range m {
		out = append(out, n)
	}
	sort.Ints(out)
	return out
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
