package uri

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"hop.top/tlc/internal/storage"
)

func writeRecipeFile(t *testing.T, dir, file, body string) string {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	p := filepath.Join(dir, file)
	require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
	return p
}

func TestRegisterTypes_RecipeCompleter(t *testing.T) {
	dir := t.TempDir()
	s, err := storage.NewSQLiteStorage(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	defer s.Close()

	recipes := filepath.Join(dir, "recipes")
	writeRecipeFile(t, recipes, "review.yaml", "recipe: code-review\nversion: 1.0.0\nsteps:\n  - id: a\n")
	writeRecipeFile(t, recipes, "review-2.yaml", "recipe: code-review\nversion: 1.1.0\nsteps:\n  - id: a\n")
	writeRecipeFile(t, recipes, "release.yaml", "recipe: release\nversion: 0.1.0\nsteps:\n  - id: a\n")

	reg, err := GetRegistry(s, &TypesDirConfig{RecipeDirs: []string{recipes}})
	require.NoError(t, err)
	assert.Contains(t, reg.Types(), "recipe")

	// Names are deduplicated across versions and filtered by prefix.
	got, err := reg.Complete(context.Background(), "recipe", "")
	require.NoError(t, err)
	assert.Equal(t, []string{"code-review", "release"}, got)

	got, err = reg.Complete(context.Background(), "recipe", "rel")
	require.NoError(t, err)
	assert.Equal(t, []string{"release"}, got)

	// A missing layer is not an error.
	reg, err = GetRegistry(s, &TypesDirConfig{RecipeDirs: []string{filepath.Join(dir, "nope")}})
	require.NoError(t, err)
	got, err = reg.Complete(context.Background(), "recipe", "")
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestResolver_ResolveRecipe(t *testing.T) {
	dir := t.TempDir()
	s, err := storage.NewSQLiteStorage(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	defer s.Close()

	recipes := filepath.Join(dir, "recipes")
	writeRecipeFile(t, recipes, "review.yaml", "recipe: code-review\nversion: 1.0.0\nsteps:\n  - id: a\n")
	writeRecipeFile(t, recipes, "review-2.yaml", "recipe: code-review\nversion: 1.1.0\nsteps:\n  - id: a\n")
	adhoc := writeRecipeFile(t, dir, "adhoc.yaml", "recipe: adhoc\nversion: 0.1.0\nsteps:\n  - id: a\n")

	r := NewResolver(s)
	r.RecipeDirs = []string{recipes}
	ctx := context.Background()

	got, err := r.ResolveRecipe(ctx, "code-review")
	require.NoError(t, err)
	assert.Equal(t, "1.1.0", got.Version)
	assert.Equal(t, recipes, got.Source)

	got, err = r.ResolveRecipe(ctx, "code-review@1.0.0")
	require.NoError(t, err)
	assert.Equal(t, "1.0.0", got.Version)

	// The tlc://recipe/<name> URI form and a bare file path both work.
	got, err = r.ResolveRecipe(ctx, "tlc://recipe/code-review")
	require.NoError(t, err)
	assert.Equal(t, "code-review", got.Name)

	got, err = r.ResolveRecipe(ctx, adhoc)
	require.NoError(t, err)
	assert.Equal(t, "adhoc", got.Name)
	assert.Equal(t, adhoc, got.Path)

	_, err = r.ResolveRecipe(ctx, "nope")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nope")
	assert.Contains(t, err.Error(), recipes)
}
