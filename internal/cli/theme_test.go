package cli

import (
	"testing"

	"charm.land/lipgloss/v2"
	kitcli "hop.top/kit/go/console/cli"
)

// newProbeRoot builds a kit Root with kit's own default theme — the
// state TLC was permanently stuck in before ui.theme was honored.
func newProbeRoot(t *testing.T) *kitcli.Root {
	t.Helper()
	return kitcli.New(kitcli.Config{
		Name:            "tlc",
		Short:           "probe",
		DisableValidate: true,
	})
}

// TestApplyUIThemeChangesTheme is the regression that ui.theme is read at
// all. Before it was wired, kitcli.New was called without Palette or
// Accent and no config value could reach the theme, so every theme name
// produced identical output.
func TestApplyUIThemeChangesTheme(t *testing.T) {
	root := newProbeRoot(t)
	before := root.Theme.Accent

	applyUITheme(root, "bauhaus", "")

	if root.Theme.Accent == before {
		t.Fatal("ui.theme did not change the theme accent; the key is inert")
	}
	if got, want := root.Theme.Palette.Command, kitcli.Bauhaus.Command; got != want {
		t.Errorf("palette command = %v, want %v", got, want)
	}
}

// TestApplyUIThemeDistinctThemes checks the names are not aliases of one
// another — a mapping that resolved every name to the same palette would
// pass the "changed something" test above while still ignoring the value.
func TestApplyUIThemeDistinctThemes(t *testing.T) {
	seen := make(map[kitcli.Palette]string)
	for name := range themePalettes() {
		root := newProbeRoot(t)
		applyUITheme(root, name, "")
		if prev, dup := seen[root.Theme.Palette]; dup {
			t.Errorf("themes %q and %q resolve to the same palette", prev, name)
		}
		seen[root.Theme.Palette] = name
	}
	if len(seen) < 2 {
		t.Fatalf("expected several distinct themes, got %d", len(seen))
	}
}

// TestApplyUIThemeTableStyleBorder is the regression that ui.table_style
// reaches the renderer. The border is the visible difference: "ascii"
// draws +---+ where "unicode" draws box-drawing characters.
func TestApplyUIThemeTableStyleBorder(t *testing.T) {
	root := newProbeRoot(t)

	applyUITheme(root, "", "ascii")
	got, ok := activeTableStyle()
	if !ok {
		t.Fatal("no table style installed")
	}
	if got.Border.Top != lipgloss.ASCIIBorder().Top {
		t.Errorf("border top = %q, want ASCII %q", got.Border.Top, lipgloss.ASCIIBorder().Top)
	}

	applyUITheme(root, "", "unicode")
	got, _ = activeTableStyle()
	if got.Border.Top != lipgloss.NormalBorder().Top {
		t.Errorf("border top = %q, want normal %q", got.Border.Top, lipgloss.NormalBorder().Top)
	}
	if lipgloss.ASCIIBorder().Top == lipgloss.NormalBorder().Top {
		t.Fatal("ascii and unicode borders are identical; the assertions above prove nothing")
	}
}

// TestApplyUIThemeColorsFollowTheme pins the reason the table style is
// re-derived rather than only re-bordered: its colors come from the
// theme, so a theme change with no table_style set must still repaint
// the table.
func TestApplyUIThemeColorsFollowTheme(t *testing.T) {
	root := newProbeRoot(t)

	applyUITheme(root, "neon", "")
	neon, _ := activeTableStyle()

	applyUITheme(root, "bauhaus", "")
	bauhaus, _ := activeTableStyle()

	if neon.Primary == bauhaus.Primary {
		t.Error("table primary color identical across themes; style not re-derived from theme")
	}
}

// TestApplyUIThemeUnknownValues checks an unknown value is tolerated.
// These keys decide what output looks like, not what it means, so a typo
// warns rather than aborting every command.
func TestApplyUIThemeUnknownValues(t *testing.T) {
	root := newProbeRoot(t)
	before := root.Theme.Accent

	applyUITheme(root, "no-such-theme", "no-such-style")

	if root.Theme.Accent != before {
		t.Error("unknown theme changed the theme; it should be ignored")
	}
	if _, ok := activeTableStyle(); !ok {
		t.Error("unknown table style left no style installed")
	}
}

// TestApplyUIThemeEmptyKeepsDefault checks a config saying nothing about
// theming behaves as it did before these keys were honored.
func TestApplyUIThemeEmptyKeepsDefault(t *testing.T) {
	root := newProbeRoot(t)
	before := root.Theme

	applyUITheme(root, "", "")

	if root.Theme.Accent != before.Accent {
		t.Error("empty ui.theme changed the theme")
	}
	style, ok := activeTableStyle()
	if !ok {
		t.Fatal("no table style installed")
	}
	if style.Border.Top != root.TableStyle().Border.Top {
		t.Error("empty ui.table_style changed the border")
	}
}

// TestApplyUIThemeNilRoot guards the initConfig path: themeTarget is nil
// in any test binary that did not run package init().
func TestApplyUIThemeNilRoot(t *testing.T) {
	applyUITheme(nil, "bauhaus", "ascii") // must not panic
}
