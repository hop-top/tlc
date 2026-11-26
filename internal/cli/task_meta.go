package cli

import (
	"fmt"

	"hop.top/kit/go/runtime/domain"
)

// metaWriteError classes a refused user meta write as a validation error
// (HTTP 422, CLI usage failure) while keeping core's sentinel matchable.
func metaWriteError(err error) error {
	return fmt.Errorf("%w: %w", domain.ErrValidation, err)
}

// metaFromFlags converts the string values of a repeated --meta k=v flag
// into a meta write document. CLI values are always strings: no JSON
// parsing and no dotted-path nesting, so what is typed is what is stored
// and what a `meta.<key>` filter compares against. Typed or nested values
// go through the REST API's JSON body.
func metaFromFlags(kv map[string]string) map[string]any {
	if len(kv) == 0 {
		return nil
	}
	out := make(map[string]any, len(kv))
	for k, v := range kv {
		out[k] = v
	}
	return out
}
