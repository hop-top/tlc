package core

import (
	"io"
	"slices"
	"time"

	"hop.top/flo/spec"
)

// The recipe document model, parser, validator, expander, renderer,
// selector, var binder, condition grammar and locator live in
// hop.top/flo/spec, where a recipe is a "flow" and a gate is a "check".
// tlc keeps its own vocabulary: the names below alias or wrap the spec
// module, and recipe_compat.go maps documents and messages between the
// two vocabularies.

// Document types.
type (
	Recipe         = spec.Flow
	RecipeRequires = spec.Requires
	RecipeTrack    = spec.Track
	RecipeStep     = spec.Step
	VarDef         = spec.VarDef
	DueSpec        = spec.DueSpec
	RecipeInfo     = spec.FlowInfo
	Locator        = spec.Locator
	Scope          = spec.Scope
	Selector       = spec.Selector
	CondEnv        = spec.CondEnv
)

// Task-spec leaf types. Their JSON tags are unchanged, so Task.Spec rows
// keep their shape.
type (
	TaskKind           = spec.Kind
	ExecSpec           = spec.ExecSpec
	HumanSpec          = spec.HumanSpec
	RetrySpec          = spec.RetrySpec
	StepGate           = spec.Check
	HumanTimeoutAction = spec.HumanTimeoutAction
	// RecipeNotFoundError reports a reference no layer could resolve.
	RecipeNotFoundError = spec.FlowNotFoundError
)

// Step kinds.
const (
	TaskKindAgent = spec.KindAgent
	TaskKindExec  = spec.KindExec
	TaskKindHuman = spec.KindHuman
)

// Human timeout actions.
const (
	HumanTimeoutApprove     = spec.HumanTimeoutApprove
	HumanTimeoutReject      = spec.HumanTimeoutReject
	HumanTimeoutKeepWaiting = spec.HumanTimeoutKeepWaiting
)

// Subject kinds a recipe can require.
const (
	SubjectNone  = spec.SubjectNone
	SubjectTask  = spec.SubjectTask
	SubjectTrack = spec.SubjectTrack
)

// BuiltinSource is the Source of recipes served from the embedded set.
const BuiltinSource = spec.BuiltinSource

// ErrNeedsCEL is returned for a when/until expression that is not the
// single-comparison grammar: it needs a CEL engine tlc does not have.
var ErrNeedsCEL = spec.ErrNeedsCEL

// ImplicitDeps lists the steps a step depends on without saying so: the
// ones whose results it reads.
func ImplicitDeps(s RecipeStep) []string { return spec.ImplicitDeps(s) }

// FoldImplicitDeps returns steps with each step's depends_on extended by
// the steps whose results it reads (spec.ImplicitDeps) that are in the
// list or in known. The input is not mutated. Without it a step that
// reads results.x but omits depends_on would run in parallel with x.
func FoldImplicitDeps(steps []RecipeStep, known map[string]string) []RecipeStep {
	ids := make(map[string]bool, len(steps)+len(known))
	for _, s := range steps {
		ids[s.ID] = true
	}
	for id := range known {
		ids[id] = true
	}
	out := make([]RecipeStep, len(steps))
	for i, s := range steps {
		out[i] = s
		deps := append([]string(nil), s.DependsOn...)
		for _, id := range spec.ImplicitDeps(s) {
			if ids[id] && !slices.Contains(deps, id) {
				deps = append(deps, id)
			}
		}
		if len(deps) > 0 {
			out[i].DependsOn = deps
		}
	}
	return out
}

// RecipeRunScope is the run namespace of the template scope: run.id,
// run.flow (the recipe name), run.version and run.started_at (RFC 3339,
// UTC). Validation accepts all four, so render must bind all four.
func RecipeRunScope(runID, name, version string, started time.Time) map[string]any {
	run := map[string]any{"flow": name, "version": version, "started_at": started.UTC().Format(time.RFC3339)}
	if runID != "" {
		run["id"] = runID
	}
	return run
}

// EvalConditionEnv evaluates a when/until expression against env. A
// non-comparison expression yields ErrNeedsCEL.
func EvalConditionEnv(expr string, env CondEnv) (bool, error) {
	return spec.EvalConditionEnv(expr, env) //nolint:wrapcheck // sentinel passes through for errors.Is
}

// ParseRecipeDuration parses a Go duration extended with day (d) and
// week (w) units: "30s", "48h", "5d", "2w".
func ParseRecipeDuration(s string) (time.Duration, error) {
	d, err := spec.ParseDuration(s)
	return d, recipeWording(err)
}

// CompareRecipeVersions orders two recipe versions semver-style.
func CompareRecipeVersions(a, b string) int { return spec.CompareVersions(a, b) }

// IsRecipeNotFound reports whether err wraps a RecipeNotFoundError.
func IsRecipeNotFound(err error) bool { return spec.IsFlowNotFound(err) }

// ValidateRecipe checks a parsed recipe.
func ValidateRecipe(r *Recipe) error { return recipeWording(spec.ValidateFlow(r)) }

// Expand resolves includes and unrolls repeats into a flat step list.
func Expand(r *Recipe, loc Locator) ([]RecipeStep, error) {
	steps, err := spec.Expand(r, loc)
	return steps, recipeWording(err)
}

// Render substitutes every placeholder in s from scope.
func Render(s string, scope Scope) (string, error) {
	out, err := spec.Render(s, scope)
	return out, recipeWording(err)
}

// RenderStep renders every templatable field of one step.
func RenderStep(step RecipeStep, scope Scope) (RecipeStep, error) {
	out, err := spec.RenderStep(step, scope)
	return out, recipeWording(err)
}

// RenderSteps renders steps in dependency order so due arithmetic can
// read the steps it follows.
func RenderSteps(steps []RecipeStep, scope Scope) ([]RecipeStep, error) {
	out, err := spec.RenderSteps(steps, scope)
	return out, recipeWording(err)
}

// ParseTaskSelector parses repeated --task values.
func ParseTaskSelector(args []string) (Selector, error) {
	sel, err := spec.ParseTaskSelector(args)
	return sel, recipeWording(err)
}

// Select narrows steps to a selection.
func Select(steps []RecipeStep, sel Selector, withDeps bool) ([]RecipeStep, map[string][]string, error) {
	out, warn, err := spec.Select(steps, sel, withDeps)
	return out, warn, recipeWording(err)
}

// ParseVarFlags parses repeated --var name=value flags.
func ParseVarFlags(flags []string) (map[string]string, error) {
	m, err := spec.ParseVarFlags(flags)
	return m, recipeWording(err)
}

// ResolveRecipeVars binds provided values over the recipe's defaults.
func ResolveRecipeVars(r *Recipe, provided map[string]string) (map[string]any, error) {
	m, err := spec.ResolveVars(r, provided)
	return m, recipeWording(err)
}

// ParseRecipe decodes and validates a recipe document, accepting the
// recipe vocabulary (recipe:, gate:, {{run.recipe}}).
func ParseRecipe(r io.Reader, filename string) (*Recipe, error) {
	return parseRecipe(r, filename)
}
