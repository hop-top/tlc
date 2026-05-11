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

func TestDisplayTimeLocalLiteralResolvesToTimeLocal(t *testing.T) {
	// The documented default in the config spec is `ui.timezone: local`.
	// LoadTimezone must accept the literal "local" string, return
	// time.Local, and avoid the invalid-tz warning path that
	// TestDisplayTimeInvalidTimezoneFallsBackToLocal exercises.
	resetForTest(t, "local")
	loc := Resolve()
	if loc != time.Local {
		t.Fatalf("ui.timezone=local should resolve to time.Local, got %v", loc)
	}
	// And no pending warning should have been buffered, since "local"
	// is a valid value (distinct from an invalid IANA name).
	tzMutex.Lock()
	warn := tzWarn
	tzMutex.Unlock()
	if warn != "" {
		t.Fatalf("ui.timezone=local should not buffer a warning, got %q", warn)
	}
	// Output must round-trip through RFC3339 like the other cases.
	instant := time.Date(2026, 6, 15, 14, 30, 0, 0, time.UTC)
	got := DisplayTime(instant, time.RFC3339)
	if _, err := time.Parse(time.RFC3339, got); err != nil {
		t.Fatalf("local-literal output not RFC3339-parseable: %q (%v)", got, err)
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

// TestDisplayTimeRelative covers the humanised render path used by
// table-format columns (T-1384). Spec §6 says default table output
// renders timestamps via util.RelativeTime / util.HumanDuration; this
// test pins that contract through the displaytime façade.
func TestDisplayTimeRelative(t *testing.T) {
	resetForTest(t, "UTC")

	// RelativeTime computes `time.Since(t)` at call time inside the
	// helper, NOT at the time the test prepares the input. If the gap
	// between preparing inputs and calling DisplayTimeRelative crosses
	// an integer-bucket boundary (e.g. due to CI lag, GC pause), pinned
	// assertions like "5m ago" can flake to "6m ago" and vice versa.
	//
	// Each subtest captures `now` itself, immediately before constructing
	// its input. The window between input creation and helper invocation
	// is now microseconds — well under any bucket.
	//
	// For the future-direction cases, HumanDuration truncates downward,
	// so `now.Add(3h)` reads back as `2h something` ("in 2h"). Add a
	// small fudge so the truncation lands on N.
	const fudge = 100 * time.Millisecond

	cases := []struct {
		name   string
		zero   bool          // when true, input is time.Time{} (skip offset)
		offset time.Duration // applied to a fresh now in the subtest
		want   string
	}{
		{"zero_time_empty", true, 0, ""},
		{"just_now", false, -30 * time.Second, "just now"},
		{"five_minutes_ago", false, -5 * time.Minute, "5m ago"},
		{"two_hours_ago", false, -2 * time.Hour, "2h ago"},
		// yesterday window is [24h, 48h) since now.
		{"yesterday", false, -30 * time.Hour, "yesterday"},
		// 48h or more renders as Nd ago.
		{"three_days_ago", false, -3 * 24 * time.Hour, "3d ago"},
		// Future cases use fudge to land on N after downward truncation.
		{"in_three_hours", false, 3*time.Hour + fudge, "in 3h"},
		{"in_two_days", false, 2*24*time.Hour + fudge, "in 2d"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			var in time.Time
			if !tc.zero {
				in = time.Now().Add(tc.offset)
			}
			got := DisplayTimeRelative(in)
			if got != tc.want {
				t.Fatalf("DisplayTimeRelative(now%+v) = %q, want %q",
					tc.offset, got, tc.want)
			}
		})
	}
}

// TestDisplayTimePtrRelativeNil mirrors the DisplayTimePtr nil
// contract: a nil *time.Time renders as "" so callers can use the
// helper unconditionally on optional fields without an IsZero guard.
func TestDisplayTimePtrRelativeNil(t *testing.T) {
	resetForTest(t, "UTC")
	got := DisplayTimePtrRelative(nil)
	if got != "" {
		t.Fatalf("DisplayTimePtrRelative(nil) = %q, want empty", got)
	}
}

// TestDisplayTimePtrRelativeNonNil confirms the *time.Time variant
// delegates to DisplayTimeRelative when the pointer is set.
func TestDisplayTimePtrRelativeNonNil(t *testing.T) {
	resetForTest(t, "UTC")
	in := time.Now().Add(-2 * time.Hour)
	got := DisplayTimePtrRelative(&in)
	if got != "2h ago" {
		t.Fatalf("DisplayTimePtrRelative(2h ago) = %q, want %q", got, "2h ago")
	}
}
