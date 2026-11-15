package recipes_test

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/flowtest"
	"hop.top/tlc/internal/recipes"
)

// builtinNames returns the recipe names the embedded set ships, one per
// file, named after the file.
func builtinNames(t *testing.T) []string {
	t.Helper()
	entries, err := fs.ReadDir(recipes.FS(), ".")
	require.NoError(t, err)
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".yaml") {
			names = append(names, strings.TrimSuffix(e.Name(), ".yaml"))
		}
	}
	require.NotEmpty(t, names, "the embedded set is empty")
	return names
}

func TestBuiltin_EveryRecipeResolvesAndExpands(t *testing.T) {
	t.Parallel()
	loc := &core.DirLocator{Builtin: recipes.FS()}
	for _, name := range builtinNames(t) {
		r, err := loc.Locate(name)
		require.NoError(t, err, "the file name must match the recipe name: %s", name)
		assert.Equal(t, core.BuiltinSource, r.Source)
		steps, err := core.Expand(r, loc)
		require.NoError(t, err, "expand %s", name)
		assert.NotEmpty(t, steps)
	}
}

// Fixtures sit beside the recipe source, so `tlc recipe test` on the
// source file finds them; they are not part of the embedded set.
func TestBuiltin_EveryRecipeHasHappyPathFixture(t *testing.T) {
	t.Parallel()
	for _, name := range builtinNames(t) {
		runs, err := flowtest.DiscoverRuns(name, ".", flowtest.WithRunName("happy-path"))
		require.NoError(t, err, name)
		require.Len(t, runs, 1, name)
		assert.DirExists(t, runs[0].RecordDir)
		assert.DirExists(t, runs[0].ContractsDir)
	}
	_, err := fs.Stat(recipes.FS(), "fixtures")
	assert.ErrorIs(t, err, fs.ErrNotExist, "fixtures must not be compiled into the binary")
}

func TestTrackConsolidate_Shape(t *testing.T) {
	t.Parallel()
	loc := &core.DirLocator{Builtin: recipes.FS()}
	r, err := loc.Locate("track-consolidate")
	require.NoError(t, err)

	assert.True(t, r.Vars["scope"].Required, "scope is required")
	for name, want := range map[string]string{"keep": "", "exclude": "", "code_root": ".", "workdir": "track-consolidate"} {
		def, ok := r.Vars[name]
		require.True(t, ok, "var %s declared", name)
		assert.False(t, def.Required, "var %s optional", name)
		assert.Equal(t, want, def.Default, "var %s default", name)
	}

	steps, err := core.Expand(r, loc)
	require.NoError(t, err)
	ids := make([]string, len(steps))
	byID := map[string]*core.RecipeStep{}
	for i, s := range steps {
		ids[i] = s.ID
		byID[s.ID] = &steps[i]
	}
	assert.Equal(t, []string{
		"inventory", "verify-tasks", "propose-regroup", "generate-script",
		"approve", "apply", "verify",
	}, ids)

	// Everything before the gate is agent work that only reads tlc.
	for _, id := range []string{"inventory", "verify-tasks", "propose-regroup", "generate-script"} {
		assert.Equal(t, core.TaskKindAgent, byID[id].EffectiveKind(), "%s is agent work", id)
	}
	// The gate is a human decision, and the only write path sits behind it.
	assert.Equal(t, core.TaskKindHuman, byID["approve"].EffectiveKind())
	assert.Equal(t, []string{"generate-script"}, byID["approve"].DependsOn)
	assert.Equal(t, core.TaskKindExec, byID["apply"].EffectiveKind())
	assert.Equal(t, []string{"approve"}, byID["apply"].DependsOn)
	assert.Equal(t, []string{"apply"}, byID["verify"].DependsOn)

	// The apply script encodes the known tlc gotchas.
	gen := byID["generate-script"].Description
	for _, want := range []string{
		"pending goes to active", "cannot be activated", "abandon it straight from pending",
		"recorded only on a status transition", "--track", "--remove-blocked-by", "--add-blocked-by",
		"no `set -e`", "apply-failures.log",
	} {
		assert.Contains(t, gen, want)
	}
	assert.Contains(t, byID["verify"].Description, "JSON output omits edges")
	assert.Contains(t, byID["verify-tasks"].Description, "file:line")
	assert.Contains(t, byID["verify-tasks"].Description, "commit SHA")
}

func TestDecisionRecord_Shape(t *testing.T) {
	t.Parallel()
	loc := &core.DirLocator{Builtin: recipes.FS()}
	r, err := loc.Locate("decision-record")
	require.NoError(t, err)

	assert.True(t, r.Vars["question"].Required, "question is required")
	for name, want := range map[string]string{
		"owner": "", "affected": "", "source_of_truth": ".",
		"record_dir": "docs/decisions", "workdir": "decision-record",
	} {
		def, ok := r.Vars[name]
		require.True(t, ok, "var %s declared", name)
		assert.False(t, def.Required, "var %s optional", name)
		assert.Equal(t, want, def.Default, "var %s default", name)
	}

	steps, err := core.Expand(r, loc)
	require.NoError(t, err)
	ids := make([]string, len(steps))
	byID := map[string]*core.RecipeStep{}
	for i, s := range steps {
		ids[i] = s.ID
		byID[s.ID] = &steps[i]
	}
	assert.Equal(t, []string{
		"frame", "options", "recommend", "generate-script",
		"decide", "apply", "verify",
	}, ids)

	// Everything before the gate is agent work that only reads.
	for _, id := range []string{"frame", "options", "recommend", "generate-script"} {
		assert.Equal(t, core.TaskKindAgent, byID[id].EffectiveKind(), "%s is agent work", id)
	}
	// The owner decides, and the only write path sits behind the decision.
	decide := byID["decide"]
	assert.Equal(t, core.TaskKindHuman, decide.EffectiveKind())
	assert.Equal(t, []string{"generate-script"}, decide.DependsOn)
	assert.Equal(t, "{{owner}}", decide.Assignee, "the owner is the step assignee, the field materialize applies")
	assert.Equal(t, core.TaskKindExec, byID["apply"].EffectiveKind())
	assert.Equal(t, []string{"decide"}, byID["apply"].DependsOn)
	assert.Equal(t, []string{"apply"}, byID["verify"].DependsOn)

	// At least two options, one of them the status quo.
	assert.Contains(t, byID["options"].Description, "at least two, one of them leaving things as they are")
	// Links run one way: the record names no task ids, tasks carry a tag.
	assert.Contains(t, byID["recommend"].Description, "The record names no task ids")
	gen := byID["generate-script"].Description
	for _, want := range []string{
		"--add-tag decision:<NNNN>-<slug>", "Compute it when the script runs",
		"Superseded by", "no `set -e`", "apply-failures.log",
	} {
		assert.Contains(t, gen, want)
	}
	assert.Contains(t, byID["frame"].Description, "the JSON output omits them")
	assert.Contains(t, byID["verify"].Description, "--tag decision:<NNNN>-<slug>")
}
