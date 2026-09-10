package labels

import (
	"fmt"
	"strings"
	"testing"
)

// stateAxes are the axes that describe one task's CURRENT condition, as
// opposed to classifying the change that produced it or naming the part
// of the repo it touches.
//
// The distinctness rule below is scoped to exactly these four, and the
// two exclusions are deliberate rather than convenient:
//
//   - `type:*` mirrors Conventional Commits and its swatches predate the
//     dimension prefix; `type:breaking` has shared B60205 with
//     `priority:critical`, and `type:ci` has shared 1D76DB with
//     `status:in-progress`, since before either axis was generated.
//     Asserting over it would assert a property this set has never held.
//   - `domain:*` is per project type and reuses the palette heavily by
//     design (`domain:api` alone appears in four different colors across
//     types), so it has no single set to be distinct from.
//
// What the four here share is that they are read TOGETHER as a strip on
// a single issue — every one of them can be present at once, on any
// project type — which is what makes an exact color match between two of
// them a defect rather than a coincidence.
var stateAxes = map[string]bool{
	"status":   true,
	"priority": true,
	"effort":   true,
	"needs":    true,
}

// TestStateAxesDoNotShareColors is the cross-axis distinctness guard.
//
// `status:blocked` shipped on D93F0B, which is `priority:high`'s swatch
// and `needs:repro`'s — a three-way collision on the one axis strip a
// triager reads at a glance. `needs:triage` and `priority:medium`
// likewise both sat on FBCA04.
//
// Scoped to the built-in vocabulary: a user's declared color is theirs
// to collide if they want, and this package will not second-guess it.
func TestStateAxesDoNotShareColors(t *testing.T) {
	owner := make(map[string]string)
	for _, l := range GetTemplates(TypeGeneric) {
		axis, _, ok := strings.Cut(l.Name, ":")
		if !ok || !stateAxes[axis] {
			continue
		}
		prev, clash := owner[l.Color]
		if clash {
			prevAxis, _, _ := strings.Cut(prev, ":")
			if prevAxis != axis {
				t.Errorf("%s and %s both use %s; the two axes render side by side on one issue", prev, l.Name, l.Color)
			}
			continue
		}
		owner[l.Color] = l.Name
	}
}

// TestBlockedDoesNotWearPriorityHigh names the specific pair, so the
// property survives a future re-palette of either axis.
func TestBlockedDoesNotWearPriorityHigh(t *testing.T) {
	byName := make(map[string]string)
	for _, l := range GetTemplates(TypeGeneric) {
		byName[l.Name] = l.Color
	}
	high, ok := byName["priority:high"]
	if !ok {
		t.Fatal("priority:high absent; the collision guard has nothing to compare against")
	}
	if got := byName["status:blocked"]; got == high {
		t.Errorf("status:blocked color %q is priority:high's", got)
	}
	if got := byName["needs:repro"]; got == high {
		t.Errorf("needs:repro color %q is priority:high's", got)
	}
}

// TestPriorityAndEffortSwatchesUnmoved re-states, from this file, that
// the fix for the collision did not pay for itself out of the two axes
// pinned by the existing stability tests.
//
// `sync push` writes the color to the forge, so moving one of these
// re-colors every issue already carrying the label. The collision had to
// be resolved by moving `status:*` and `needs:*` instead.
func TestPriorityAndEffortSwatchesUnmoved(t *testing.T) {
	want := map[string]string{
		"priority:critical": "B60205",
		"priority:high":     "D93F0B",
		"priority:medium":   "FBCA04",
		"priority:low":      "0E8A16",
		"effort:xs":         "C2E0C6",
		"effort:s":          "9EDAB0",
		"effort:m":          "7BC99B",
		"effort:l":          "4FA97F",
		"effort:xl":         "2E8B62",
	}
	for _, l := range GetTemplates(TypeGeneric) {
		w, ok := want[l.Name]
		if !ok {
			continue
		}
		if l.Color != w {
			t.Errorf("%s color = %q, want %q (pinned; sync push re-colors live issues)", l.Name, l.Color, w)
		}
		delete(want, l.Name)
	}
	for name := range want {
		t.Errorf("missing pinned label %q", name)
	}
}

// TestStateAxisColorsAreDistinctEnough guards against trading an exact
// collision for an indistinguishable near-miss.
//
// The threshold is the smallest separation the set already tolerated
// before this fix — `priority:low` 0E8A16 against `effort:xl` 2E8B62 —
// so it pins the palette at no worse than it has always been rather than
// inventing a perceptual standard this project has never claimed.
func TestStateAxisColorsAreDistinctEnough(t *testing.T) {
	const floor = 82.0

	type entry struct{ name, color string }
	var got []entry
	for _, l := range GetTemplates(TypeGeneric) {
		axis, _, ok := strings.Cut(l.Name, ":")
		if ok && stateAxes[axis] {
			got = append(got, entry{l.Name, l.Color})
		}
	}

	for i := range got {
		for j := i + 1; j < len(got); j++ {
			a, b := got[i], got[j]
			axisA, _, _ := strings.Cut(a.name, ":")
			axisB, _, _ := strings.Cut(b.name, ":")
			if axisA == axisB {
				continue
			}
			d, err := colorDistance(a.color, b.color)
			if err != nil {
				t.Fatalf("%s/%s: %v", a.name, b.name, err)
			}
			if d < floor {
				t.Errorf("%s (%s) and %s (%s) differ by %.1f, under the %.0f floor", a.name, a.color, b.name, b.color, d, floor)
			}
		}
	}
}

// colorDistance is the plain RGB euclidean distance between two hex
// swatches. Crude next to a perceptual metric, and sufficient here: it
// is used only to catch a swatch chosen so close to another that the two
// badges read as one.
func colorDistance(a, b string) (float64, error) {
	ra, err := parseHex(a)
	if err != nil {
		return 0, err
	}
	rb, err := parseHex(b)
	if err != nil {
		return 0, err
	}
	var sum float64
	for i := range ra {
		d := float64(ra[i]) - float64(rb[i])
		sum += d * d
	}
	// math.Sqrt via exponent avoids an import for one call.
	return sqrt(sum), nil
}

func parseHex(s string) ([3]int, error) {
	var out [3]int
	if len(s) != 6 {
		return out, fmt.Errorf("color %q is not 6 hex digits", s)
	}
	for i := range 3 {
		var v int
		if _, err := fmt.Sscanf(s[i*2:i*2+2], "%02x", &v); err != nil {
			return out, fmt.Errorf("color %q: %w", s, err)
		}
		out[i] = v
	}
	return out, nil
}

func sqrt(x float64) float64 {
	if x <= 0 {
		return 0
	}
	z := x
	for range 40 {
		z = (z + x/z) / 2
	}
	return z
}
