package uri

import (
	"regexp"
	"strconv"
	"strings"

	"hop.top/tlc/internal/core"
)

// reNumeric matches a bare numeric string (e.g. "46", "0046").
var reNumeric = regexp.MustCompile(`^[0-9]+$`)

// NormalizeTaskID converts flexible task ID inputs into the canonical
// T-NNNN form stored in the database. The "T-" prefix is matched
// case-insensitively so "t-0046" normalises identically to "T-0046".
//
// Accepted forms:
//   - "T-0046"  → "T-0046"  (already canonical)
//   - "t-0046"  → "T-0046"  (lowercase prefix)
//   - "@T-0046" → "T-0046"  (strip leading @)
//   - "46"      → "T-0046"  (bare number)
//   - "0046"    → "T-0046"  (zero-padded number)
//
// Non-task strings (e.g. project/task URIs) are returned unchanged so
// the caller's existing URI routing logic continues to work.
func NormalizeTaskID(s string) string {
	// Strip a single leading "@".
	raw := strings.TrimPrefix(s, "@")

	// If now matches T-NNNN already (case-insensitive), return canonical form.
	if len(raw) >= 2 && (raw[0] == 'T' || raw[0] == 't') && raw[1] == '-' {
		suffix := raw[2:]
		if n, err := strconv.Atoi(suffix); err == nil {
			return core.FormatTaskSeq(int64(n))
		}
		// Non-numeric suffix – return as-is (don't mangle unknown formats).
		return raw
	}

	// Bare numeric: "46" or "0046".
	if reNumeric.MatchString(raw) {
		n, _ := strconv.Atoi(raw) //nolint:errcheck // guarded by reNumeric.MatchString
		return core.FormatTaskSeq(int64(n))
	}

	// Anything else (e.g. "project/task", "tlc://…") – pass through unchanged.
	return s
}
