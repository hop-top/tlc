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

// warnDisplayTimezone forwards to displaytime.WarnInvalid so root.go
// can keep a stable internal-cli symbol.
func warnDisplayTimezone() {
	displaytime.WarnInvalid()
}
