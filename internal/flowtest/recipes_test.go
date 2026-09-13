//go:build !shimbin

package flowtest_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/flowtest"
)

// recipesDir is the canonical location of the example recipes `tlc recipe
// test` exercises.
const recipesDir = "../../examples/recipes"

// allRecipes returns every recipe file directly under recipesDir.
func allRecipes(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(recipesDir)
	require.NoError(t, err, "read recipes dir")

	var recipes []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		recipes = append(recipes, filepath.Join(recipesDir, e.Name()))
	}
	require.NotEmpty(t, recipes, "no recipe files found in %s", recipesDir)
	return recipes
}

// TestAllRecipesParse verifies every example recipe parses and expands.
func TestAllRecipesParse(t *testing.T) {
	t.Parallel()
	for _, path := range allRecipes(t) {
		name := strings.TrimSuffix(filepath.Base(path), ".yaml")
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f, err := os.Open(path)
			require.NoError(t, err)
			defer f.Close()

			r, err := core.ParseRecipe(f, path)
			require.NoError(t, err, "ParseRecipe failed")
			assert.Equal(t, name, r.Name, "the file name must match the recipe name so fixtures resolve")

			steps, err := core.Expand(r, &core.DirLocator{Dirs: []string{recipesDir}})
			require.NoError(t, err, "Expand failed")
			assert.NotEmpty(t, steps)
		})
	}
}

// TestAllRecipesHaveHappyPathFixture verifies each recipe has a happy-path
// run scaffold next to it.
func TestAllRecipesHaveHappyPathFixture(t *testing.T) {
	t.Parallel()
	for _, path := range allRecipes(t) {
		name := strings.TrimSuffix(filepath.Base(path), ".yaml")
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			runs, err := flowtest.DiscoverRuns(name, filepath.Dir(path), flowtest.WithRunName("happy-path"))
			require.NoError(t, err, "DiscoverRuns should find happy-path for %s", name)
			require.Len(t, runs, 1)
			assert.Equal(t, "happy-path", runs[0].Name)
			assert.Equal(t, 0, runs[0].ExpectedExit)
			assert.DirExists(t, runs[0].RecordDir)
			assert.DirExists(t, runs[0].ContractsDir)
		})
	}
}
