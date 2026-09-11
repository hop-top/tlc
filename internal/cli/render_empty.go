package cli

import "reflect"

// normalizeEmptySlices rewrites nil slices to empty, non-nil slices so
// serialized output renders `[]` rather than `null`.
//
// A well-formed query matching zero rows is not an error, but Go's
// encoding/json renders a nil slice as `null`. Downstream consumers that
// iterate the result then break — `jq '.[]'` fails with "Cannot iterate
// over null", surfacing as a confusing nonzero exit in pipelines that
// look like tlc failures but are not.
//
// Storage layers return `var xs []*T` accumulators that stay nil on zero
// rows, so the defect reaches every list-shaped JSON payload. Fixing it
// at the render boundary covers all of them at once instead of patching
// query sites one at a time, and it also reaches nil slices nested one
// level deep inside map payloads (e.g. the `logs` field of `task show`).
//
// The value is returned unchanged when no rewrite applies.
func normalizeEmptySlices(data any) any {
	if data == nil {
		return data
	}
	v := reflect.ValueOf(data)
	switch v.Kind() {
	case reflect.Slice:
		if v.IsNil() {
			return reflect.MakeSlice(v.Type(), 0, 0).Interface()
		}
	case reflect.Map:
		// Only rewrite map[string]any payloads assembled by the CLI for
		// composite documents; rewriting arbitrary map types would need
		// a fresh map and risks surprising callers.
		if m, ok := data.(map[string]any); ok {
			for k, val := range m {
				m[k] = normalizeEmptySlices(val)
			}
		}
	}
	return data
}
