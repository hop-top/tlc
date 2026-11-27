package core

import "fmt"

// prepareStepExtras reads and checks the tlc-owned fields of a rendered
// step before anything is written, so a bad one leaves no partial run:
// the reserved meta keys, the tag policy and the priority vocabulary,
// the same checks task create and plan ingest apply to the same fields.
func prepareStepExtras(recipe string, s RecipeStep) (StepExtras, error) {
	x, err := checkStepExtras(s)
	if err != nil {
		return x, fmt.Errorf("recipe %s: %w", recipe, err)
	}
	if err := ValidateTags(x.Tags); err != nil {
		return x, fmt.Errorf("recipe %s step %s: %w", recipe, s.ID, err)
	}
	if !ValidPriority(Priority(x.Priority)) {
		return x, fmt.Errorf(
			"recipe %s step %s: invalid priority %q; allowed: %s", recipe, s.ID, x.Priority, PriorityVocabularyList(),
		)
	}
	return x, nil
}
