package cli

import (
	"context"
	"errors"
	"testing"

	"hop.top/tlc/internal/core"
)

type fakeAliasStorage struct {
	tasks  map[string]*core.Task
	tracks map[string]*core.Track
	err    error
}

func (f *fakeAliasStorage) GetTask(_ context.Context, id string) (*core.Task, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.tasks[id], nil
}

func (f *fakeAliasStorage) GetTrack(_ context.Context, id string) (*core.Track, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.tracks[id], nil
}

func TestAliasReference(t *testing.T) {
	const proj = "hop-top/tlc"
	const taskTID = "task_01h455vb4pex5vsknk084sn001"
	const trackTID = "track_01h455vb4pex5vsknk084sn999"

	store := &fakeAliasStorage{
		tasks: map[string]*core.Task{
			taskTID: {ID: taskTID, Seq: 1313},
		},
		tracks: map[string]*core.Track{
			trackTID: {ID: trackTID, Slug: "tlc-typeid-display-hygiene"},
		},
	}

	cases := []struct {
		name string
		in   string
		want string
	}{
		{"local task typeid → alias", "tlc://" + proj + "/" + taskTID, "tlc://" + proj + "/T-1313"},
		{"local track typeid → slug", "tlc://" + proj + "/" + trackTID, "tlc://" + proj + "/tlc-typeid-display-hygiene"},
		{"foreign project unchanged", "tlc://other/proj/" + taskTID, "tlc://other/proj/" + taskTID},
		{"local relative form unchanged", "tlc:///T-1313", "tlc:///T-1313"},
		{"already aliased unchanged", "tlc://" + proj + "/T-1313", "tlc://" + proj + "/T-1313"},
		{"https scheme unchanged", "https://example.com/issue/42", "https://example.com/issue/42"},
		{"empty unchanged", "", ""},
		{"path tail rejected (defensive)", "tlc://" + proj + "/" + taskTID + "/foo", "tlc://" + proj + "/" + taskTID + "/foo"},
		{"query tail rejected", "tlc://" + proj + "/" + taskTID + "?x=1", "tlc://" + proj + "/" + taskTID + "?x=1"},
		{"fragment tail rejected", "tlc://" + proj + "/" + taskTID + "#frag", "tlc://" + proj + "/" + taskTID + "#frag"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := aliasReference(context.Background(), store, tc.in, proj)
			if got != tc.want {
				t.Errorf("aliasReference(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestAliasReference_LookupMissFallsBack(t *testing.T) {
	store := &fakeAliasStorage{tasks: map[string]*core.Task{}}
	const ref = "tlc://hop-top/tlc/task_01h455vb4pex5vsknk084sn999"
	if got := aliasReference(context.Background(), store, ref, "hop-top/tlc"); got != ref {
		t.Errorf("missing task should fall back to ref; got %q", got)
	}
}

func TestAliasReference_StorageErrorFallsBack(t *testing.T) {
	store := &fakeAliasStorage{err: errors.New("db closed")}
	const ref = "tlc://hop-top/tlc/task_01h455vb4pex5vsknk084sn001"
	if got := aliasReference(context.Background(), store, ref, "hop-top/tlc"); got != ref {
		t.Errorf("storage error should fall back to ref; got %q", got)
	}
}

func TestAliasReference_NilStorage(t *testing.T) {
	const ref = "tlc://hop-top/tlc/task_01h455vb4pex5vsknk084sn001"
	if got := aliasReference(context.Background(), nil, ref, "hop-top/tlc"); got != ref {
		t.Errorf("nil storage should fall back to ref; got %q", got)
	}
}
