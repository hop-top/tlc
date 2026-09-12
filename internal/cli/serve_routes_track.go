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

// registerTrackRoutes wires a minimal track REST surface onto router
// under /tracks: list, show, create. Update/lifecycle transitions
// (activate, complete, abandon) are deferred — see the final report —
// to keep this first pass focused on a clean, well-tested subset.
func registerTrackRoutes(router *api.Router, deps *serveDeps) {
	tracks := router.Group("/tracks")

	tracks.Handle("GET", "", handleTrackList(deps))
	tracks.Handle("POST", "", handleTrackCreate(deps))
	tracks.Handle("GET", "/{id}", handleTrackShow(deps))
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

		now := time.Now().UTC()
		track := &core.Track{
			Slug:      req.Slug,
			Title:     req.Title,
			Type:      req.Type,
			Status:    core.TrackStatusPending,
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
