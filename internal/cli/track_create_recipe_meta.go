package cli

import (
	"fmt"
	"time"

	"hop.top/tlc/internal/core"
)

// applyRecipeTrackMeta renders the recipe's track.meta with the same
// scope as track.title and merges it into the track as user meta, through
// the gate every track meta write shares. A reserved key is refused here,
// before anything is created.
func applyRecipeTrackMeta(track *core.Track, r *core.Recipe, rt *core.RecipeTrack, vars map[string]any, subj *recipeSubject) error {
	if rt == nil || len(rt.Meta) == 0 {
		return nil
	}
	scope := core.Scope{Vars: vars, Run: core.RecipeRunScope("", r.Name, r.Version, time.Now())}
	if subj != nil {
		scope.Subject = subj.Fields
	}
	meta, err := core.RenderRecipeMap(rt.Meta, scope)
	if err != nil {
		return fmt.Errorf("recipe %s: track.meta: %w", r.Name, err)
	}
	if err := core.ValidateTrackMetaWrite(meta, nil); err != nil {
		return fmt.Errorf("recipe %s: track.meta: %w", r.Name, err)
	}
	merged, err := core.MergeTrackMeta(track.Meta, meta, nil)
	if err != nil {
		return fmt.Errorf("recipe %s: track.meta: %w", r.Name, err)
	}
	track.Meta = merged
	return nil
}
