package cli

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"hop.top/kit/go/console/output"
	"hop.top/tlc/internal/core"
)

var recipeListSource string

// RecipeCmd is the parent command for recipe operations.
var RecipeCmd = &cobra.Command{
	Use:   "recipe",
	Short: "Manage recipes: reusable templates that create tracks and tasks",
	Long: `Recipes are versioned YAML templates that materialize into tracks and
tasks: an ordered list of steps with dependencies, kinds (agent, exec,
human), retries, gates and due dates, plus the variables a run binds.

Recipes are searched in recipe.dir (relative to the project root), then
the project's .tlc/recipes, then the user config dir's recipes. Reference
one by name, by name@version, or by file path.`,
}

var recipeListCmd = &cobra.Command{
	Use:   "list",
	Short: "List recipes in the search path",
	Long: `List every recipe found in the search path: recipe.dir, the project's
.tlc/recipes and the user config dir's recipes. Rows keep layer order,
then sort by name and descending version. --source keeps one layer: a
directory path, or "builtin" for the embedded set.`,
	Args: cobra.NoArgs,
	Annotations: map[string]string{
		"kit/side-effect": "read",
		"kit/idempotent":  "yes",
	},
	RunE: runRecipeList,
}

var recipeShowCmd = &cobra.Command{
	Use:   "show <recipe>",
	Short: "Show a recipe's header, vars and expanded steps",
	Long: `Show a recipe after expansion: includes are spliced in as <id>/<step>
and repeat blocks are unrolled, exactly as track create --recipe would
create them. --format json or yaml prints the expanded document.`,
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: recipeNameCompletion,
	Annotations: map[string]string{
		"kit/side-effect": "read",
		"kit/idempotent":  "yes",
	},
	RunE: runRecipeShow,
}

var recipeValidateCmd = &cobra.Command{
	Use:   "validate <recipe>",
	Short: "Validate a recipe without creating anything",
	Long: `Parse, validate and expand a recipe (by name, name@version or path).
Includes are resolved through the search path so a missing or cyclic
include is reported too. No storage is opened. Exit 0 when valid, exit 1
with the first problem otherwise.`,
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: recipeNameCompletion,
	Annotations: map[string]string{
		"kit/side-effect": "read",
		"kit/idempotent":  "yes",
	},
	RunE: runRecipeValidate,
}

// recipeRow is the table view of a listed recipe.
type recipeRow struct {
	Name        string `table:"NAME"`
	Version     string `table:"VERSION"`
	Source      string `table:"SOURCE"`
	Description string `table:"DESCRIPTION"`
}

// recipeVarRow is the table view of a declared var.
type recipeVarRow struct {
	Name        string `table:"NAME"`
	Required    string `table:"REQUIRED"`
	Default     string `table:"DEFAULT"`
	Description string `table:"DESCRIPTION"`
}

// recipeStepRow is the table view of an expanded step.
type recipeStepRow struct {
	Ordinal   int    `table:"#"`
	ID        string `table:"ID"`
	Kind      string `table:"KIND"`
	Title     string `table:"TITLE"`
	DependsOn string `table:"DEPENDS ON"`
	When      string `table:"WHEN"`
}

// recipeView is the structured (json/yaml) form of an expanded recipe.
type recipeView struct {
	Name        string                 `json:"recipe" yaml:"recipe"`
	Version     string                 `json:"version" yaml:"version"`
	Description string                 `json:"description,omitempty" yaml:"description,omitempty"`
	Requires    core.RecipeRequires    `json:"requires,omitempty" yaml:"requires,omitempty"`
	Vars        map[string]core.VarDef `json:"vars,omitempty" yaml:"vars,omitempty"`
	Agent       string                 `json:"agent,omitempty" yaml:"agent,omitempty"`
	Track       *core.RecipeTrack      `json:"track,omitempty" yaml:"track,omitempty"`
	Steps       []core.RecipeStep      `json:"steps" yaml:"steps"`
	Path        string                 `json:"path" yaml:"path"`
	Source      string                 `json:"source" yaml:"source"`
	Hash        string                 `json:"hash" yaml:"hash"`
}

func runRecipeList(cmd *cobra.Command, _ []string) error {
	loc := recipeLocator()
	infos, err := loc.List()
	if err != nil {
		return fmt.Errorf("list recipes: %w", err)
	}
	if recipeListSource != "" {
		infos = filterRecipeSource(infos, recipeListSource)
	}
	out := cmd.OutOrStdout()
	format := viper.GetString("output.format")
	switch format {
	case formatJSON, formatYAML:
		return renderRecipeOutput(out, format, normalizeEmptySlices(infos))
	}
	if len(infos) == 0 {
		where := strings.Join(loc.Dirs, ", ")
		if recipeListSource != "" {
			where = recipeListSource
		}
		_, _ = fmt.Fprintf(out, "No recipes found in %s\n", where)
		return nil
	}
	rows := make([]recipeRow, len(infos))
	for i, info := range infos {
		rows[i] = recipeRow{Name: info.Name, Version: info.Version, Source: info.Source, Description: info.Description}
	}
	return renderRecipeTable(out, format, rows)
}

func filterRecipeSource(infos []core.RecipeInfo, source string) []core.RecipeInfo {
	var kept []core.RecipeInfo
	for _, info := range infos {
		if info.Source == source {
			kept = append(kept, info)
		}
	}
	return kept
}

func runRecipeShow(cmd *cobra.Command, args []string) error {
	r, steps, err := loadExpandedRecipe(args[0])
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	format := viper.GetString("output.format")
	switch format {
	case formatJSON, formatYAML:
		return renderRecipeOutput(out, format, newRecipeView(r, steps))
	}
	printRecipeHeader(out, r)
	if len(r.Vars) > 0 {
		_, _ = fmt.Fprintln(out, "\nVars:")
		if err := renderRecipeTable(out, format, recipeVarRows(r.Vars)); err != nil {
			return err
		}
	}
	_, _ = fmt.Fprintf(out, "\nSteps (%d):\n", len(steps))
	return renderRecipeTable(out, format, recipeStepRows(steps))
}

func runRecipeValidate(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()
	r, steps, err := loadExpandedRecipe(args[0])
	if err != nil {
		_, _ = fmt.Fprintf(out, "Validation failed: %s\n", err)
		return err
	}
	_, _ = fmt.Fprintf(out, "Recipe %s is valid (%d steps)\n", r.Key(), len(steps))
	return nil
}

// loadExpandedRecipe resolves ref through the search path and expands it.
func loadExpandedRecipe(ref string) (*core.Recipe, []core.RecipeStep, error) {
	loc := recipeLocator()
	r, err := loc.Locate(ref)
	if err != nil {
		return nil, nil, err //nolint:wrapcheck // locator errors name the ref and the search path
	}
	steps, err := core.Expand(r, loc)
	if err != nil {
		return nil, nil, err //nolint:wrapcheck // expansion errors name the recipe and step
	}
	return r, steps, nil
}

func newRecipeView(r *core.Recipe, steps []core.RecipeStep) recipeView {
	return recipeView{
		Name: r.Name, Version: r.Version, Description: r.Description,
		Requires: r.Requires, Vars: r.Vars, Agent: r.Agent, Track: r.Track,
		Steps: steps, Path: r.Path, Source: r.Source, Hash: r.Hash,
	}
}

func printRecipeHeader(out io.Writer, r *core.Recipe) {
	_, _ = fmt.Fprintln(out, r.Key())
	if r.Description != "" {
		_, _ = fmt.Fprintf(out, "  %s\n", r.Description)
	}
	_, _ = fmt.Fprintf(out, "  Source:   %s\n", r.Path)
	if r.Requires.Subject != "" {
		_, _ = fmt.Fprintf(out, "  Requires: %s\n", r.Requires.Subject)
	}
	if r.Agent != "" {
		_, _ = fmt.Fprintf(out, "  Agent:    %s\n", r.Agent)
	}
	if r.Track != nil && r.Track.Title != "" {
		_, _ = fmt.Fprintf(out, "  Track:    %s\n", r.Track.Title)
	}
}

func recipeVarRows(vars map[string]core.VarDef) []recipeVarRow {
	names := make([]string, 0, len(vars))
	for name := range vars {
		names = append(names, name)
	}
	sort.Strings(names)
	rows := make([]recipeVarRow, 0, len(names))
	for _, name := range names {
		def := vars[name]
		row := recipeVarRow{Name: name, Description: def.Description}
		if def.Required {
			row.Required = "yes"
		}
		if def.Default != nil {
			row.Default = fmt.Sprint(def.Default)
		}
		rows = append(rows, row)
	}
	return rows
}

func recipeStepRows(steps []core.RecipeStep) []recipeStepRow {
	rows := make([]recipeStepRow, len(steps))
	for i, s := range steps {
		rows[i] = recipeStepRow{
			Ordinal:   s.Ordinal,
			ID:        s.ID,
			Kind:      string(s.EffectiveKind()),
			Title:     s.Title,
			DependsOn: strings.Join(s.DependsOn, ","),
			When:      s.When,
		}
	}
	return rows
}

func renderRecipeOutput(out io.Writer, format string, data any) error {
	if err := output.Render(out, format, data); err != nil {
		return fmt.Errorf("render %s: %w", format, err)
	}
	return nil
}

func renderRecipeTable[T any](out io.Writer, format string, rows []T) error {
	if err := renderStyledList(out, format, rows, nil); err != nil {
		return fmt.Errorf("render table: %w", err)
	}
	return nil
}

// recipeNameCompletion completes recipe names from the search path; the
// default directive keeps file-path completion available too.
func recipeNameCompletion(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	infos, err := recipeLocator().List()
	if err != nil {
		return nil, cobra.ShellCompDirectiveDefault
	}
	seen := map[string]bool{}
	var names []string
	for _, info := range infos {
		if seen[info.Name] || !strings.HasPrefix(info.Name, toComplete) {
			continue
		}
		seen[info.Name] = true
		names = append(names, info.Name)
	}
	sort.Strings(names)
	return names, cobra.ShellCompDirectiveDefault
}

func init() {
	recipeListCmd.Flags().StringVar(&recipeListSource, "source", "", "Only this layer: a directory path, or \"builtin\"")
	RecipeCmd.AddCommand(recipeListCmd, recipeShowCmd, recipeValidateCmd)
	RootCmd.AddCommand(RecipeCmd)
}
