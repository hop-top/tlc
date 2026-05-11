package displaytime

import (
	"testing"
	"time"

	"github.com/spf13/viper"
)

// resetForTest clears displaytime state and viper's ui.timezone key
// between sub-tests so each case starts from a known baseline.
func resetForTest(tb testing.TB, name string) {
	tb.Helper()
	viper.Set("ui.timezone", name)
	Reset()
}

func TestDisplayTimeZeroRendersEmpty(t *testing.T) {
	resetForTest(t, "UTC")
	got := DisplayTime(time.Time{}, LayoutRFC3339)
	if got != "" {
		t.Fatalf("zero time should render as empty string, got %q", got)
	}
}

func TestDisplayTimePtrNilRendersEmpty(t *testing.T) {
	resetForTest(t, "UTC")
	got := DisplayTimePtr(nil, LayoutRFC3339)
	if got != "" {
		t.Fatalf("nil *time.Time should render as empty string, got %q", got)
	}
}

func TestDisplayTimeRoundTripAcrossZones(t *testing.T) {
	// Pick an unambiguous summer instant where every tested zone has a
	// distinct offset from UTC, so we can compare formatted output
	// without DST edge cases.
	instant := time.Date(2026, 6, 15, 14, 30, 0, 0, time.UTC)

	cases := []struct {
		name   string
		tz     string
		layout string
		want   string
	}{
		{"utc/rfc3339", "UTC", time.RFC3339, "2026-06-15T14:30:00Z"},
		{"new_york/rfc3339", "America/New_York", time.RFC3339, "2026-06-15T10:30:00-04:00"},
		{"tokyo/rfc3339", "Asia/Tokyo", time.RFC3339, "2026-06-15T23:30:00+09:00"},
		{"utc/date", "UTC", LayoutDate, "2026-06-15"},
		{"tokyo/date", "Asia/Tokyo", LayoutDate, "2026-06-15"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			resetForTest(t, tc.tz)
			got := DisplayTime(instant, tc.layout)
			if got != tc.want {
				t.Fatalf("tz=%s layout=%s: want %q, got %q",
					tc.tz, tc.layout, tc.want, got)
			}
		})
	}
}

func TestDisplayTimeLocalFallbackUnsetTimezone(t *testing.T) {
	// Empty ui.timezone resolves to time.Local. We cannot assert an
	// exact string without knowing the test host's tz, but we can
	// assert: (1) output is non-empty, (2) the resolver did not error
	// and fall through to UTC by accident — verifiable by rendering
	// the same instant in UTC and confirming results differ only when
	// host != UTC. To keep the test stable on UTC build hosts we just
	// confirm that the function returns something parseable.
	resetForTest(t, "")
	instant := time.Date(2026, 6, 15, 14, 30, 0, 0, time.UTC)
	got := DisplayTime(instant, time.RFC3339)
	if got == "" {
		t.Fatal("local fallback should still produce a non-empty render")
	}
	if _, err := time.Parse(time.RFC3339, got); err != nil {
		t.Fatalf("local fallback output not RFC3339-parseable: %q (%v)", got, err)
	}
}

func TestDisplayTimeInvalidTimezoneFallsBackToLocal(t *testing.T) {
	resetForTest(t, "Not/A_Real_Zone")
	loc := Resolve()
	if loc != time.Local {
		t.Fatalf("invalid tz should fall back to time.Local, got %v", loc)
	}
	// And output must still parse back as RFC3339.
	instant := time.Date(2026, 6, 15, 14, 30, 0, 0, time.UTC)
	got := DisplayTime(instant, time.RFC3339)
	if _, err := time.Parse(time.RFC3339, got); err != nil {
		t.Fatalf("invalid-tz fallback output not RFC3339-parseable: %q (%v)", got, err)
	}
}

func TestResolveIsMemoised(t *testing.T) {
	resetForTest(t, "America/New_York")
	first := Resolve()
	// Mutate viper without resetting; Resolve should return the cached
	// value, not re-read the key.
	viper.Set("ui.timezone", "Asia/Tokyo")
	second := Resolve()
	if first != second {
		t.Fatalf("Resolve should memoise; got %v then %v", first, second)
	}
	// After explicit Reset, the new value wins.
	Reset()
	third := Resolve()
	if third == first {
		t.Fatalf("Reset should re-resolve from viper; still got cached %v", third)
	}
}

func TestDisplayTimePtrNonNil(t *testing.T) {
	resetForTest(t, "UTC")
	instant := time.Date(2026, 6, 15, 14, 30, 0, 0, time.UTC)
	got := DisplayTimePtr(&instant, time.RFC3339)
	want := "2026-06-15T14:30:00Z"
	if got != want {
		t.Fatalf("DisplayTimePtr: want %q, got %q", want, got)
	}
}
