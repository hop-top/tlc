package core

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestIsReservedMetaKey(t *testing.T) {
	reserved := []string{
		"blocked_by", "blocked_by_plan", "blocked_by_recipe",
		"blocked_by_unresolved", "blocked_by_cross_project",
		"eva", "priority_source", "priority_rule", "last_sync_hash",
		"needs_push", "blocked_from", "external_uid", "x_tlc",
		"recipe", "recipe_version", "recipe_hash", "subject",
		"origin_system", "origin_id", "origin_key", "origin_url", "origin_anything",
	}
	for _, k := range reserved {
		if !IsReservedMetaKey(k) {
			t.Errorf("IsReservedMetaKey(%q) = false, want true", k)
		}
	}
	for _, k := range []string{"domain", "owner", "links", "requirements", "origin", "Eva"} {
		if IsReservedMetaKey(k) {
			t.Errorf("IsReservedMetaKey(%q) = true, want false", k)
		}
	}
}

//nolint:gocognit,gocyclo // one subtest per merge rule; the nesting is the closures
func TestMergeUserMeta(t *testing.T) {
	t.Run("sets into nil map", func(t *testing.T) {
		got, err := MergeUserMeta(nil, map[string]any{"owner": "ana"}, nil)
		if err != nil {
			t.Fatal(err)
		}
		want := map[string]any{"owner": "ana"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %v, want %v", got, want)
		}
	})

	t.Run("merge leaves other keys, user and internal", func(t *testing.T) {
		in := map[string]any{
			"blocked_by": []string{"T-0001"}, "eva": []string{"x"},
			"origin_id": "9", "keep": "me", "owner": "old",
		}
		got, err := MergeUserMeta(in, map[string]any{"owner": "new", "added": 1.0}, nil)
		if err != nil {
			t.Fatal(err)
		}
		want := map[string]any{
			"blocked_by": []string{"T-0001"}, "eva": []string{"x"},
			"origin_id": "9", "keep": "me", "owner": "new", "added": 1.0,
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %v, want %v", got, want)
		}
	})

	t.Run("unset removes only named keys, missing key is no-op", func(t *testing.T) {
		in := map[string]any{"a": "1", "b": "2", "eva": []string{"x"}}
		got, err := MergeUserMeta(in, nil, []string{"a", "nope"})
		if err != nil {
			t.Fatal(err)
		}
		want := map[string]any{"b": "2", "eva": []string{"x"}}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %v, want %v", got, want)
		}
	})

	t.Run("RFC 7396: null removes, nested objects merge", func(t *testing.T) {
		in := map[string]any{
			"gone": "x",
			"cfg":  map[string]any{"a": "1", "b": "2"},
		}
		patch := map[string]any{
			"gone": nil,
			"cfg":  map[string]any{"b": nil, "c": "3"},
			"new":  map[string]any{"deep": map[string]any{"k": nil, "j": true}},
		}
		got, err := MergeUserMeta(in, patch, nil)
		if err != nil {
			t.Fatal(err)
		}
		want := map[string]any{
			"cfg": map[string]any{"a": "1", "c": "3"},
			"new": map[string]any{"deep": map[string]any{"j": true}},
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %v, want %v", got, want)
		}
	})

	t.Run("RFC 7396: non-object value replaces", func(t *testing.T) {
		in := map[string]any{"cfg": map[string]any{"a": "1"}, "l": []any{"x"}}
		got, err := MergeUserMeta(in, map[string]any{"cfg": "flat", "l": []any{"y", "z"}}, nil)
		if err != nil {
			t.Fatal(err)
		}
		want := map[string]any{"cfg": "flat", "l": []any{"y", "z"}}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %v, want %v", got, want)
		}
	})

	t.Run("reserved key rejected on set and unset, nothing applied", func(t *testing.T) {
		for _, tc := range []struct {
			name  string
			set   map[string]any
			unset []string
		}{
			{"set eva", map[string]any{"ok": "1", "eva": "x"}, nil},
			{"set null reserved", map[string]any{"ok": "1", "blocked_by": nil}, nil},
			{"set origin prefix", map[string]any{"origin_url": "u"}, nil},
			{"unset priority_source", nil, []string{"ok", "priority_source"}},
			{"unset blocked_by_plan", nil, []string{"blocked_by_plan"}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				in := map[string]any{"ok": "0", "eva": []string{"x"}, "priority_source": "manual"}
				snapshot := map[string]any{"ok": "0", "eva": []string{"x"}, "priority_source": "manual"}
				got, err := MergeUserMeta(in, tc.set, tc.unset)
				if !errors.Is(err, ErrReservedMetaKey) {
					t.Fatalf("err = %v, want ErrReservedMetaKey", err)
				}
				if !reflect.DeepEqual(in, snapshot) {
					t.Fatalf("input mutated on error: %v", in)
				}
				if got != nil {
					t.Fatalf("got %v on error, want nil", got)
				}
			})
		}
	})

	t.Run("reserved error names the key", func(t *testing.T) {
		_, err := MergeUserMeta(nil, map[string]any{"eva": "x"}, nil)
		if err == nil || !strings.Contains(err.Error(), `"eva"`) {
			t.Fatalf("err = %v, want message naming \"eva\"", err)
		}
	})

	t.Run("nested key named like a reserved key is allowed", func(t *testing.T) {
		got, err := MergeUserMeta(nil, map[string]any{"cfg": map[string]any{"eva": "x"}}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, map[string]any{"cfg": map[string]any{"eva": "x"}}) {
			t.Fatalf("got %v", got)
		}
	})

	t.Run("empty key rejected", func(t *testing.T) {
		if _, err := MergeUserMeta(nil, map[string]any{"": "x"}, nil); err == nil {
			t.Fatal("empty set key accepted")
		}
		if _, err := MergeUserMeta(nil, nil, []string{""}); err == nil {
			t.Fatal("empty unset key accepted")
		}
	})

	t.Run("same key set and unset rejected", func(t *testing.T) {
		in := map[string]any{"a": "1"}
		if _, err := MergeUserMeta(in, map[string]any{"a": "2"}, []string{"a"}); err == nil {
			t.Fatal("conflicting set+unset accepted")
		}
		if in["a"] != "1" {
			t.Fatalf("input mutated: %v", in)
		}
	})

	t.Run("no-op write leaves nil map nil", func(t *testing.T) {
		got, err := MergeUserMeta(nil, nil, nil)
		if err != nil || got != nil {
			t.Fatalf("got %v, %v; want nil, nil", got, err)
		}
	})
}
