package cli

// HTTP coverage for track meta and PATCH /tracks/{id}: POST /tracks
// accepts `meta`; PATCH merges it per JSON Merge Patch (RFC 7396) and
// applies status, assigned_to and due through the same validated path
// `track update` uses; GET /tracks filters on meta.<key>=<value>.

import (
	"encoding/json"
	"net/http"
	"net/url"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"hop.top/tlc/internal/core"
)

func decodeTrack(t *testing.T, body []byte) core.Track {
	t.Helper()
	var tr core.Track
	if err := json.Unmarshal(body, &tr); err != nil {
		t.Fatalf("decode track: %v\n%s", err, body)
	}
	return tr
}

func getTrackBodyOverHTTP(t *testing.T, router http.Handler, id string) string {
	t.Helper()
	rec := doServeRequestRaw(t, router, "GET", "/tracks/"+id, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /tracks/%s: %d %s", id, rec.Code, rec.Body.String())
	}
	return rec.Body.String()
}

func createTrackOverHTTP(t *testing.T, router http.Handler, body string) core.Track {
	t.Helper()
	rec := doServeRequestRaw(t, router, "POST", "/tracks", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /tracks %s: %d %s", body, rec.Code, rec.Body.String())
	}
	return decodeTrack(t, rec.Body.Bytes())
}

func TestServe_TrackCreateWithMeta(t *testing.T) {
	withTestLock(func() {
		_, cleanup := setupTestDir(t)
		defer cleanup()
		router := newTestServeRouter(t)

		created := createTrackOverHTTP(t, router,
			`{"slug":"with-meta","title":"With meta","type":"feature","meta":{"owner":"ana","n":3,"cfg":{"a":true}}}`)
		want := map[string]any{"owner": "ana", "n": 3.0, "cfg": map[string]any{"a": true}}
		if !reflect.DeepEqual(created.Meta, want) {
			t.Fatalf("created meta = %v, want %v", created.Meta, want)
		}
		got := decodeTrack(t, []byte(getTrackBodyOverHTTP(t, router, created.ID)))
		if !reflect.DeepEqual(got.Meta, want) {
			t.Fatalf("stored meta = %v, want %v", got.Meta, want)
		}
	})
}

func TestServe_TrackCreateReservedMetaRejected(t *testing.T) {
	withTestLock(func() {
		_, cleanup := setupTestDir(t)
		defer cleanup()
		router := newTestServeRouter(t)

		for _, key := range []string{"plans", "external_uid", "x_tlc", "due_date_only"} {
			rec := doServeRequestRaw(t, router, "POST", "/tracks",
				`{"slug":"bad-meta","title":"bad","type":"feature","meta":{"ok":"1","`+key+`":"x"}}`)
			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("meta.%s: expected 422, got %d: %s", key, rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), `reserved meta key \"`+key+`\"`) {
				t.Errorf("meta.%s: error should name the reserved key: %s", key, rec.Body.String())
			}
		}
		if rec := doServeRequestRaw(t, router, "GET", "/tracks/bad-meta", ""); rec.Code != http.StatusNotFound {
			t.Fatalf("a rejected create left a track behind: %d %s", rec.Code, rec.Body.String())
		}
	})
}

func TestServe_TrackPatchMetaMergePatch(t *testing.T) {
	withTestLock(func() {
		_, cleanup := setupTestDir(t)
		defer cleanup()
		router := newTestServeRouter(t)

		tr := createTrackOverHTTP(t, router,
			`{"slug":"patch-meta","title":"t","type":"feature","meta":{"keep":"k","owner":"old","gone":"x","cfg":{"a":"1","b":"2"}}}`)

		// Raw JSON: null must reach the handler as null.
		patch := func(body string) (int, string) {
			rec := doServeRequestRaw(t, router, "PATCH", "/tracks/"+tr.ID, body)
			return rec.Code, rec.Body.String()
		}

		code, body := patch(`{"meta":{"owner":"new","added":1,"gone":null,"cfg":{"b":null,"c":"3"}}}`)
		if code != http.StatusOK {
			t.Fatalf("PATCH: %d %s", code, body)
		}
		want := map[string]any{
			"keep": "k", "owner": "new", "added": 1.0,
			"cfg": map[string]any{"a": "1", "c": "3"},
		}
		if got := decodeTrack(t, []byte(body)); !reflect.DeepEqual(got.Meta, want) {
			t.Fatalf("merged meta = %v, want %v", got.Meta, want)
		}
		if got := decodeTrack(t, []byte(getTrackBodyOverHTTP(t, router, tr.ID))); !reflect.DeepEqual(got.Meta, want) {
			t.Fatalf("stored meta = %v, want %v", got.Meta, want)
		}

		// Absent, empty and null `meta` leave everything alone.
		for _, b := range []string{`{"assigned_to":"ana"}`, `{"meta":{}}`, `{"meta":null}`} {
			if code, body := patch(b); code != http.StatusOK {
				t.Fatalf("PATCH %s: %d %s", b, code, body)
			}
			if after := decodeTrack(t, []byte(getTrackBodyOverHTTP(t, router, tr.ID))); !reflect.DeepEqual(after.Meta, want) {
				t.Fatalf("PATCH %s changed meta: %v", b, after.Meta)
			}
		}
	})
}

func TestServe_TrackPatchReservedMetaRejectedAtomically(t *testing.T) {
	withTestLock(func() {
		_, cleanup := setupTestDir(t)
		defer cleanup()
		router := newTestServeRouter(t)

		tr := createTrackOverHTTP(t, router,
			`{"slug":"atomic-meta","title":"orig","type":"feature","meta":{"owner":"ana"}}`)
		before := getTrackBodyOverHTTP(t, router, tr.ID)

		for _, body := range []string{
			`{"assigned_to":"bob","meta":{"owner":"bob","plans":["x"]}}`,
			`{"assigned_to":"bob","meta":{"plans":null}}`,
			`{"assigned_to":"bob","meta":{"external_uid":"u"}}`,
			`{"assigned_to":"bob","due":"2030-01-01","meta":{"x_tlc":{}}}`,
			`{"assigned_to":"bob","status":"abandoned","meta":{"due_date_only":true}}`,
		} {
			rec := doServeRequestRaw(t, router, "PATCH", "/tracks/"+tr.ID, body)
			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("PATCH %s: expected 422, got %d: %s", body, rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), "reserved meta key") {
				t.Errorf("PATCH %s: error should say reserved meta key: %s", body, rec.Body.String())
			}
			if after := getTrackBodyOverHTTP(t, router, tr.ID); after != before {
				t.Fatalf("PATCH %s partially applied:\nbefore %s\nafter  %s", body, before, after)
			}
		}
	})
}

func TestServe_TrackPatchFields(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		router := newTestServeRouter(t)

		tr := createTrackOverHTTP(t, router, `{"slug":"patch-fields","title":"t","type":"feature"}`)

		// Activation needs a linked task, same as the CLI.
		s, _ := getStorageRaw()
		defer s.Close()
		linked := &core.Task{ID: core.NewTaskID(), Title: "linked", Status: core.StatusTodo, TrackID: &tr.ID}
		if err := s.CreateTask(ctx, linked); err != nil {
			t.Fatal(err)
		}

		stored := decodeTrack(t, []byte(getTrackBodyOverHTTP(t, router, tr.ID)))
		rec := doServeRequestRaw(t, router, "PATCH", "/tracks/"+tr.ID,
			`{"status":"active","assigned_to":"@ana","due":"2030-01-02T03:04:05Z"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("PATCH: %d %s", rec.Code, rec.Body.String())
		}
		got := decodeTrack(t, []byte(getTrackBodyOverHTTP(t, router, tr.ID)))
		if got.Status != core.TrackStatusActive {
			t.Errorf("status = %s, want active", got.Status)
		}
		if got.AssignedTo == nil || *got.AssignedTo != "ana" {
			t.Errorf("assigned_to = %v, want ana (leading @ stripped as in the CLI)", got.AssignedTo)
		}
		if got.DueAt == nil || !got.DueAt.Equal(time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)) {
			t.Errorf("due_at = %v, want 2030-01-02T03:04:05Z", got.DueAt)
		}
		if got.UpdatedAt.Before(stored.UpdatedAt) {
			t.Errorf("updated_at went backwards: %v -> %v", stored.UpdatedAt, got.UpdatedAt)
		}

		// "-" clears assignee and due, as in the CLI.
		rec = doServeRequestRaw(t, router, "PATCH", "/tracks/"+tr.ID, `{"assigned_to":"-","due":"-"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("PATCH clear: %d %s", rec.Code, rec.Body.String())
		}
		got = decodeTrack(t, []byte(getTrackBodyOverHTTP(t, router, tr.ID)))
		if got.AssignedTo != nil || got.DueAt != nil {
			t.Errorf("after clear: assigned_to=%v due_at=%v, want both unset", got.AssignedTo, got.DueAt)
		}
		if got.Status != core.TrackStatusActive {
			t.Errorf("fields omitted from PATCH changed status to %s", got.Status)
		}
	})
}

func TestServe_TrackPatchInvalidInput(t *testing.T) {
	withTestLock(func() {
		_, cleanup := setupTestDir(t)
		defer cleanup()
		router := newTestServeRouter(t)

		tr := createTrackOverHTTP(t, router,
			`{"slug":"patch-bad","title":"t","type":"feature","meta":{"owner":"ana"}}`)
		before := getTrackBodyOverHTTP(t, router, tr.ID)

		// pending -> completed is not an allowed transition: a client
		// error carrying the message the CLI prints, nothing applied
		// (including the meta in the same request).
		rec := doServeRequestRaw(t, router, "PATCH", "/tracks/"+tr.ID,
			`{"status":"completed","meta":{"owner":"bob"}}`)
		if rec.Code != http.StatusConflict {
			t.Fatalf("invalid transition: expected 409, got %d: %s", rec.Code, rec.Body.String())
		}
		wantMsg := core.ErrInvalidTrackTransition{
			From: core.TrackStatusPending, To: core.TrackStatusCompleted,
			Msg: "transition not allowed",
			Allowed: []core.TrackStatus{
				core.TrackStatusActive, core.TrackStatusAbandoned,
			},
		}.Error()
		if !strings.Contains(rec.Body.String(), wantMsg) {
			t.Errorf("error should carry the CLI message %q, got: %s", wantMsg, rec.Body.String())
		}
		if after := getTrackBodyOverHTTP(t, router, tr.ID); after != before {
			t.Fatalf("rejected transition partially applied:\nbefore %s\nafter  %s", before, after)
		}

		// Activating with no linked task is the other guarded transition.
		rec = doServeRequestRaw(t, router, "PATCH", "/tracks/"+tr.ID, `{"status":"active"}`)
		if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "0 linked tasks") {
			t.Fatalf("activate empty track: %d %s", rec.Code, rec.Body.String())
		}

		for _, tc := range []struct {
			body string
			code int
		}{
			{`{"status":"bogus"}`, http.StatusUnprocessableEntity},
			{`{"due":"not a date"}`, http.StatusUnprocessableEntity},
			{`{"meta":{"":"x"}}`, http.StatusUnprocessableEntity},
			{`{"meta":"notanobject"}`, http.StatusBadRequest},
			{`not json`, http.StatusBadRequest},
		} {
			rec := doServeRequestRaw(t, router, "PATCH", "/tracks/"+tr.ID, tc.body)
			if rec.Code != tc.code {
				t.Errorf("PATCH %s: expected %d, got %d: %s", tc.body, tc.code, rec.Code, rec.Body.String())
			}
		}
		if after := getTrackBodyOverHTTP(t, router, tr.ID); after != before {
			t.Fatalf("rejected requests changed the track:\nbefore %s\nafter  %s", before, after)
		}
	})
}

func TestServe_TrackPatchUnknownTrack(t *testing.T) {
	withTestLock(func() {
		_, cleanup := setupTestDir(t)
		defer cleanup()
		router := newTestServeRouter(t)

		rec := doServeRequestRaw(t, router, "PATCH", "/tracks/no-such-track", `{"assigned_to":"ana"}`)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

func TestServe_TrackPatchByAliasAndSlug(t *testing.T) {
	withTestLock(func() {
		_, cleanup := setupTestDir(t)
		defer cleanup()
		router := newTestServeRouter(t)

		createTrackOverHTTP(t, router, `{"slug":"by-slug","title":"t","type":"feature"}`)
		rec := doServeRequestRaw(t, router, "PATCH", "/tracks/by-slug", `{"meta":{"owner":"ana"}}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("PATCH by slug: %d %s", rec.Code, rec.Body.String())
		}
		if got := decodeTrack(t, rec.Body.Bytes()); got.Meta["owner"] != "ana" {
			t.Fatalf("meta = %v", got.Meta)
		}
	})
}

func listTrackSlugsOverHTTP(t *testing.T, router http.Handler, query string) (int, []string) {
	t.Helper()
	rec := doServeRequestRaw(t, router, "GET", "/tracks"+query, "")
	if rec.Code != http.StatusOK {
		return rec.Code, nil
	}
	var resp struct {
		Tracks []core.Track `json:"tracks"`
		Total  int          `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode list: %v\n%s", err, rec.Body.String())
	}
	slugs := []string{}
	for _, tr := range resp.Tracks {
		slugs = append(slugs, tr.Slug)
	}
	if resp.Total != len(slugs) {
		t.Errorf("total = %d, want %d", resp.Total, len(slugs))
	}
	return rec.Code, slugs
}

func TestServe_TrackListMetaFilter(t *testing.T) {
	withTestLock(func() {
		_, cleanup := setupTestDir(t)
		defer cleanup()
		router := newTestServeRouter(t)

		createTrackOverHTTP(t, router, `{"slug":"one-trk","title":"1","type":"feature","meta":{"owner":"ana","env":"prod","n":5}}`)
		createTrackOverHTTP(t, router, `{"slug":"two-trk","title":"2","type":"feature","meta":{"owner":"bob","env":"prod"}}`)
		createTrackOverHTTP(t, router, `{"slug":"three-trk","title":"3","type":"bug"}`)

		cases := []struct {
			name, query string
			want        []string
		}{
			{"match one", "?meta.owner=ana", []string{"one-trk"}},
			{"match two", "?meta.env=prod", []string{"one-trk", "two-trk"}},
			{"AND across keys", "?meta.env=prod&meta.owner=bob", []string{"two-trk"}},
			{"no match", "?meta.owner=carol", []string{}},
			{"number by string form", "?meta.n=5", []string{"one-trk"}},
			{"with other filters", "?meta.env=prod&type=bug", []string{}},
			{"no meta params", "", []string{"one-trk", "three-trk", "two-trk"}},
		}
		for _, tc := range cases {
			code, got := listTrackSlugsOverHTTP(t, router, tc.query)
			if code != http.StatusOK {
				t.Errorf("%s: status %d", tc.name, code)
				continue
			}
			sort.Strings(got)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
			}
		}
	})
}

func TestServe_TrackListMetaFilterRejectsBadKeys(t *testing.T) {
	withTestLock(func() {
		_, cleanup := setupTestDir(t)
		defer cleanup()
		router := newTestServeRouter(t)

		createTrackOverHTTP(t, router, `{"slug":"only-trk","title":"1","type":"feature","meta":{"owner":"ana"}}`)

		for _, key := range []string{
			"a.b", "a b", "x') OR 1=1 --", `x" OR "1"="1`, "o'q", "a;b", "", "-x",
		} {
			q := "?" + url.QueryEscape("meta."+key) + "=1"
			rec := doServeRequestRaw(t, router, "GET", "/tracks"+q, "")
			if rec.Code != http.StatusUnprocessableEntity {
				t.Errorf("key %q: expected 422, got %d: %s", key, rec.Code, rec.Body.String())
				continue
			}
			if !strings.Contains(rec.Body.String(), "validation_error") {
				t.Errorf("key %q: want validation_error body, got %s", key, rec.Body.String())
			}
		}
		if _, got := listTrackSlugsOverHTTP(t, router, ""); len(got) != 1 {
			t.Fatalf("tracks after rejected filters: %v", got)
		}
	})
}
