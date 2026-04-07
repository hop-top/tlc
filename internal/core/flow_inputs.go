package core

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"text/template"
)

// flowInputPlaceholder matches {{name}} or {{ name }} where name is a valid
// identifier (letters, digits, underscore). Whitespace inside the braces is
// tolerated to allow {{ track_id }} as well as {{track_id}}.
//
// Role: preprocessor. SubstituteFlowInputs rewrites known-key matches to
// text/template field access ({{.name}}) and wraps unknown matches in a
// text/template string-literal action ({{"{{foo}}"}}) so the engine emits
// them verbatim. Non-input templating (e.g. shell ${VAR}) is preserved
// because it doesn't match this regex at all.
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
// inputs using Go's text/template engine.
//
// Algorithm:
//  1. Preprocess: rewrite each {{name}} via one regex pass. Known keys (in
//     inputs) become text/template field access ({{.name}}). Unknown keys
//     are wrapped in a string-literal template action ({{"{{foo}}"}}) so
//     the engine emits them verbatim — a bare {{foo}} would fail
//     text/template.Parse with "function \"foo\" not defined".
//  2. Execute: parse and execute the preprocessed string via text/template
//     with Option("missingkey=zero") as defence-in-depth; preprocessing
//     ensures this never fires for the current YAML corpus.
//
// Unknown placeholder passthrough preserves non-input templating (e.g.
// shell ${VAR}, which doesn't match the regex and is untouched).
// Defensive: if template parsing or execution fails for any reason, the
// original string is returned unchanged.
func SubstituteFlowInputs(s string, inputs map[string]any) string {
	if s == "" || len(inputs) == 0 {
		return s
	}

	// Step 1: selective preprocessing. Rewrite known-key placeholders to
	// text/template field access ({{.name}}), and wrap unknown placeholders
	// in a literal-string template action ({{"{{foo}}"}}) so text/template
	// emits them verbatim instead of failing to parse {{foo}} as an action.
	preprocessed := flowInputPlaceholder.ReplaceAllStringFunc(s, func(match string) string {
		sub := flowInputPlaceholder.FindStringSubmatch(match)
		if len(sub) != 2 {
			return match
		}
		if _, ok := inputs[sub[1]]; ok {
			return "{{." + sub[1] + "}}"
		}
		// Unknown placeholder: emit as a literal string via template action.
		return `{{"` + match + `"}}`
	})

	// Step 2: execute via text/template. missingkey=zero is defence-in-depth;
	// preprocessing ensures no unknown keys reach this step.
	tmpl, err := template.New("flow-input").
		Option("missingkey=zero").
		Parse(preprocessed)
	if err != nil {
		return s
	}
	var buf strings.Builder
	if err := tmpl.Execute(&buf, inputs); err != nil {
		return s
	}
	return buf.String()
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
