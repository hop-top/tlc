package uri

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// reNumeric matches a bare numeric string (e.g. "46", "0046").
var reNumeric = regexp.MustCompile(`^[0-9]+$`)

// NormalizeTaskID converts flexible task ID inputs into the canonical
// T-NNNN form stored in the database.
//
// Accepted forms:
//   - "T-0046"  → "T-0046"  (already canonical)
//   - "@T-0046" → "T-0046"  (strip leading @)
//   - "46"      → "T-0046"  (bare number)
//   - "0046"    → "T-0046"  (zero-padded number)
//
// Non-task strings (e.g. project/task URIs) are returned unchanged so
// the caller's existing URI routing logic continues to work.
func NormalizeTaskID(s string) string {
	// Strip a single leading "@".
	raw := strings.TrimPrefix(s, "@")

	// If now matches T-NNNN already, return canonical form.
	if strings.HasPrefix(raw, "T-") {
		suffix := strings.TrimPrefix(raw, "T-")
		if n, err := strconv.Atoi(suffix); err == nil {
			return fmt.Sprintf("T-%04d", n)
		}
		// Non-numeric suffix – return as-is (don't mangle unknown formats).
		return raw
	}

	// Bare numeric: "46" or "0046".
	if reNumeric.MatchString(raw) {
		n, _ := strconv.Atoi(raw) //nolint:errcheck // guarded by reNumeric.MatchString
		return fmt.Sprintf("T-%04d", n)
	}

	// Anything else (e.g. "project/task", "tlc://…") – pass through unchanged.
	return s
}
