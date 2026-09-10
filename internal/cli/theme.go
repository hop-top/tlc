package cli

import (
	"fmt"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
	"charm.land/log/v2"
	kitcli "hop.top/kit/go/console/cli"
	"hop.top/kit/go/console/output"
)

// This file makes `ui.theme` and `ui.table_style` mean something.
//
// Both keys read as working before this existed, which is what made them
// worse than an ordinary dead key. The theme genuinely applied and tables
// genuinely had a style — but from kit's defaults, never from the user's
// config. kit derives its theme from cli.Config.Palette/.Accent, and TLC's
// kitcli.New(...) passed neither, so every value a user wrote under `ui:`
// was inert while the output it named kept changing anyway.
//
// The wiring has to happen HERE rather than in kitRoot(): kitRootInstance
// is a package var, so kitcli.New runs at package-init time, long before
// cobra.OnInitialize(initConfig) has read a config file. applyUITheme is
// called from the end of initConfig instead, once the merged config is
// readable, and re-derives the table style from the resulting theme.

// themePalettes maps a `ui.theme` value to a kit palette.
//
// Names are kit's own built-ins rather than a TLC-invented vocabulary, so
// a theme learned here transfers to any other kit-powered tool.
func themePalettes() map[string]kitcli.Palette {
	return map[string]kitcli.Palette{
		"neon":    kitcli.Neon,
		"dark":    kitcli.Dark,
		"bauhaus": kitcli.Bauhaus,
	}
}

// tableBorders maps a `ui.table_style` value to a lipgloss border.
//
// "ascii" exists for terminals and pipelines that mangle box-drawing
// characters; "none" drops the border while keeping the theme's colors.
func tableBorders() map[string]lipgloss.Border {
	return map[string]lipgloss.Border{
		"unicode": lipgloss.NormalBorder(),
		"rounded": lipgloss.RoundedBorder(),
		"thick":   lipgloss.ThickBorder(),
		"double":  lipgloss.DoubleBorder(),
		"ascii":   lipgloss.ASCIIBorder(),
		"none":    lipgloss.HiddenBorder(),
	}
}

// sortedNameList renders a name set for an error message, so the same bad
// value always reports the same list (Go map order is randomized).
func sortedNameList[V any](m map[string]V) string {
	names := make([]string, 0, len(m))
	for k := range m {
		names = append(names, k)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// resolveTheme returns the palette named by theme.
//
// An empty name keeps kit's default, so a config that says nothing about
// theming behaves exactly as it did before this key was honored.
func resolveTheme(theme string) (kitcli.Palette, bool, error) {
	name := strings.ToLower(strings.TrimSpace(theme))
	if name == "" {
		return kitcli.Palette{}, false, nil
	}
	p, ok := themePalettes()[name]
	if !ok {
		return kitcli.Palette{}, false, fmt.Errorf(
			"ui.theme %q is not a known theme: must be one of %s",
			theme, sortedNameList(themePalettes()),
		)
	}
	return p, true, nil
}

// resolveTableBorder returns the border named by style.
func resolveTableBorder(style string) (lipgloss.Border, bool, error) {
	name := strings.ToLower(strings.TrimSpace(style))
	if name == "" {
		return lipgloss.Border{}, false, nil
	}
	b, ok := tableBorders()[name]
	if !ok {
		return lipgloss.Border{}, false, fmt.Errorf(
			"ui.table_style %q is not a known table style: must be one of %s",
			style, sortedNameList(tableBorders()),
		)
	}
	return b, true, nil
}

// themeForPalette builds a kit Theme from a palette.
//
// kit derives a Theme from a Palette only inside cli.New — its
// themeFromPalette is unexported — so the palette is handed to a
// throwaway Root purely to read the Theme back out. New builds its own
// private viper and cobra command and touches no global state, so the
// discarded Root costs nothing beyond the allocation.
//
// If kit later exports a palette-to-theme constructor, this collapses to
// a single call.
func themeForPalette(p kitcli.Palette) kitcli.Theme {
	return kitcli.New(kitcli.Config{
		Name:            "tlc",
		Short:           "theme probe",
		Palette:         p,
		DisableValidate: true,
	}).Theme
}

// buildTableStyle derives the TableStyle for a theme and border choice.
//
// Colors come from the theme so a table always matches the rest of the
// CLI's output; only the border is the table's own business.
func buildTableStyle(root *kitcli.Root, border lipgloss.Border, hasBorder bool) output.TableStyle {
	s := root.TableStyle()
	if hasBorder {
		s.Border = border
	}
	return s
}

// applyUITheme applies `ui.theme` and `ui.table_style` to the kit root and
// the package-level table style.
//
// Unknown values warn rather than abort. These keys decide what output
// LOOKS like, not what it means, so a typo in one is not a reason to
// refuse to run every command in the tool — the same warning-only footing
// the rest of the config validation path sits on. The warning names the
// valid set, so the typo is still diagnosable.
func applyUITheme(root *kitcli.Root, theme, tableStyle string) {
	if root == nil {
		return
	}

	if p, ok, err := resolveTheme(theme); err != nil {
		log.Warn("Invalid configuration", "error", err)
	} else if ok {
		root.Theme = themeForPalette(p)
	}

	border, hasBorder, err := resolveTableBorder(tableStyle)
	if err != nil {
		log.Warn("Invalid configuration", "error", err)
	}

	// Re-derive the table style even when only the theme changed: its
	// colors come from the theme, so a theme applied without this would
	// leave tables painted in the previous theme's colors.
	setTableStyle(buildTableStyle(root, border, hasBorder))
}
