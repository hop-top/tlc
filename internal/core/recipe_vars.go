package core

import (
	"fmt"
	"sort"
	"strings"
)

// ParseVarFlags parses repeated --var flags into a map. Each flag holds one
// or more comma-separated key=value pairs; the first "=" splits key from
// value, so values may contain "=" but not ",".
func ParseVarFlags(flags []string) (map[string]string, error) {
	out := make(map[string]string)
	for _, flag := range flags {
		for _, pair := range strings.Split(flag, ",") {
			key, val, ok := strings.Cut(pair, "=")
			if !ok || key == "" {
				return nil, fmt.Errorf("invalid --var %q: want key=value[,key=value]", flag)
			}
			out[key] = val
		}
	}
	return out, nil
}

// ResolveRecipeVars checks provided against the recipe's declared vars and
// fills in defaults. Unknown keys are rejected before required ones are
// checked so a typo reads as "unknown", not "missing". Defaults keep their
// declared type; provided values are strings.
func ResolveRecipeVars(r *Recipe, provided map[string]string) (map[string]any, error) {
	for k := range provided {
		if _, ok := r.Vars[k]; !ok {
			return nil, fmt.Errorf("unknown var %q for recipe %s; declare it under vars", k, r.Name)
		}
	}
	names := make([]string, 0, len(r.Vars))
	for k := range r.Vars {
		names = append(names, k)
	}
	sort.Strings(names)

	resolved := make(map[string]any, len(names))
	var missing []string
	for _, k := range names {
		def := r.Vars[k]
		switch {
		case provided[k] != "" || hasKey(provided, k):
			resolved[k] = provided[k]
		case def.Default != nil:
			resolved[k] = def.Default
		case def.Required:
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required var(s) for recipe %s: %s; pass with --var %s=<value>",
			r.Name, strings.Join(missing, ", "), missing[0])
	}
	return resolved, nil
}

func hasKey(m map[string]string, k string) bool {
	_, ok := m[k]
	return ok
}
