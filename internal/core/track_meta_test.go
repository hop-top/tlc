package core

import (
	"errors"
	"reflect"
	"testing"
)

func TestIsReservedTrackMetaKey(t *testing.T) {
	// Keys tlc itself writes into a track's meta: the plan links, the
	// calendar-import carriers and the all-day marker.
	for _, k := range []string{"plans", "external_uid", "x_tlc", "due_date_only"} {
		if !IsReservedTrackMetaKey(k) {
			t.Errorf("IsReservedTrackMetaKey(%q) = false, want true", k)
		}
	}
	// User keys, including names that are reserved on a task.
	for _, k := range []string{"owner", "team", "Plans", "plan", "eva", "blocked_by", "origin_id"} {
		if IsReservedTrackMetaKey(k) {
			t.Errorf("IsReservedTrackMetaKey(%q) = true, want false", k)
		}
	}
}

func TestTaskAndTrackReservedSetsAreIndependent(t *testing.T) {
	if IsReservedMetaKey("plans") {
		t.Error("plans is a track key; the task set must not reserve it")
	}
	if !IsReservedMetaKey("eva") {
		t.Error("eva must stay reserved on tasks")
	}
}

//nolint:gocognit,gocyclo // one subtest per merge rule; the nesting is the closures
func TestMergeTrackMeta(t *testing.T) {
	t.Run("merges and keeps internal keys", func(t *testing.T) {
		in := map[string]any{"plans": []string{"a.md"}, "owner": "ana", "keep": "k"}
		got, err := MergeTrackMeta(in, map[string]any{"owner": "bob", "added": "1"}, []string{"keep"})
		if err != nil {
			t.Fatal(err)
		}
		want := map[string]any{"plans": []string{"a.md"}, "owner": "bob", "added": "1"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %v, want %v", got, want)
		}
	})

	t.Run("merge patch null removes and objects merge", func(t *testing.T) {
		in := map[string]any{"cfg": map[string]any{"a": "1", "b": "2"}, "gone": "x"}
		got, err := MergeTrackMeta(in, map[string]any{"cfg": map[string]any{"b": nil, "c": "3"}, "gone": nil}, nil)
		if err != nil {
			t.Fatal(err)
		}
		want := map[string]any{"cfg": map[string]any{"a": "1", "c": "3"}}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %v, want %v", got, want)
		}
	})

	t.Run("reserved key rejected on set and unset, input untouched", func(t *testing.T) {
		for name, tc := range map[string]struct {
			set   map[string]any
			unset []string
		}{
			"set plans":      {set: map[string]any{"owner": "x", "plans": "p"}},
			"unset plans":    {unset: []string{"owner", "plans"}},
			"set x_tlc":      {set: map[string]any{"x_tlc": map[string]any{}}},
			"unset external": {unset: []string{"external_uid"}},
			"set due flag":   {set: map[string]any{"due_date_only": true}},
		} {
			in := map[string]any{"owner": "ana", "plans": []string{"a.md"}}
			got, err := MergeTrackMeta(in, tc.set, tc.unset)
			if !errors.Is(err, ErrReservedMetaKey) {
				t.Fatalf("%s: err = %v, want ErrReservedMetaKey", name, err)
			}
			if got != nil {
				t.Errorf("%s: result = %v, want nil on error", name, got)
			}
			if !reflect.DeepEqual(in, map[string]any{"owner": "ana", "plans": []string{"a.md"}}) {
				t.Errorf("%s: input mutated: %v", name, in)
			}
		}
	})

	t.Run("empty key and set+unset conflict rejected", func(t *testing.T) {
		if _, err := MergeTrackMeta(nil, map[string]any{"": "x"}, nil); err == nil {
			t.Error("empty key accepted")
		}
		if _, err := MergeTrackMeta(nil, map[string]any{"a": "1"}, []string{"a"}); err == nil {
			t.Error("same key set and unset accepted")
		}
	})

	t.Run("no-op keeps nil", func(t *testing.T) {
		got, err := MergeTrackMeta(nil, nil, nil)
		if err != nil || got != nil {
			t.Fatalf("got %v, %v; want nil, nil", got, err)
		}
	})
}

func TestValidateTrackMetaWrite(t *testing.T) {
	if err := ValidateTrackMetaWrite(map[string]any{"owner": "a"}, []string{"b"}); err != nil {
		t.Fatalf("valid write rejected: %v", err)
	}
	if err := ValidateTrackMetaWrite(map[string]any{"plans": "a"}, nil); !errors.Is(err, ErrReservedMetaKey) {
		t.Fatalf("err = %v, want ErrReservedMetaKey", err)
	}
}
