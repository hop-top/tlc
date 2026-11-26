package storage

import (
	"context"
	"errors"
	"sort"
	"strings"
	"testing"
	"time"

	"hop.top/tlc/internal/core"
)

func trackIDs(ts []*core.Track) []string {
	ids := make([]string, len(ts))
	for i, tr := range ts {
		ids[i] = tr.ID
	}
	sort.Strings(ids)
	return ids
}

func seedMetaTracks(t *testing.T, s *SQLiteStorage) {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Second)
	for _, tr := range []*core.Track{
		{ID: "t-ana", Slug: "t-ana", Title: "ana", Type: "feature", Meta: map[string]any{"owner": "ana", "env": "prod", "n": 5.0, "ok": true}},
		{ID: "t-bob", Slug: "t-bob", Title: "bob", Type: "feature", Meta: map[string]any{"owner": "bob", "env": "prod", "cfg": map[string]any{"a": "1"}, "list": []any{"x"}}},
		{ID: "t-none", Slug: "t-none", Title: "none", Type: "feature"},
		{ID: "t-quote", Slug: "t-quote", Title: "quote", Type: "feature", Meta: map[string]any{"owner": "o'brien; DROP TABLE tracks"}},
	} {
		tr.Status = core.TrackStatusPending
		tr.CreatedAt, tr.UpdatedAt = now, now
		if err := s.CreateTrack(context.Background(), tr); err != nil {
			t.Fatalf("CreateTrack %s: %v", tr.ID, err)
		}
	}
}

func listMeta(t *testing.T, s *SQLiteStorage, meta map[string]string) ([]string, error) {
	t.Helper()
	got, err := s.ListTracks(context.Background(), core.TrackQuery{Meta: meta})
	if err != nil {
		return nil, err
	}
	return trackIDs(got), nil
}

func eq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestListTracks_MetaFilter(t *testing.T) {
	s := newTrackTestStorage(t)
	seedMetaTracks(t, s)

	cases := []struct {
		name string
		meta map[string]string
		want []string
	}{
		{"no filter lists all", nil, []string{"t-ana", "t-bob", "t-none", "t-quote"}},
		{"match one", map[string]string{"owner": "ana"}, []string{"t-ana"}},
		{"match two", map[string]string{"env": "prod"}, []string{"t-ana", "t-bob"}},
		{"no match", map[string]string{"owner": "carol"}, nil},
		{"absent key", map[string]string{"nokey": "x"}, nil},
		{"keys AND together", map[string]string{"env": "prod", "owner": "bob"}, []string{"t-bob"}},
		{"AND with no overlap", map[string]string{"owner": "ana", "env": "staging"}, nil},
		{"exact not prefix", map[string]string{"owner": "an"}, nil},
		{"case sensitive", map[string]string{"owner": "Ana"}, nil},
		{"number compares by string form", map[string]string{"n": "5"}, []string{"t-ana"}},
		{"bool compares by string form", map[string]string{"ok": "true"}, []string{"t-ana"}},
		{"bool is not 1", map[string]string{"ok": "1"}, nil},
		{"object never matches", map[string]string{"cfg": "{\"a\":\"1\"}"}, nil},
		{"array never matches", map[string]string{"list": "[\"x\"]"}, nil},
		{"empty value does not match absent", map[string]string{"nokey": ""}, nil},
		{"value is bound, not interpolated", map[string]string{"owner": "o'brien; DROP TABLE tracks"}, []string{"t-quote"}},
		{"injection-shaped value matches nothing", map[string]string{"owner": "x' OR '1'='1"}, nil},
	}
	for _, tc := range cases {
		got, err := listMeta(t, s, tc.meta)
		if err != nil {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		if !eq(got, tc.want) {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestListTracks_MetaFilterCombinesWithOtherFilters(t *testing.T) {
	s := newTrackTestStorage(t)
	seedMetaTracks(t, s)
	got, err := s.ListTracks(context.Background(), core.TrackQuery{
		Type: "feature", Status: []core.TrackStatus{core.TrackStatusPending},
		Meta: map[string]string{"env": "prod"}, Limit: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("limit with meta filter: got %d tracks, want 1", len(got))
	}
}

func TestListTracks_MetaFilterRejectsBadKeys(t *testing.T) {
	s := newTrackTestStorage(t)
	seedMetaTracks(t, s)
	for _, k := range []string{"", "a.b", "a b", "o'q", `a"b`, "x') OR 1=1 --", "a)--"} {
		_, err := listMeta(t, s, map[string]string{k: "x"})
		if !errors.Is(err, core.ErrInvalidMetaFilterKey) {
			t.Errorf("key %q: err = %v, want ErrInvalidMetaFilterKey", k, err)
		}
	}
	// The table survived every attempt.
	if got, err := listMeta(t, s, nil); err != nil || len(got) != 4 {
		t.Fatalf("tracks after rejected keys: %v, %v", got, err)
	}
}

func TestMetaFilterClauses(t *testing.T) {
	clauses, args, err := MetaFilterClauses("tracks.meta", map[string]string{"b": "2", "a": "1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(clauses) != 2 {
		t.Fatalf("clauses = %v, want one per key", clauses)
	}
	joined := strings.Join(clauses, " ")
	if strings.Contains(joined, "'1'") || strings.Contains(joined, `"a"`) {
		t.Errorf("user input interpolated into SQL: %s", joined)
	}
	if strings.Count(joined, "?") != len(args) {
		t.Errorf("%d placeholders, %d args", strings.Count(joined, "?"), len(args))
	}
	// Deterministic: keys sorted, so a=1 comes first.
	if !strings.Contains(args[0].(string), "a") {
		t.Errorf("args not sorted by key: %v", args)
	}
	if c, a, err := MetaFilterClauses("meta", nil); err != nil || c != nil || a != nil {
		t.Errorf("empty filter: %v %v %v", c, a, err)
	}
	if _, _, err := MetaFilterClauses("meta", map[string]string{"x'": "1"}); !errors.Is(err, core.ErrInvalidMetaFilterKey) {
		t.Errorf("bad key err = %v", err)
	}
}
