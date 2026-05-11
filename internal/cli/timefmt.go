package cli

import (
	"time"

	"hop.top/tlc/internal/displaytime"
)

// Layout aliases — see internal/displaytime for the canonical values.
const (
	LayoutRFC3339    = displaytime.LayoutRFC3339
	LayoutDate       = displaytime.LayoutDate
	LayoutDateTime   = displaytime.LayoutDateTime
	LayoutDateMinute = displaytime.LayoutDateMinute
	LayoutTimeOnly   = displaytime.LayoutTimeOnly
)

// DisplayTime renders t in the user's configured display timezone using
// layout. See internal/displaytime.DisplayTime for full semantics.
func DisplayTime(t time.Time, layout string) string {
	return displaytime.DisplayTime(t, layout)
}

// DisplayTimePtr is the *time.Time variant; nil renders as "".
func DisplayTimePtr(t *time.Time, layout string) string {
	return displaytime.DisplayTimePtr(t, layout)
}

// DisplayTimeRelative renders t humanised relative to now ("2h ago",
// "in 3d"). Use for table-format columns; JSON/YAML render paths keep
// DisplayTime with LayoutRFC3339. See displaytime.DisplayTimeRelative.
func DisplayTimeRelative(t time.Time) string {
	return displaytime.DisplayTimeRelative(t)
}

// DisplayTimePtrRelative is the *time.Time variant; nil renders as "".
func DisplayTimePtrRelative(t *time.Time) string {
	return displaytime.DisplayTimePtrRelative(t)
}

// warnDisplayTimezone forwards to displaytime.WarnInvalid so root.go
// can keep a stable internal-cli symbol.
func warnDisplayTimezone() {
	displaytime.WarnInvalid()
}
