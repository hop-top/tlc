// Package displaytime is the single tlc path for rendering absolute
// time values to user-facing output. Every CLI and TUI render site that
// would otherwise call t.Format(...) MUST instead call DisplayTime /
// DisplayTimePtr from this package so timezone behaviour stays uniform
// and bound to cfg.UI.Timezone (see docs/temporal-spec-0.1.md §6).
//
// Storage-layer code is out of scope: writes/reads at the SQLite border
// stay in UTC RFC3339 per spec §4 and never call this package.
package displaytime

import (
	"sync"
	"time"

	"charm.land/log/v2"
	"github.com/spf13/viper"
	"hop.top/kit/go/core/util"
)

// Layout constants for the display path. These mirror the table in
// docs/temporal-spec-0.1.md §6.
const (
	// LayoutRFC3339 is the JSON/YAML layout for absolute timestamps.
	LayoutRFC3339 = time.RFC3339
	// LayoutDate is the date-only layout used by the `tls` format and
	// the table column for due dates (where time-of-day adds noise).
	LayoutDate = "2006-01-02"
	// LayoutDateTime is the table layout for `created_at`, `updated_at`,
	// and other timestamps that need second-level resolution.
	LayoutDateTime = "2006-01-02 15:04:05"
	// LayoutDateMinute trims seconds. Used by reminder views and the
	// `log` short-form table.
	LayoutDateMinute = "2006-01-02 15:04"
	// LayoutTimeOnly is the same-day fallback used by `task remind`.
	LayoutTimeOnly = "15:04"
)

// cached display location, resolved once per process from viper's
// `ui.timezone` key. Tests reset via Reset.
var (
	tzOnce  sync.Once
	tzLoc   *time.Location
	tzWarn  string // non-empty when the configured value was rejected
	tzMutex sync.Mutex
)

// Resolve returns the cached *time.Location for ui.timezone, falling
// back to time.Local on invalid IANA names. Subsequent calls return the
// cached value with no further lookup.
//
// The fallback is silent at the call site; one-time warning is emitted
// via WarnInvalid (typically from setupLogging) so we never spam on
// every render. Spec §6.
func Resolve() *time.Location {
	tzOnce.Do(func() {
		name := viper.GetString("ui.timezone")
		loc, err := util.LoadTimezone(name)
		if err != nil {
			tzLoc = time.Local
			tzWarn = name
			return
		}
		if loc == nil {
			loc = time.Local
		}
		tzLoc = loc
	})
	tzMutex.Lock()
	defer tzMutex.Unlock()
	return tzLoc
}

// WarnInvalid emits a one-time warning when ui.timezone was configured
// to an invalid value. Safe to call repeatedly; only the first call
// after Resolve detected the bad name produces output. Wire from
// setupLogging() so the warning lands through the configured sink and
// never on every render.
func WarnInvalid() {
	_ = Resolve()
	tzMutex.Lock()
	bad := tzWarn
	tzWarn = ""
	tzMutex.Unlock()
	if bad != "" {
		log.Warn(
			"invalid ui.timezone; falling back to system local",
			"value", bad,
		)
	}
}

// Reset clears the cached *time.Location. Test-only seam: production
// callers go through Resolve, which memoises via sync.Once.
func Reset() {
	tzMutex.Lock()
	defer tzMutex.Unlock()
	tzOnce = sync.Once{}
	tzLoc = nil
	tzWarn = ""
}

// DisplayTime renders t in the display timezone (cfg.UI.Timezone via
// viper) using layout. A zero time renders as "" so callers can use it
// unconditionally on optional fields without a guard.
//
// This is the only path absolute timestamps should take to user-facing
// output. See docs/temporal-spec-0.1.md §6.
func DisplayTime(t time.Time, layout string) string {
	if t.IsZero() {
		return ""
	}
	return util.FormatInZone(t, Resolve(), layout)
}

// DisplayTimePtr is the *time.Time variant. Nil renders as "" per spec
// §6 ("Nil renders empty"). Used at every render site that holds an
// optional temporal field (DueAt, RemindAt, StaleFiredAt, EndedAt).
func DisplayTimePtr(t *time.Time, layout string) string {
	if t == nil {
		return ""
	}
	return DisplayTime(*t, layout)
}
