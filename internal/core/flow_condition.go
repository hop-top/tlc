package core

import (
	"fmt"
	"strings"
)

// EvalCondition evaluates a Step.Condition expression against flow inputs.
//
// Grammar (intentionally tiny — no expr/CEL dependency):
//
//	expr   := lhs OP rhs
//	lhs    := "inputs." IDENT  |  IDENT
//	OP     := "=="  |  "!="
//	rhs    := "'" STRING "'"  |  '"' STRING '"'  |  IDENT
//
// Whitespace around tokens is tolerated. Empty `expr` returns true (no gate).
// Unknown input keys return an error so callers can fail loudly rather than
// silently skip — matches the docstring contract in task-flow-spec.
//
// Examples:
//
//	inputs.target == 'staging'
//	target != "production"
//	env == staging
func EvalCondition(expr string, inputs map[string]any) (bool, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return true, nil
	}

	op, opIdx := findOperator(expr)
	if opIdx < 0 {
		return false, fmt.Errorf("invalid condition %q: expected '==' or '!='", expr)
	}

	lhsRaw := strings.TrimSpace(expr[:opIdx])
	rhsRaw := strings.TrimSpace(expr[opIdx+len(op):])

	if lhsRaw == "" || rhsRaw == "" {
		return false, fmt.Errorf("invalid condition %q: empty operand", expr)
	}

	key := strings.TrimPrefix(lhsRaw, "inputs.")
	val, ok := inputs[key]
	if !ok {
		return false, fmt.Errorf("condition %q references unknown input key %q", expr, key)
	}

	rhsVal := unquote(rhsRaw)

	got := fmt.Sprintf("%v", val)
	switch op {
	case "==":
		return got == rhsVal, nil
	case "!=":
		return got != rhsVal, nil
	default:
		// unreachable; findOperator guarantees one of the two.
		return false, fmt.Errorf("invalid condition %q: unsupported operator %q", expr, op)
	}
}

// findOperator returns the matched operator and its byte index. `!=` wins over
// `==` only when it appears first; the parser does not support compound exprs.
func findOperator(expr string) (string, int) {
	neIdx := strings.Index(expr, "!=")
	eqIdx := strings.Index(expr, "==")
	switch {
	case neIdx < 0 && eqIdx < 0:
		return "", -1
	case neIdx < 0:
		return "==", eqIdx
	case eqIdx < 0:
		return "!=", neIdx
	case neIdx < eqIdx:
		return "!=", neIdx
	default:
		return "==", eqIdx
	}
}

// unquote strips a single matching pair of single or double quotes. Bare
// identifiers (e.g. `staging`) are returned as-is.
func unquote(s string) string {
	if len(s) >= 2 {
		first, last := s[0], s[len(s)-1]
		if (first == '\'' && last == '\'') || (first == '"' && last == '"') {
			return s[1 : len(s)-1]
		}
	}
	return s
}
