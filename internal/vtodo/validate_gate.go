package vtodo

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	vstar "hop.top/vstar"
	"hop.top/vstar/validate"
)

// AllowedErrorCodes is the set of validate.SeverityError codes an
// export may carry and still pass ValidateExport. Every entry is a
// documented deviation from spec-vstar conformance; the list lives
// here, not in a comment, so a test can pin it and a reader can find
// it. See docs/VSTAR-CONFORMANCE.md, "Known deviations".
//
//   - VS040: a VTODO needs DUE, or STATUS=COMPLETED with COMPLETED.
//     tlc tasks and tracks are routinely open and undated; inventing
//     a DUE would be data the user never entered, so the export
//     carries the component as it is and the gate tolerates it.
var AllowedErrorCodes = map[string]struct{}{
	validate.CodeVTODOMissingDue: {},
}

// Report is the outcome of ValidateExport, split by what tlc does with
// each finding: Blocking fails the export, Allowed is an error the
// allow-list tolerates, Warnings are informational (spec SHOULDs).
type Report struct {
	Blocking []validate.Diagnostic
	Allowed  []validate.Diagnostic
	Warnings []validate.Diagnostic
}

// Err returns nil when nothing blocks, else one error naming every
// blocking diagnostic by code and path.
func (r Report) Err() error {
	if len(r.Blocking) == 0 {
		return nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "vtodo: export fails V* validation (%d blocking)", len(r.Blocking))
	for _, d := range r.Blocking {
		fmt.Fprintf(&b, "\n  %s %s: %s", d.Code, d.Path, d.Message)
	}
	return errors.New(b.String())
}

// ValidateExport runs the vstar semantic validator over every
// component of cal, nested sub-components included, and sorts the
// diagnostics into a Report.
//
// validate.Validate walks only cal.Components; it never descends into
// Component.Sub, so a VALARM missing its UID, DTSTAMP or X-VSTAR-HASH
// is invisible to it. The walk here calls validate.ValidateComponent on
// every Sub at every depth so the spec 02 requirements hold for the
// reminders tlc emits too. A nested diagnostic's Path is anchored under
// its parent, e.g. "VCALENDAR.VTODO[uid=a].VALARM[uid=a-alarm].UID";
// the nested locator is the one ValidateComponent chose, so a UID-less
// sub-component reads "[#0]" regardless of its position.
//
// Calendar-scoped rules (VS031, orphan supersession) run for top-level
// components only, which is where vstar defines them.
func ValidateExport(cal vstar.Calendar) Report {
	diags := validate.Validate(cal)
	index := map[vstar.CompType]int{}
	for _, c := range cal.Components {
		validateSubs(c, "VCALENDAR."+locator(c, index), &diags)
	}
	var r Report
	for _, d := range diags {
		switch {
		case d.Severity == validate.SeverityWarning:
			r.Warnings = append(r.Warnings, d)
		case allowed(d.Code):
			r.Allowed = append(r.Allowed, d)
		default:
			r.Blocking = append(r.Blocking, d)
		}
	}
	return r
}

// validateSubs validates every sub-component of c, recursively,
// appending to out with each diagnostic's Path prefixed by path (c's
// own locator).
func validateSubs(c vstar.Component, path string, out *[]validate.Diagnostic) {
	for _, sub := range c.Sub {
		own := validate.ValidateComponent(sub)
		for i := range own {
			own[i].Path = path + "." + own[i].Path
		}
		*out = append(*out, own...)
		// The nested locator is what ValidateComponent used, so the
		// recursion's prefix matches the paths it just emitted.
		validateSubs(sub, path+"."+locator(sub, map[vstar.CompType]int{}), out)
	}
}

// locator mirrors the segment validate.Validate builds for a top-level
// component: "TYPE[uid=<uid>]", or "TYPE[#<n>]" for the n-th UID-less
// component of that type, counting through index.
func locator(c vstar.Component, index map[vstar.CompType]int) string {
	if uid := c.UID(); uid != "" {
		return string(c.Type) + "[uid=" + uid + "]"
	}
	n := index[c.Type]
	index[c.Type] = n + 1
	return string(c.Type) + "[#" + strconv.Itoa(n) + "]"
}

func allowed(code string) bool {
	_, ok := AllowedErrorCodes[code]
	return ok
}
