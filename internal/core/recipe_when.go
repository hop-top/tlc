package core

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// whenResultsRef marks a `when:` that reads run-time state: results.<step>
// exists only once the step's task has run.
const whenResultsRef = "results."

// CreateTimeWhen evaluates a step's `when:` against the recipe vars alone,
// the environment the executor also gives it for everything but results.
//
// known is false when the condition cannot be decided at create time and
// is left to the executor: it is empty, it reads results.*, or it is not a
// single comparison (ErrNeedsCEL). err is non-nil only when known is false
// and the expression failed to evaluate for another reason, such as an
// undefined var; the executor blocks the task with the same error, so
// nothing is dropped on a guess.
func CreateTimeWhen(step RecipeStep, vars map[string]any) (verdict, known bool, err error) {
	when := strings.TrimSpace(step.When)
	if when == "" || strings.Contains(when, whenResultsRef) {
		return true, false, nil
	}
	ok, evalErr := EvalConditionEnv(when, CondEnv{Vars: vars})
	switch {
	case errors.Is(evalErr, ErrNeedsCEL):
		return true, false, nil
	case evalErr != nil:
		return true, false, evalErr
	}
	return ok, true, nil
}

// WhenDrop is the outcome of DropFalseWhen.
type WhenDrop struct {
	// Steps are the steps still to materialize, in input order, with every
	// dependency on a dropped step removed.
	Steps []RecipeStep
	// Dropped lists the ids of the steps whose `when:` is false.
	Dropped []string
	// Deps maps a kept step to the dependencies removed from it; the same
	// shape as the dependencies a --task selection drops.
	Deps map[string][]string
	// Warnings word each drop and each condition left to the executor
	// because it failed to evaluate.
	Warnings []string
}

// DropFalseWhen removes the steps whose `when:` is false for vars. A step
// the executor would skip at run time because of that same condition is
// not materialized, and what depended on it loses the edge exactly as when
// a --task selection leaves the step out: a skipped task is terminal, so
// its dependents run either way. Steps whose condition cannot be decided
// from vars, and steps whose condition is true, are kept untouched.
func DropFalseWhen(steps []RecipeStep, vars map[string]any) WhenDrop {
	out := WhenDrop{Deps: map[string][]string{}}
	gone := map[string]string{} // dropped step id → its condition
	for _, step := range steps {
		verdict, known, err := CreateTimeWhen(step, vars)
		switch {
		case err != nil:
			out.Warnings = append(out.Warnings, fmt.Sprintf(
				"step %s: when %q not evaluated at create time (%v); left to the executor", step.ID, step.When, err))
		case known && !verdict:
			gone[step.ID] = step.When
			out.Dropped = append(out.Dropped, step.ID)
			out.Warnings = append(out.Warnings, fmt.Sprintf(
				"step %s: not materialized, when %q is false for the given vars", step.ID, step.When))
		}
	}
	for _, step := range steps {
		if _, dropped := gone[step.ID]; dropped {
			continue
		}
		if slices.ContainsFunc(step.DependsOn, func(dep string) bool { _, ok := gone[dep]; return ok }) {
			var kept []string
			for _, dep := range step.DependsOn {
				if _, ok := gone[dep]; ok {
					out.Deps[step.ID] = append(out.Deps[step.ID], dep)
					continue
				}
				kept = append(kept, dep)
			}
			step.DependsOn = kept
		}
		out.Steps = append(out.Steps, step)
	}
	return out
}
