package core

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// flowInputPlaceholder matches {{name}} or {{ name }} where name is a valid
// identifier (letters, digits, underscore). Whitespace inside the braces is
// tolerated to allow {{ track_id }} as well as {{track_id}}.
var flowInputPlaceholder = regexp.MustCompile(`\{\{\s*([A-Za-z_][A-Za-z0-9_]*)\s*\}\}`)

// ResolveFlowInputs validates and resolves the input variables for a flow run.
// It enforces required inputs declared in flow.Config.Inputs and applies
// declared defaults when a value is not provided. Unknown keys are rejected.
//
// The returned map is safe to mutate by the caller.
func ResolveFlowInputs(flow *Flow, provided map[string]string) (map[string]any, error) {
	resolved := make(map[string]any)

	var defs map[string]FlowInputDef
	if flow != nil && flow.Config != nil {
		defs = flow.Config.Inputs
	}

	// Reject unknown keys before validating required/defaults so users get a
	// clear "unknown input" error rather than a confusing missing-required one.
	for k := range provided {
		if _, ok := defs[k]; !ok {
			return nil, fmt.Errorf("unknown input %q for flow %s; declare it under config.inputs", k, flowName(flow))
		}
	}

	// Apply provided values, falling back to declared defaults, and check
	// required inputs are satisfied. Iterate in sorted order so error messages
	// are deterministic.
	keys := make([]string, 0, len(defs))
	for k := range defs {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var missing []string
	for _, k := range keys {
		def := defs[k]
		if v, ok := provided[k]; ok {
			resolved[k] = v
			continue
		}
		if def.Default != nil {
			resolved[k] = def.Default
			continue
		}
		if def.Required {
			missing = append(missing, k)
		}
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf(
			"missing required input(s) for flow %s: %s; pass with --var %s=<value>",
			flowName(flow), strings.Join(missing, ", "), missing[0],
		)
	}

	return resolved, nil
}

// SubstituteFlowInputs replaces {{name}} placeholders in s with values from
// inputs. Unknown placeholders are left untouched (so non-input templating,
// e.g. shell ${VAR}, is not accidentally consumed).
func SubstituteFlowInputs(s string, inputs map[string]any) string {
	if s == "" || len(inputs) == 0 {
		return s
	}
	return flowInputPlaceholder.ReplaceAllStringFunc(s, func(match string) string {
		sub := flowInputPlaceholder.FindStringSubmatch(match)
		if len(sub) != 2 {
			return match
		}
		v, ok := inputs[sub[1]]
		if !ok {
			return match
		}
		return fmt.Sprintf("%v", v)
	})
}

// ParseFlowVarFlags parses repeated --var key=value strings into a map.
// Returns an error on malformed entries (missing '=' or empty key).
func ParseFlowVarFlags(vars []string) (map[string]string, error) {
	out := make(map[string]string, len(vars))
	for _, raw := range vars {
		idx := strings.IndexByte(raw, '=')
		if idx <= 0 {
			return nil, fmt.Errorf("invalid --var %q; expected key=value", raw)
		}
		key := raw[:idx]
		val := raw[idx+1:]
		out[key] = val
	}
	return out, nil
}

func flowName(flow *Flow) string {
	if flow == nil {
		return "<nil>"
	}
	if flow.ID != "" {
		return flow.ID
	}
	return flow.Name
}
