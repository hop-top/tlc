package cli

import "testing"

// TestStatusSnapshot_ExtraCounts pins the review and blocked buckets:
// printed only when non-empty, in review-then-blocked order.
func TestStatusSnapshot_ExtraCounts(t *testing.T) {
	cases := []struct {
		name string
		snap statusSnapshot
		want string
	}{
		{"none", statusSnapshot{}, ""},
		{"review", statusSnapshot{inReview: 2}, ", 2 in review"},
		{"blocked", statusSnapshot{blocked: 1}, ", 1 blocked"},
		{"both", statusSnapshot{inReview: 2, blocked: 1}, ", 2 in review, 1 blocked"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.snap.extraCounts(); got != tc.want {
				t.Errorf("extraCounts() = %q, want %q", got, tc.want)
			}
		})
	}
}
