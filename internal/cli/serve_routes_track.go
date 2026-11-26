package cli

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"hop.top/kit/go/transport/api"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/events"
)

// registerTrackRoutes wires the track REST surface onto router under
// /tracks: list, show, create and a partial update (PATCH). Lifecycle
// verbs beyond a status change (archive, abandon with cleanup) stay on
// the CLI.
func registerTrackRoutes(router *api.Router, deps *serveDeps) {
	tracks := router.Group("/tracks")

	tracks.Handle("GET", "", handleTrackList(deps))
	tracks.Handle("POST", "", handleTrackCreate(deps))
	tracks.Handle("GET", "/{id}", handleTrackShow(deps))
	tracks.Handle("PATCH", "/{id}", handleTrackUpdate(deps))
}

type trackListResponse struct {
	Tracks []*core.Track `json:"tracks"`
	Total  int           `json:"total"`
}

func handleTrackList(deps *serveDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		q := core.TrackQuery{Limit: 100}

		query := r.URL.Query()
		if v := query.Get("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				q.Limit = n
			}
		}
		if v := query.Get("offset"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n >= 0 {
				q.Offset = n
			}
		}
		if v := query.Get("type"); v != "" {
			q.Type = v
		}
		if v := query.Get("status"); v != "" {
			q.Status = []core.TrackStatus{core.TrackStatus(v)}
		}
		if query.Get("all_projects") == "true" {
			q.AllProjects = true
		}
		// meta.<key>=<value>, repeatable, ANDed: string equality on the
		// member's scalar form. Keys are whitelisted; see core.ValidateMetaFilterKey.
		meta, err := metaFiltersFromQuery(query)
		if err != nil {
			writeTrackUpdateError(w, err)
			return
		}
		q.Meta = meta

		tracksList, err := deps.storage.ListTracks(ctx, q)
		if err != nil {
			writeAPIErrorf(w, http.StatusInternalServerError, "internal_error", "failed to list tracks: %v", err)
			return
		}
		api.JSON(w, http.StatusOK, trackListResponse{Tracks: tracksList, Total: len(tracksList)})
	}
}

func handleTrackShow(deps *serveDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		id := api.PathParam(r, "id")

		resolvedID, err := resolveTrackID(ctx, deps.storage, id)
		if err != nil {
			writeTrackResolveError(w, id, err)
			return
		}

		track, err := deps.storage.GetTrack(ctx, resolvedID)
		if err != nil {
			writeAPIErrorf(w, http.StatusInternalServerError, "internal_error", "failed to get track: %v", err)
			return
		}
		if track == nil {
			writeAPIErrorf(w, http.StatusNotFound, "not_found", "track %q not found", id)
			return
		}
		api.JSON(w, http.StatusOK, track)
	}
}

// writeTrackResolveError maps a resolveTrackID failure to the right HTTP
// status, mirroring writeResolveError's task-side convention:
// genuinely-not-found (ErrTrackNotFound) is 404, everything else
// (ambiguous prefix/fuzzy matches, empty input) is 422 since the input
// itself was invalid or under-specified rather than a lookup miss.
func writeTrackResolveError(w http.ResponseWriter, id string, err error) {
	if errors.Is(err, ErrTrackNotFound) {
		writeAPIErrorf(w, http.StatusNotFound, "not_found", "track %q not found", id)
		return
	}
	writeAPIErrorf(w, http.StatusUnprocessableEntity, "invalid_track", "%v", err)
}

// trackCreateRequest is the JSON body accepted by POST /tracks.
type trackCreateRequest struct {
	Slug  string `json:"slug"`
	Title string `json:"title"`
	Type  string `json:"type"`
	// Meta is user meta to store on the new track. Reserved keys (see
	// core.IsReservedTrackMetaKey) are refused with 422.
	Meta map[string]any `json:"meta,omitempty"`
}

func handleTrackCreate(deps *serveDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		var req trackCreateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeAPIErrorf(w, http.StatusBadRequest, "invalid_json", "invalid request body: %v", err)
			return
		}
		if req.Slug == "" {
			writeAPIErrorf(w, http.StatusUnprocessableEntity, "validation_error", "slug is required")
			return
		}
		// Write path: the request mints a new slug.
		if err := core.ValidateNewTrackSlug(req.Slug, getConfigSlugMaxLen()); err != nil {
			writeAPIErrorf(w, http.StatusUnprocessableEntity, "validation_error", "%v", err)
			return
		}

		meta, err := core.MergeTrackMeta(nil, req.Meta, nil)
		if err != nil {
			writeTrackUpdateError(w, metaWriteError(err))
			return
		}

		now := time.Now().UTC()
		track := &core.Track{
			Slug:      req.Slug,
			Title:     req.Title,
			Type:      req.Type,
			Status:    core.TrackStatusPending,
			Meta:      meta,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if proj := core.DetectProject(); proj != nil && proj.ProjectID != "" {
			track.ProjectID = &proj.ProjectID
		}

		svc := core.NewTrackService(
			deps.storage, deps.storage,
			core.WithSlugMaxLen(getConfigSlugMaxLen()),
		)
		if err := svc.CreateTrack(ctx, track); err != nil {
			writeAPIErrorf(w, http.StatusUnprocessableEntity, "validation_error", "%v", err)
			return
		}

		publishDomainEvent(ctx, deps.publisher, events.TopicTrackCreated, events.TrackCreatedPayload{
			TrackID: track.ID,
			Title:   track.Title,
			Type:    track.Type,
		})

		api.JSON(w, http.StatusCreated, track)
	}
}

// trackUpdateRequest is the JSON body accepted by PATCH /tracks/{id}.
// Only fields present are applied, so an omitted field never clobbers
// stored data; they go through the same TrackFieldChanges path as
// `track update`, so status transitions are validated exactly as the CLI
// validates them.
type trackUpdateRequest struct {
	Status     *string `json:"status,omitempty"`
	AssignedTo *string `json:"assigned_to,omitempty"`
	Due        *string `json:"due,omitempty"`
	// Meta is a JSON Merge Patch (RFC 7396) over the track's user meta:
	// a key sets, null removes, an object merges recursively, absent
	// keys are left alone. Absent, null or {} changes nothing. Reserved
	// keys (see core.IsReservedTrackMetaKey) are refused with 422.
	Meta map[string]any `json:"meta,omitempty"`
}

func handleTrackUpdate(deps *serveDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		id := api.PathParam(r, "id")

		resolvedID, err := resolveTrackID(ctx, deps.storage, id)
		if err != nil {
			writeTrackResolveError(w, id, err)
			return
		}

		var req trackUpdateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeAPIErrorf(w, http.StatusBadRequest, "invalid_json", "invalid request body: %v", err)
			return
		}
		if req.Status != nil && (*req.Status == "" || !core.ValidTrackStatus(core.TrackStatus(*req.Status))) {
			writeAPIErrorf(w, http.StatusUnprocessableEntity, "invalid_status",
				"track status %q invalid; valid: pending, active, completed, abandoned, archived", *req.Status)
			return
		}

		apply, err := TrackFieldChanges{
			Status: req.Status, AssignedTo: req.AssignedTo, Due: req.Due, Meta: req.Meta,
		}.mutation()
		if err != nil {
			writeTrackUpdateError(w, err)
			return
		}

		svc := core.NewTrackService(deps.storage, deps.storage, core.WithSlugMaxLen(getConfigSlugMaxLen()))
		if err := svc.UpdateTrack(ctx, resolvedID, apply); err != nil {
			writeTrackUpdateError(w, err)
			return
		}

		track, err := deps.storage.GetTrack(ctx, resolvedID)
		if err != nil {
			writeAPIErrorf(w, http.StatusInternalServerError, "internal_error", "failed to get track: %v", err)
			return
		}
		api.JSON(w, http.StatusOK, track)
	}
}
