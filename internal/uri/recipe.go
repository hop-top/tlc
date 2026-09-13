package uri

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"hop.top/cite/scheme"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/uriutil"
)

const (
	recipeTypeName = "recipe"
	recipeRefKey   = "recipe:"
)

// recipeCompletion completes recipe names across the configured search
// path, deduplicated over versions.
func recipeCompletion(dc *TypesDirConfig) scheme.TypeRegistration {
	return scheme.TypeRegistration{
		Name: recipeTypeName,
		Completer: func(_ context.Context, prefix string) ([]string, error) {
			loc := &core.DirLocator{Dirs: dc.recipeDirs()}
			infos, err := loc.List()
			if err != nil {
				return nil, fmt.Errorf("list recipes: %w", err)
			}
			seen := map[string]bool{}
			var names []string
			for _, info := range infos {
				if seen[info.Name] || !strings.HasPrefix(info.Name, prefix) {
					continue
				}
				seen[info.Name] = true
				names = append(names, info.Name)
			}
			sort.Strings(names)
			return names, nil
		},
	}
}

// ResolveRecipe resolves a recipe reference: a file path, a name, a pinned
// name@version, tlc://recipe/<name>, or the project-scoped
// tlc://<project>/recipe:<name>. Names are searched through RecipeDirs;
// a project-scoped reference searches that project's config-dir recipes
// first.
func (r *Resolver) ResolveRecipe(ctx context.Context, input string) (*core.Recipe, error) {
	if _, err := os.Stat(input); err == nil {
		return (&core.DirLocator{}).Locate(input) //nolint:wrapcheck // locator errors already name the file
	}
	ref, dirs := input, r.RecipeDirs
	if strings.Contains(input, "://") {
		u, err := splitTaskInput(input)
		if err != nil {
			return nil, err
		}
		var projectID string
		ref, projectID = recipeRefFromURI(u)
		if projectID != "" {
			if dir := r.projectRecipesDir(ctx, projectID); dir != "" {
				dirs = append([]string{dir}, dirs...)
			}
		}
	}
	loc := &core.DirLocator{Dirs: dirs}
	return loc.Locate(ref) //nolint:wrapcheck // the not-found error names the ref and the search path
}

// recipeRefFromURI splits tlc://recipe/<name> and
// tlc://<project>/recipe:<name> into the recipe reference and the
// optional project.
func recipeRefFromURI(u *scheme.URI) (ref, projectID string) {
	if u.Namespace == recipeTypeName {
		return u.ID, ""
	}
	projectID, id := uriutil.SplitProjectTask(u.Namespace, u.ID)
	return strings.TrimPrefix(id, recipeRefKey), projectID
}

// projectRecipesDir returns <project config dir>/recipes for a registered
// project, or "" when the project is unknown or no registry is attached.
func (r *Resolver) projectRecipesDir(ctx context.Context, projectID string) string {
	if r.storage == nil {
		return ""
	}
	regProj, err := r.storage.LookupProject(ctx, projectID)
	if err != nil || regProj == nil || regProj.DBPath == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(regProj.DBPath), "recipes")
}
