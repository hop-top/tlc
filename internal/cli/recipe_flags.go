package cli

import "github.com/spf13/cobra"

// Recipe flag state. One set of package vars backs every verb that
// registers the flags: the create verbs take the whole set, the execute
// verbs only --recipe and --var (plus --recreate on track execute).
var (
	recipeFlagRecipe   string
	recipeFlagVars     []string
	recipeFlagTasks    []string
	recipeFlagWithDeps bool
	recipeFlagFor      string
	recipeFlagAssign   bool
	recipeFlagInfer    bool
	recipeFlagRecreate bool
)

// recipeCreateOpts is the recipe surface of one invocation as read back
// from the flags.
type recipeCreateOpts struct {
	Recipe   string
	Vars     []string
	Tasks    []string
	WithDeps bool
	For      string
	Assign   bool
	Infer    bool
	Recreate bool
}

func recipeOptsFromFlags() recipeCreateOpts {
	return recipeCreateOpts{
		Recipe:   recipeFlagRecipe,
		Vars:     recipeFlagVars,
		Tasks:    recipeFlagTasks,
		WithDeps: recipeFlagWithDeps,
		For:      recipeFlagFor,
		Assign:   recipeFlagAssign,
		Infer:    recipeFlagInfer,
		Recreate: recipeFlagRecreate,
	}
}

// registerRecipeFlags adds the recipe flags a create verb accepts.
func registerRecipeFlags(cmd *cobra.Command) {
	registerRecipeRefFlags(cmd)
	f := cmd.Flags()
	f.StringArrayVar(&recipeFlagTasks, "task", nil,
		"Materialize only these steps: ordinals (3), ranges (1-5) or ids (lint); repeatable, comma-separated")
	f.BoolVar(&recipeFlagWithDeps, "with-deps", false, "With --task: pull in the dependencies of the selected steps")
	f.StringVar(&recipeFlagFor, "for", "",
		"Subject the recipe applies to: a task (T-NNNN) or a track (L-NNNN, slug); bound as subject.*")
	f.BoolVar(&recipeFlagAssign, "assign", false, "Assign steps that name no assignee through the assignment engine")
	f.BoolVar(&recipeFlagInfer, "infer", false, "Infer missing vars from the subject (not implemented yet)")
}

// registerRecipeRefFlags adds --recipe and --var, the subset the execute
// verbs accept.
func registerRecipeRefFlags(cmd *cobra.Command) {
	f := cmd.Flags()
	f.StringVar(&recipeFlagRecipe, "recipe", "", "Recipe to materialize: a name, name@version or file path")
	f.StringArrayVar(&recipeFlagVars, "var", nil, "Recipe var as key=value[,key=value]; repeatable")
}
