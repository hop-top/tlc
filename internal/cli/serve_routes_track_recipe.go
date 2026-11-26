package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"

	"hop.top/kit/go/transport/api"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/events"
)

// trackRecipeResponse is the 201 body of a recipe create: the created
// track exactly as a plain create returns it, plus the materialization.
type trackRecipeResponse struct {
	*core.Track
	RecipeRun materializeView `json:"recipe_run"`
}

// createTrackFromRecipeRequest serves POST /tracks with a recipe. The work
// is createTrackFromRecipe, the one routine behind `track create
// --recipe`; here it only maps the request and its failures onto HTTP.
func createTrackFromRecipeRequest(w http.ResponseWriter, r *http.Request, deps *serveDeps, req *trackCreateRequest) {
	ctx := r.Context()
	if len(req.Meta) > 0 {
		writeAPIErrorf(w, http.StatusUnprocessableEntity, "validation_error", "meta cannot be combined with recipe")
		return
	}
	if req.Slug != "" {
		if err := core.ValidateNewTrackSlug(req.Slug, getConfigSlugMaxLen()); err != nil {
			writeAPIErrorf(w, http.StatusUnprocessableEntity, "validation_error", "%v", err)
			return
		}
	}
	vars, err := recipeVarsFromJSON(req.Vars)
	if err != nil {
		writeAPIErrorf(w, http.StatusUnprocessableEntity, codeInvalidVars, "%v", err)
		return
	}
	out, err := createTrackFromRecipe(ctx, deps.storage, recipeTrackRequest{
		Recipe: req.Recipe, Vars: vars, Title: req.Title, Type: req.Type, Slug: req.Slug,
	})
	if err != nil {
		if re, ok := isRequestError(err); ok {
			writeAPIErrorf(w, http.StatusUnprocessableEntity, re.Code, "%v", re.Err)
			return
		}
		writeAPIErrorf(w, http.StatusUnprocessableEntity, "validation_error", "%v", err)
		return
	}
	publishDomainEvent(ctx, deps.publisher, events.TopicTrackCreated, events.TrackCreatedPayload{
		TrackID: out.Track.ID,
		Title:   out.Track.Title,
		Type:    out.Track.Type,
	})
	api.JSON(w, http.StatusCreated, trackRecipeResponse{
		Track: out.Track,
		RecipeRun: materializeView{
			MaterializeResult: *out.Result, Recipe: out.Plan.Recipe.Name, Version: out.Plan.Recipe.Version,
			TrackID: out.Track.ID, Track: formatTrackAlias(out.Track),
		},
	})
}

// recipeVarsFromJSON turns the request's vars into the name→string form
// var binding takes: strings as they are, numbers and booleans in their
// plain spelling, lists and maps as JSON text, the way a typed var is
// written after --var.
func recipeVarsFromJSON(in map[string]any) (map[string]string, error) {
	out := make(map[string]string, len(in))
	names := make([]string, 0, len(in))
	for name := range in {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		switch v := in[name].(type) {
		case string:
			out[name] = v
		case bool:
			out[name] = strconv.FormatBool(v)
		case float64:
			out[name] = strconv.FormatFloat(v, 'f', -1, 64)
		case nil:
			return nil, fmt.Errorf("var %q is null; omit it to use the recipe default", name)
		default:
			text, err := json.Marshal(v)
			if err != nil {
				return nil, fmt.Errorf("var %q: %w", name, err)
			}
			out[name] = string(text)
		}
	}
	return out, nil
}
