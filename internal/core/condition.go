package core

import (
	"fmt"
	"strconv"
	"strings"
)

// CondEnv is what a condition expression can read.
//
//   - Results: upstream task results keyed by step id, read as
//     results.<step>.<dotted path> — the executor's `when:` input.
//   - Vars: recipe vars, read as vars.<name>.
//   - Inputs: the legacy flow inputs, read as inputs.<name>; a bare
//     identifier resolves in Inputs first, then Vars.
type CondEnv struct {
	Inputs  map[string]any
	Vars    map[string]any
	Results map[string]map[string]any
}

// EvalCondition evaluates an inputs-only expression. It is the legacy
// flow entry point and delegates to EvalConditionEnv.
func EvalCondition(expr string, inputs map[string]any) (bool, error) {
	return EvalConditionEnv(expr, CondEnv{Inputs: inputs})
}

// EvalConditionEnv evaluates a condition against env.
//
// Grammar (intentionally tiny — no expr/CEL dependency):
//
//	expr   := lhs OP rhs
//	lhs    := "results." STEP "." PATH  |  "vars." IDENT  |  "inputs." IDENT  |  IDENT
//	OP     := "=="  |  "!="  |  "<"  |  "<="  |  ">"  |  ">="
//	rhs    := "'" STRING "'"  |  '"' STRING '"'  |  NUMBER  |  true  |  false  |  IDENT
//
// Whitespace around tokens is tolerated. Empty expr returns true (no
// gate). Equality compares the value's %v rendering with the literal, so
// numbers and booleans match their unquoted or quoted spelling. Ordering
// operators require both sides to be numbers. An unknown step, path or
// key is an error so callers fail loudly rather than silently skip.
func EvalConditionEnv(expr string, env CondEnv) (bool, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return true, nil
	}

	op, opIdx := findOperator(expr)
	if opIdx < 0 {
		return false, fmt.Errorf("invalid condition %q: expected one of == != < <= > >=", expr)
	}
	lhsRaw := strings.TrimSpace(expr[:opIdx])
	rhsRaw := strings.TrimSpace(expr[opIdx+len(op):])
	if lhsRaw == "" || rhsRaw == "" {
		return false, fmt.Errorf("invalid condition %q: empty operand", expr)
	}

	val, err := env.lookup(lhsRaw)
	if err != nil {
		return false, fmt.Errorf("condition %q: %w", expr, err)
	}
	rhsVal := unquote(rhsRaw)

	switch op {
	case "==":
		return fmt.Sprintf("%v", val) == rhsVal, nil
	case "!=":
		return fmt.Sprintf("%v", val) != rhsVal, nil
	}

	lhsNum, ok := toFloat(val)
	if !ok {
		return false, fmt.Errorf("condition %q: %s is not a number (%v), cannot compare with %s", expr, lhsRaw, val, op)
	}
	rhsNum, err := strconv.ParseFloat(rhsVal, 64)
	if err != nil {
		return false, fmt.Errorf("condition %q: %q is not a number, cannot compare with %s", expr, rhsVal, op)
	}
	switch op {
	case "<":
		return lhsNum < rhsNum, nil
	case "<=":
		return lhsNum <= rhsNum, nil
	case ">":
		return lhsNum > rhsNum, nil
	default: // ">="
		return lhsNum >= rhsNum, nil
	}
}

// lookup resolves an lhs reference in the environment.
func (env CondEnv) lookup(ref string) (any, error) {
	switch {
	case strings.HasPrefix(ref, "results."):
		return env.lookupResult(strings.TrimPrefix(ref, "results."))
	case strings.HasPrefix(ref, "vars."):
		return lookupKey(env.Vars, strings.TrimPrefix(ref, "vars."), "var")
	case strings.HasPrefix(ref, "inputs."):
		return lookupKey(env.Inputs, strings.TrimPrefix(ref, "inputs."), "input")
	}
	if v, ok := env.Inputs[ref]; ok {
		return v, nil
	}
	if v, ok := env.Vars[ref]; ok {
		return v, nil
	}
	return nil, fmt.Errorf("references unknown key %q", ref)
}

// lookupResult resolves "<step>.<dotted path>" into Results.
func (env CondEnv) lookupResult(path string) (any, error) {
	step, rest, ok := strings.Cut(path, ".")
	if !ok || step == "" || rest == "" {
		return nil, fmt.Errorf("results reference %q must be results.<step>.<field>", path)
	}
	result, found := env.Results[step]
	if !found {
		return nil, fmt.Errorf("references unknown step %q", step)
	}
	var cur any = result
	for _, part := range strings.Split(rest, ".") {
		m, isMap := cur.(map[string]any)
		if !isMap {
			return nil, fmt.Errorf("results.%s.%s: %q is not an object", step, rest, part)
		}
		next, ok := m[part]
		if !ok {
			return nil, fmt.Errorf("references unknown field %q of step %q", part, step)
		}
		cur = next
	}
	return cur, nil
}

func lookupKey(m map[string]any, key, kind string) (any, error) {
	v, ok := m[key]
	if !ok {
		return nil, fmt.Errorf("references unknown %s key %q", kind, key)
	}
	return v, nil
}

// toFloat widens any numeric value (including a numeric string) to float64.
func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case float32:
		return float64(n), true
	case float64:
		return n, true
	case string:
		f, err := strconv.ParseFloat(n, 64)
		return f, err == nil
	}
	return 0, false
}

// conditionOperators in matching order: two-character operators first so
// "<=" is never read as "<" followed by "= …".
var conditionOperators = []string{"==", "!=", "<=", ">=", "<", ">"}

// findOperator returns the earliest operator in expr and its byte index;
// at equal positions the longer operator wins.
func findOperator(expr string) (string, int) {
	bestOp, bestIdx := "", -1
	for _, op := range conditionOperators {
		idx := strings.Index(expr, op)
		if idx < 0 {
			continue
		}
		if bestIdx < 0 || idx < bestIdx {
			bestOp, bestIdx = op, idx
		}
	}
	return bestOp, bestIdx
}

// unquote strips a single matching pair of single or double quotes. Bare
// identifiers and numbers are returned as-is.
func unquote(s string) string {
	if len(s) >= 2 {
		first, last := s[0], s[len(s)-1]
		if (first == '\'' && last == '\'') || (first == '"' && last == '"') {
			return s[1 : len(s)-1]
		}
	}
	return s
}
