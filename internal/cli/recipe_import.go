package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	kitcli "hop.top/kit/go/console/cli"
	"hop.top/tlc/internal/core"
)

var (
	recipeImportVars     []string
	recipeImportName     string
	recipeImportVersion  string
	recipeImportOutput   string
	recipeImportInstall  bool
	recipeImportForce    bool
	recipeImportExternal bool
)

const recipeImportStdout = "-"

var recipeImportCmd = &cobra.Command{
	Use:   "import <track|url>",
	Short: "Write a recipe captured from a track or converted from a markdown procedure",
	Long: `Capture an existing track as a recipe, or convert a markdown procedure
reachable by URL (a GitHub blob or raw link) into one.

A track becomes one step per task in dependency order: ids come from
recipe provenance where the tasks have it, otherwise from the titles;
depends_on follows the blocked-by edges inside the track. --var name=value
lifts every whole-word occurrence of value back into {{name}} and declares
the var; the vars of the track's latest recipe run are lifted the same way.
Edges to tasks outside the track are dropped unless --external-deps pulls
those tasks in as steps.

A URL is fetched with the relative markdown files it links, rewritten in
the recipe format by the model behind LLM_API_KEY (or OPENROUTER_API_KEY,
OPENAI_API_KEY, ANTHROPIC_API_KEY; LLM_API_URL and LLM_MODEL tune the
endpoint), validated, and kept verbatim as track.plan.

The document is written to <name>.yaml in the working directory, to the
project's recipes directory with --install, or to stdout with -o -. An
existing file is never overwritten without --force.`,
	Args: cobra.ExactArgs(1),
	Annotations: map[string]string{
		"kit/side-effect": "write-local",
		"kit/idempotent":  "no",
	},
	RunE: runRecipeImport,
}

func runRecipeImport(cmd *cobra.Command, args []string) error {
	rec, warnings, err := importRecipe(cmd.Context(), args[0])
	if err != nil {
		return err
	}
	data, err := core.MarshalRecipe(rec)
	if err != nil {
		return err //nolint:wrapcheck // names the recipe already
	}
	errOut := cmd.ErrOrStderr()
	for _, w := range warnings {
		_, _ = fmt.Fprintf(errOut, "warning: %s\n", w)
	}
	dest, err := recipeImportDest(rec.Name)
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	if dest == recipeImportStdout {
		_, err = out.Write(data)
		return err //nolint:wrapcheck // plain write to stdout
	}
	if kitcli.IsDryRun(cmd) {
		if _, err := out.Write(data); err != nil {
			return fmt.Errorf("write preview: %w", err)
		}
		_, _ = fmt.Fprintf(errOut, "dry-run: would write %s\n", dest)
		return nil
	}
	return writeRecipeFile(out, dest, data, rec)
}

// importRecipe dispatches on the argument: an http(s) URL goes through
// the model-backed importer, anything else resolves as a track.
func importRecipe(ctx context.Context, ref string) (*core.Recipe, []string, error) {
	vars, err := core.ParseVarFlags(recipeImportVars)
	if err != nil {
		return nil, nil, err //nolint:wrapcheck // names the flag value
	}
	if !isRecipeSourceURL(ref) {
		return captureRecipeFromTrack(ctx, ref, vars)
	}
	if len(vars) > 0 {
		return nil, nil, errors.New("--var lifts values out of a captured track; a URL import takes no vars")
	}
	rec, err := importRecipeFromURL(ctx, ref)
	if err != nil {
		return nil, nil, err
	}
	if err := applyRecipeHeaderOverrides(rec); err != nil {
		return nil, nil, err
	}
	return rec, nil, nil
}

func isRecipeSourceURL(ref string) bool {
	return strings.HasPrefix(ref, "https://") || strings.HasPrefix(ref, "http://")
}

func importRecipeFromURL(ctx context.Context, url string) (*core.Recipe, error) {
	llm, err := core.LLMClientFromEnv()
	if err != nil {
		return nil, fmt.Errorf("recipe import %s: %w", url, err)
	}
	rec, err := core.NewRecipeImporter(llm).ImportFromURL(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("recipe import: %w", err)
	}
	return rec, nil
}

// applyRecipeHeaderOverrides applies --name and --version to an imported
// recipe and re-validates the header.
func applyRecipeHeaderOverrides(rec *core.Recipe) error {
	if recipeImportName != "" {
		rec.Name = recipeImportName
	}
	if recipeImportVersion != "" {
		rec.Version = recipeImportVersion
	}
	if err := core.ValidateRecipe(rec); err != nil {
		return fmt.Errorf("recipe import: %w", err)
	}
	return nil
}

func captureRecipeFromTrack(ctx context.Context, ref string, vars map[string]string) (*core.Recipe, []string, error) {
	s, err := getStorageRaw()
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = s.Close() }()
	id, err := resolveTrackID(ctx, s, ref)
	if err != nil {
		return nil, nil, err
	}
	track, err := s.GetTrack(ctx, id)
	if err != nil {
		return nil, nil, fmt.Errorf("get track %s: %w", id, err)
	}
	if track == nil {
		return nil, nil, newTrackNotFoundError("track %q not found; run 'tlc track list' to see available tracks", ref)
	}
	opts := core.CaptureOpts{
		Name: recipeImportName, Version: recipeImportVersion, Vars: vars,
		Plan: readTrackPlan(track), IncludeExternalDeps: recipeImportExternal,
	}
	rec, warnings, err := core.CaptureTrack(ctx, s, s, track, opts)
	if err != nil {
		return nil, nil, err //nolint:wrapcheck // capture errors name the track and the step
	}
	return rec, warnings, nil
}

// readTrackPlan returns the body of the track's plan.md with its
// frontmatter stripped, or "" when the scaffold does not exist.
func readTrackPlan(track *core.Track) string {
	cfgDir := projectConfigDir()
	if cfgDir == "" {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(cfgDir, tracksDir(), trackScaffoldDirName(track), "plan.md"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(stripFrontmatter(string(data)))
}

// stripFrontmatter drops a leading "---" block; a document without one
// is returned as is.
func stripFrontmatter(s string) string {
	const fence = "---\n"
	rest, ok := strings.CutPrefix(s, fence)
	if !ok {
		return s
	}
	if strings.HasPrefix(rest, fence) {
		return rest[len(fence):]
	}
	i := strings.Index(rest, "\n"+fence)
	if i < 0 {
		return s
	}
	return rest[i+1+len(fence):]
}

// recipeImportDest picks the output path: -o wins, --install targets the
// project's recipes directory, the default is <name>.yaml in the
// working directory.
func recipeImportDest(name string) (string, error) {
	switch {
	case recipeImportOutput != "" && recipeImportInstall:
		return "", errors.New("--output and --install both name a destination; pass one of them")
	case recipeImportOutput != "":
		return recipeImportOutput, nil
	case recipeImportInstall:
		cfgDir := projectConfigDir()
		if cfgDir == "" {
			return "", errors.New("--install needs a project config dir; run 'tlc init' first or pass -o <path>")
		}
		return filepath.Join(cfgDir, recipesDirName, name+".yaml"), nil
	}
	return name + ".yaml", nil
}

func writeRecipeFile(out io.Writer, dest string, data []byte, rec *core.Recipe) error {
	if _, err := os.Stat(dest); err == nil && !recipeImportForce {
		return fmt.Errorf("%s already exists; pass --force to overwrite it or -o <path> to write elsewhere", dest)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o750); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(dest), err)
	}
	if err := os.WriteFile(dest, data, 0o644); err != nil { //nolint:gosec // a recipe is a shared project file
		return fmt.Errorf("write %s: %w", dest, err)
	}
	_, _ = fmt.Fprintf(out, "Wrote recipe %s (%d steps) to %s\n", rec.Key(), len(rec.Steps), dest)
	return nil
}

func init() {
	f := recipeImportCmd.Flags()
	f.StringArrayVar(&recipeImportVars, "var", nil, "Lift value out of the captured track as {{name}} (name=value, repeatable)")
	f.StringVar(&recipeImportName, "name", "", "Recipe name (default: the track slug or the document title)")
	f.StringVar(&recipeImportVersion, "version", "", "Recipe version (default: 0.1.0)")
	f.StringVarP(&recipeImportOutput, "output", "o", "", "Output path; - writes to stdout (default: <name>.yaml)")
	f.BoolVar(&recipeImportInstall, "install", false, "Write to the project's recipes directory")
	f.BoolVar(&recipeImportForce, "force", false, "Overwrite an existing file")
	f.BoolVar(&recipeImportExternal, "external-deps", false, "Capture tasks outside the track that its tasks depend on")
}
