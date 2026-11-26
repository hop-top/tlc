package cli

import (
	"context"
	"errors"
	"fmt"
	"io"

	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/storage"
)

// recipeTrackRequest is a track-from-recipe request that did not come
// through the CLI flags: the REST create. It runs the same steps as
// `track create --recipe` — resolve, bind vars, build the track, render
// the plan, create the track, materialize — without prompting, subjects
// or the track flags.
type recipeTrackRequest struct {
	Recipe string            // a name or name@version; paths are refused
	Vars   map[string]string // already split by var name
	Title  string            // "" takes the recipe's track.title
	Type   string            // "" takes track.type, then config
	Slug   string            // "" derives it from the title
}

// recipeTrackOutcome is what a request created.
type recipeTrackOutcome struct {
	Track  *core.Track
	Plan   *recipePlan
	Result *core.MaterializeResult
}

// recipeRequestError is a failure caused by the request itself: it maps to
// a 4xx whose code is Code.
type recipeRequestError struct {
	Code string
	Err  error
}

func (e *recipeRequestError) Error() string { return e.Err.Error() }
func (e *recipeRequestError) Unwrap() error { return e.Err }

func requestErr(code string, err error) error { return &recipeRequestError{Code: code, Err: err} }

// Request error codes.
const (
	codeRecipeNotFound = "recipe_not_found"
	codeInvalidRecipe  = "invalid_recipe"
	codeInvalidVars    = "invalid_vars"
)

// createTrackFromRecipe materializes the request into a new track. Every
// failure is a *recipeRequestError except the ones the track service and
// materializer report themselves, which carry their own messages.
func createTrackFromRecipe(ctx context.Context, s *storage.SQLiteStorage, req recipeTrackRequest) (*recipeTrackOutcome, error) {
	loc := recipeLocator()
	r, err := loc.LocateNamed(req.Recipe)
	if err != nil {
		if core.IsRecipeNotFound(err) {
			return nil, requestErr(codeRecipeNotFound, err)
		}
		return nil, requestErr(codeInvalidRecipe, err)
	}
	expanded, err := core.Expand(r, loc)
	if err != nil {
		return nil, requestErr(codeInvalidRecipe, err)
	}
	opts := recipeCreateOpts{Recipe: req.Recipe, ProvidedVars: req.Vars}
	if opts.ProvidedVars == nil {
		opts.ProvidedVars = map[string]string{}
	}
	vars, err := bindRecipeVars(nil, r, opts, nil)
	if err != nil {
		return nil, requestErr(codeInvalidVars, err)
	}
	track, err := trackFromRecipe(r, req.Title, vars, nil, req.Type, func(title, trackType string) (*core.Track, error) {
		return newTrackBase(req.Slug, title, trackType)
	})
	if err != nil {
		return nil, requestErr(codeInvalidRecipe, err)
	}
	var subj *recipeSubject
	if r.Requires.Subject == core.SubjectTrack {
		subj = subjectFromTrack(track)
	}
	if err := enforceRecipeSubject(r, subj, "this endpoint cannot bind a task subject; use the CLI with --for"); err != nil {
		return nil, requestErr(codeInvalidRecipe, err)
	}
	plan, err := renderRecipePlan(nil, r, expanded, opts, subj, nil)
	if err != nil {
		return nil, requestErr(codeInvalidRecipe, err)
	}
	planMD, err := renderTrackPlan(r, plan)
	if err != nil {
		return nil, requestErr(codeInvalidRecipe, err)
	}
	if err := createTrackWithPlan(ctx, io.Discard, s, track, planMD); err != nil {
		return nil, err
	}
	target := recipeTarget{TrackID: track.ID, TrackType: track.Type, ProjectID: currentProjectID()}
	res, err := materializeRecipe(ctx, io.Discard, s, plan, target, false)
	if err != nil {
		return nil, fmt.Errorf("track %s created but materializing recipe %s failed: %w", formatTrackAlias(track), r.Name, err)
	}
	return &recipeTrackOutcome{Track: track, Plan: plan, Result: res}, nil
}

// isRequestError reports whether err is a *recipeRequestError, returning it.
func isRequestError(err error) (*recipeRequestError, bool) {
	var re *recipeRequestError
	return re, errors.As(err, &re)
}
