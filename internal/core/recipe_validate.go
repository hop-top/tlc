package core

import (
	"fmt"
	"strings"
)

// ValidateRecipe checks everything that can be known without resolving
// includes: header, vars, each step's shape, and the dependency graph
// with include and repeat steps as opaque nodes. References into a block
// (results.sec/scan.*, steps.loop/1/fix.*) are deferred to Expand, which
// validates the flattened graph again.
func ValidateRecipe(r *Recipe) error {
	if err := validateRecipeHeader(r); err != nil {
		return err
	}
	if err := validateRecipeVars(r); err != nil {
		return err
	}
	for i := range r.Steps {
		if err := validateStepShape(&r.Steps[i], i, ""); err != nil {
			return err
		}
	}
	return validateStepGraph(r.Steps, refScope{
		vars:    r.Vars,
		subject: r.Requires.Subject,
	})
}

func validateRecipeHeader(r *Recipe) error {
	switch {
	case r.Name == "":
		return fmt.Errorf("missing required field \"recipe\" (the recipe name)")
	case !recipeNameRe.MatchString(r.Name):
		return fmt.Errorf("recipe name %q must match %s", r.Name, recipeNameRe)
	case r.Version == "":
		return fmt.Errorf("recipe %s: version is required", r.Name)
	case len(r.Steps) == 0:
		return fmt.Errorf("recipe %s: steps: at least one step is required", r.Name)
	}
	switch r.Requires.Subject {
	case "", SubjectNone, SubjectTask, SubjectTrack:
		return nil
	}
	return fmt.Errorf("recipe %s: requires.subject %q must be task, track or none", r.Name, r.Requires.Subject)
}

func validateRecipeVars(r *Recipe) error {
	for name, def := range r.Vars {
		switch {
		case reservedVarNames[name]:
			return fmt.Errorf("var %q is reserved (namespaces: subject, results, run, steps, vars)", name)
		case !varNameRe.MatchString(name):
			return fmt.Errorf("var %q must match %s", name, varNameRe)
		case def.Required && def.Default != nil:
			return fmt.Errorf("var %q: required and default contradict each other; a default makes the var optional", name)
		}
	}
	return nil
}

// validateStepShape checks one step in isolation. until is the enclosing
// repeat block's until expression, or empty outside a block.
func validateStepShape(s *RecipeStep, idx int, until string) error {
	if s.ID == "" {
		return fmt.Errorf("step #%d: id is required", idx+1)
	}
	if !stepIDRe.MatchString(s.ID) {
		return fmt.Errorf("step id %q must match %s (\"/\" is reserved for expansion)", s.ID, stepIDRe)
	}
	if until != "" && s.When != "" {
		return fmt.Errorf("step %s: cannot combine when with the enclosing block's until", s.ID)
	}
	if err := validateStepBlockShape(s); err != nil {
		return err
	}
	if s.IsBlock() {
		return validateBlockChildren(s)
	}
	return validateTaskShape(s)
}

// validateStepBlockShape enforces "exactly one of task, include, repeat"
// and that block-only keys appear on blocks.
func validateStepBlockShape(s *RecipeStep) error {
	if s.IsInclude() {
		if len(s.Steps) > 0 || s.Repeat != 0 || s.Until != "" {
			return fmt.Errorf("step %s: include cannot be combined with steps, repeat or until", s.ID)
		}
	} else if s.With != nil {
		return fmt.Errorf("step %s: with is only valid on an include step", s.ID)
	}
	if err := validateRepeatKeys(s); err != nil {
		return err
	}
	if s.IsBlock() && (s.Kind != "" || s.Exec != nil || s.Human != nil || s.Retry != nil || s.Gate != nil) {
		return fmt.Errorf("step %s: kind, exec, human, retry and gate belong on task steps, not on a block", s.ID)
	}
	return nil
}

func validateRepeatKeys(s *RecipeStep) error {
	switch {
	case s.Repeat < 0:
		return fmt.Errorf("step %s: repeat must be a positive count", s.ID)
	case s.Repeat > 0 && len(s.Steps) == 0:
		return fmt.Errorf("step %s: repeat requires nested steps", s.ID)
	case len(s.Steps) > 0 && s.Repeat == 0:
		return fmt.Errorf("step %s: nested steps require repeat: <count>", s.ID)
	case s.Until != "" && s.Repeat == 0:
		return fmt.Errorf("step %s: until is only valid with repeat", s.ID)
	}
	return nil
}

func validateBlockChildren(s *RecipeStep) error {
	if s.Until != "" {
		if _, err := parseCondition(s.Until); err != nil {
			return fmt.Errorf("step %s: until: %w", s.ID, err)
		}
	}
	for i := range s.Steps {
		if err := validateStepShape(&s.Steps[i], i, s.Until); err != nil {
			return fmt.Errorf("step %s: %w", s.ID, err)
		}
	}
	return nil
}

func validateTaskShape(s *RecipeStep) error {
	if !s.Kind.Valid() {
		return fmt.Errorf("step %s: unknown kind %q (want agent, exec or human)", s.ID, s.Kind)
	}
	kind := s.EffectiveKind()
	if err := validateExecShape(s, kind); err != nil {
		return err
	}
	if err := validateHumanShape(s, kind); err != nil {
		return err
	}
	if s.Retry != nil {
		if s.Retry.MaxAttempts < 0 {
			return fmt.Errorf("step %s: retry.max_attempts must not be negative", s.ID)
		}
		if err := validateDurationField(s.Retry.Backoff, s.ID, "retry.backoff"); err != nil {
			return err
		}
	}
	if s.Gate != nil && s.Gate.Contract == "" {
		return fmt.Errorf("step %s: gate.contract is required", s.ID)
	}
	if s.Due.After != "" && s.Due.After == s.ID {
		return fmt.Errorf("step %s: due.after references itself", s.ID)
	}
	return validateDurationField(strings.TrimLeft(s.Due.Offset, "+-"), s.ID, "due.offset")
}

func validateExecShape(s *RecipeStep, kind TaskKind) error {
	if kind != TaskKindExec {
		if s.Exec != nil {
			return fmt.Errorf("step %s: exec block requires kind: exec", s.ID)
		}
		return nil
	}
	if s.Exec == nil || len(s.Exec.Argv) == 0 {
		return fmt.Errorf("step %s: exec step requires exec.argv", s.ID)
	}
	return validateDurationField(s.Exec.Timeout, s.ID, "exec.timeout")
}

func validateHumanShape(s *RecipeStep, kind TaskKind) error {
	if s.Human == nil {
		return nil
	}
	if kind != TaskKindHuman {
		return fmt.Errorf("step %s: human block requires kind: human", s.ID)
	}
	switch HumanTimeoutAction(s.Human.OnTimeout) {
	case "", HumanTimeoutApprove, HumanTimeoutReject:
	default:
		return fmt.Errorf("step %s: human.on_timeout %q must be approve or reject", s.ID, s.Human.OnTimeout)
	}
	return validateDurationField(s.Human.Timeout, s.ID, "human.timeout")
}

func validateDurationField(v, stepID, field string) error {
	if v == "" {
		return nil
	}
	if _, err := ParseRecipeDuration(v); err != nil {
		return fmt.Errorf("step %s: %s: %w", stepID, field, err)
	}
	return nil
}

// refScope is what a step list's references may resolve against.
type refScope struct {
	vars    map[string]VarDef
	subject string
	// outer holds ids visible from an enclosing list; a reference to one
	// is accepted here and checked for upstream-ness after expansion.
	outer map[string]bool
	// inRepeat allows run.iteration.
	inRepeat bool
}

// stepGraph is the dependency view of one step list.
type stepGraph struct {
	ids      map[string]bool
	blocks   map[string]bool
	upstream map[string]map[string]bool
}

// validateStepGraph checks ids, dependencies, cycles and every reference
// of a step list, recursing into repeat blocks.
func validateStepGraph(steps []RecipeStep, sc refScope) error {
	g, err := buildStepGraph(steps)
	if err != nil {
		return err
	}
	for i := range steps {
		s := &steps[i]
		if err := validateStepRefs(s, g, sc); err != nil {
			return err
		}
		if s.IsRepeat() {
			if err := validateRepeatGraph(s, g, sc); err != nil {
				return err
			}
		}
	}
	return nil
}

func buildStepGraph(steps []RecipeStep) (*stepGraph, error) {
	g := &stepGraph{ids: map[string]bool{}, blocks: map[string]bool{}, upstream: map[string]map[string]bool{}}
	for i := range steps {
		if g.ids[steps[i].ID] {
			return nil, fmt.Errorf("duplicate step id %q", steps[i].ID)
		}
		g.ids[steps[i].ID] = true
		if steps[i].IsBlock() {
			g.blocks[steps[i].ID] = true
		}
	}
	for i := range steps {
		for _, dep := range steps[i].DependsOn {
			if !g.ids[dep] {
				return nil, fmt.Errorf("step %s: depends_on references unknown step %q", steps[i].ID, dep)
			}
		}
	}
	if err := detectStepCycles(steps); err != nil {
		return nil, err
	}
	byID := indexSteps(steps)
	for i := range steps {
		g.upstream[steps[i].ID] = upstreamClosure(steps[i].ID, byID)
	}
	return g, nil
}

func indexSteps(steps []RecipeStep) map[string]*RecipeStep {
	byID := make(map[string]*RecipeStep, len(steps))
	for i := range steps {
		byID[steps[i].ID] = &steps[i]
	}
	return byID
}

// upstreamClosure returns every step reachable through depends_on.
func upstreamClosure(id string, byID map[string]*RecipeStep) map[string]bool {
	seen := map[string]bool{}
	var walk func(string)
	walk = func(cur string) {
		for _, dep := range byID[cur].DependsOn {
			if !seen[dep] {
				seen[dep] = true
				walk(dep)
			}
		}
	}
	walk(id)
	return seen
}

// detectStepCycles reports a depends_on cycle; ids are known to exist.
func detectStepCycles(steps []RecipeStep) error {
	byID := indexSteps(steps)
	const (
		visiting = 1
		done     = 2
	)
	state := map[string]int{}
	var visit func(string) error
	visit = func(id string) error {
		state[id] = visiting
		for _, dep := range byID[id].DependsOn {
			switch state[dep] {
			case visiting:
				return fmt.Errorf("depends_on cycle through step %q", dep)
			case done:
				continue
			}
			if err := visit(dep); err != nil {
				return err
			}
		}
		state[id] = done
		return nil
	}
	for i := range steps {
		if state[steps[i].ID] == 0 {
			if err := visit(steps[i].ID); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateRepeatGraph(s *RecipeStep, g *stepGraph, sc refScope) error {
	inner := refScope{vars: sc.vars, subject: sc.subject, inRepeat: true, outer: map[string]bool{}}
	for id := range sc.outer {
		inner.outer[id] = true
	}
	for id := range g.ids {
		inner.outer[id] = true
	}
	if err := validateStepGraph(s.Steps, inner); err != nil {
		return fmt.Errorf("step %s: %w", s.ID, err)
	}
	if s.Until == "" {
		return nil
	}
	c, err := parseCondition(s.Until)
	if err != nil {
		return fmt.Errorf("step %s: until: %w", s.ID, err)
	}
	if target, _, ok := c.resultsRef(); ok && !hasStepID(s.Steps, target) {
		return fmt.Errorf("step %s: until references results.%s, which is not a step of the block", s.ID, target)
	}
	return nil
}

func hasStepID(steps []RecipeStep, id string) bool {
	for i := range steps {
		if steps[i].ID == id {
			return true
		}
	}
	return false
}

// validateStepRefs checks a step's when, due.after and every template
// reference in its fields.
func validateStepRefs(s *RecipeStep, g *stepGraph, sc refScope) error {
	if s.When != "" {
		c, err := parseCondition(s.When)
		if err != nil {
			return fmt.Errorf("step %s: when: %w", s.ID, err)
		}
		if target, _, ok := c.resultsRef(); ok {
			if err := checkResultsTarget(s, target, g, sc); err != nil {
				return fmt.Errorf("step %s: when: %w", s.ID, err)
			}
		}
	}
	if s.Due.After != "" {
		if err := checkStepTarget(s, s.Due.After, g, sc); err != nil {
			return fmt.Errorf("step %s: due.after: %w", s.ID, err)
		}
	}
	for _, ref := range stepRefs(*s) {
		if err := checkTemplateRef(s, ref, g, sc); err != nil {
			return fmt.Errorf("step %s: {{%s}}: %w", s.ID, ref, err)
		}
	}
	return nil
}

func checkTemplateRef(s *RecipeStep, ref string, g *stepGraph, sc refScope) error {
	ns, rest, dotted := strings.Cut(ref, ".")
	if !dotted {
		return checkVarRef(ref, sc)
	}
	switch ns {
	case nsVars:
		return checkVarRef(rest, sc)
	case nsSubject:
		return checkSubjectRef(rest, sc)
	case nsRun:
		if !runFields[rest] || (rest == fieldIteration && !sc.inRepeat) {
			return fmt.Errorf("unknown run field %q (run.iteration is only bound inside a repeat block)", rest)
		}
		return nil
	case nsResults:
		target, path, ok := strings.Cut(rest, ".")
		if !ok || path == "" {
			return fmt.Errorf("want results.<step>.<field>")
		}
		return checkResultsTarget(s, target, g, sc)
	case nsSteps:
		return checkStepsRef(s, rest, g, sc)
	}
	return fmt.Errorf("unknown namespace %q", ns)
}

func checkVarRef(name string, sc refScope) error {
	if _, ok := sc.vars[name]; !ok {
		return fmt.Errorf("references unknown var %q; declare it under vars", name)
	}
	return nil
}

func checkSubjectRef(field string, sc refScope) error {
	if !subjectFields[field] {
		return fmt.Errorf("unknown subject field %q", field)
	}
	if sc.subject != SubjectTask && sc.subject != SubjectTrack {
		return fmt.Errorf("references subject.%s but the recipe has no requires.subject", field)
	}
	return nil
}

func checkStepsRef(s *RecipeStep, rest string, g *stepGraph, sc refScope) error {
	id, field, ok := strings.Cut(rest, ".")
	if !ok || field == "" {
		return fmt.Errorf("want steps.<id>.<field>")
	}
	if id == s.ID {
		return fmt.Errorf("references itself")
	}
	if !stepFields[field] {
		return fmt.Errorf("unknown step field %q", field)
	}
	return checkStepTarget(s, id, g, sc)
}

// checkResultsTarget requires a results.<target> reference to name a step
// upstream of s, or a reference deferred to expansion.
func checkResultsTarget(s *RecipeStep, target string, g *stepGraph, sc refScope) error {
	if g.ids[target] {
		if target == s.ID {
			return fmt.Errorf("references its own results")
		}
		if !g.upstream[s.ID][target] {
			return fmt.Errorf("references results.%s but %s is not in depends_on (directly or transitively)", target, target)
		}
		return nil
	}
	if deferredRef(target, g, sc) {
		return nil
	}
	return fmt.Errorf("references unknown step %q", target)
}

// checkStepTarget requires a step id to exist here or be deferred.
func checkStepTarget(s *RecipeStep, target string, g *stepGraph, sc refScope) error {
	if target == s.ID {
		return fmt.Errorf("references itself")
	}
	if g.ids[target] || deferredRef(target, g, sc) {
		return nil
	}
	return fmt.Errorf("references unknown step %q", target)
}

// deferredRef reports whether a reference can only be checked after
// expansion: it points into a block (sec/scan) or at an enclosing list.
func deferredRef(target string, g *stepGraph, sc refScope) bool {
	if sc.outer[target] {
		return true
	}
	head, _, nested := strings.Cut(target, "/")
	return nested && (g.blocks[head] || sc.outer[head])
}
