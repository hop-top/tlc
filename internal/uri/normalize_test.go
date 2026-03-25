package uri

import (
	"testing"
)

func TestNormalizeTaskID(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		// Already canonical – must be returned unchanged.
		{"T-0046", "T-0046"},
		{"T-0001", "T-0001"},
		{"T-0100", "T-0100"},
		{"T-9999", "T-9999"},

		// Strip leading @.
		{"@T-0046", "T-0046"},
		{"@T-0001", "T-0001"},

		// Bare numeric.
		{"46", "T-0046"},
		{"1", "T-0001"},
		{"100", "T-0100"},
		{"9999", "T-9999"},

		// Zero-padded numeric.
		{"0046", "T-0046"},
		{"0001", "T-0001"},
		{"00046", "T-0046"},

		// Re-normalise non-canonical T- prefix.
		{"T-46", "T-0046"},
		{"T-1", "T-0001"},

		// Pass-through: project/task URI shorthand.
		{"myproject/T-0046", "myproject/T-0046"},

		// Pass-through: full URI.
		{"tlc://myproject/T-0046", "tlc://myproject/T-0046"},

		// Pass-through: non-numeric T- suffix (unknown format).
		{"T-abc", "T-abc"},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got := NormalizeTaskID(tc.input)
			if got != tc.want {
				t.Errorf("NormalizeTaskID(%q) = %q; want %q", tc.input, got, tc.want)
			}
		})
	}
}
