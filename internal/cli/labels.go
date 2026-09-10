package cli

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/labels"
)

var projectType string

// projectTypeList renders the accepted --type values for help text.
//
// Both the long help and the flag usage read from labels.AllProjectTypes
// so the documented set cannot drift from the set that actually
// produces distinct labels — the drift that let `--type python-mvc` be
// advertised while silently emitting the generic labels.
func projectTypeList() string {
	names := make([]string, 0, len(labels.AllProjectTypes()))
	for _, pt := range labels.AllProjectTypes() {
		names = append(names, string(pt))
	}
	return strings.Join(names, ", ")
}

var labelCmd = &cobra.Command{
	Use:   "label",
	Short: "Manage project labels",
}

var labelInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Auto-detect and initialize project labels",
	Long: `Detect the project type and seed a suggested label set.

Use --type to force a specific template (` + projectTypeList() + `).

Under task.tags.policy: closed, the template's domain:* values are also
recorded in task.tags.allowed so the closed vocabulary names its own
domains; the file written is reported on stdout. Re-running converges.`,
	Annotations: map[string]string{
		"kit/side-effect": "write-local",
		"kit/idempotent":  "yes",
	},
	RunE: func(_ *cobra.Command, _ []string) error {
		wd, _ := os.Getwd() //nolint:errcheck // best-effort directory detection

		var pType labels.ProjectType
		if projectType != "" {
			pType = labels.ProjectType(projectType)
		} else {
			pType = labels.DetectProjectType(wd)
		}

		fmt.Printf("Detected project type: %s\n", pType)

		templates, conflicts := labels.GetTemplatesWithConflicts(pType)

		// Printed before the set, and to stderr, so a duplicate is
		// visible without corrupting the label listing a caller may be
		// piping. The seeding still proceeds: one label carrying the
		// wrong one of two declared colors beats seeding nothing.
		for _, c := range conflicts {
			fmt.Fprintf(os.Stderr, "warning: %s\n", c)
		}

		fmt.Printf("Suggested labels for %s:\n", pType)
		for _, l := range templates {
			fmt.Printf("  - %s (%s): %s\n", l.Name, l.Color, l.Description)
		}

		fmt.Println("\n✓ Labels initialized locally")

		return seedDomainVocabulary(pType)
	},
}

// seedDomainVocabulary records the template's `domain:*` values in
// `task.tags.allowed`, so a closed policy can name its domains instead
// of admitting the whole namespace by wildcard.
//
// The domain axis is the one axis core cannot enumerate for itself:
// values are chosen per project TYPE, and internal/core cannot see the
// type — only the detector that picked the template can, and the
// dependency cannot be inverted because internal/labels already imports
// core. So core admits `domain:*` wholesale, and under `closed` that
// means `domain:strage` is accepted exactly as readily as `domain:cli`.
// Writing the chosen values here is what closes that gap: the config
// records what was actually seeded, and the vocabulary built from it
// admits those values literally.
//
// Gated on `closed` because `allowed` is read by nothing else. Under the
// default `open` policy the key is inert, so writing it would rewrite a
// config file to no observable effect — a surprise on a command a user
// reached for its listing, and one the `write-local` annotation does not
// excuse. Under `closed` the user has already declared they want a
// vocabulary enforced, which is precisely the config this list serves.
func seedDomainVocabulary(pType labels.ProjectType) error {
	// Read the EFFECTIVE policy, not the raw key. TagsConfig.Effective is
	// what folds an empty value into `open`, and every other consumer
	// goes through it; reading `task.tags.policy` off viper directly
	// would bypass that fold and make this the one place that disagrees
	// with the gate it is trying to serve.
	policy, _ := core.TagPolicyFor()
	if policy != config.TagPolicyClosed {
		return nil
	}

	domains := labels.DomainTags(pType)
	if len(domains) == 0 {
		return nil
	}

	existing := viper.GetStringSlice("task.tags.allowed")
	merged, added := mergeAllowedTags(existing, domains)
	if added == 0 {
		// Converged. Re-writing an unchanged list would still rewrite the
		// file — viper re-renders the whole document — so a second run
		// would churn mtime and any comments for no change. `kit/idempotent:
		// yes` promises the second run is a no-op, not merely that it lands
		// on the same values.
		fmt.Printf("Tag vocabulary already records %d domain values; config unchanged\n",
			len(domains))
		return nil
	}

	viper.Set("task.tags.allowed", merged)
	target, err := config.PrepareViperForWrite(viper.GetViper())
	if err != nil {
		return fmt.Errorf("label init could not resolve a writable config file: %w;"+
			" run 'tlc config path' to see the cascade", err)
	}
	if err := viper.WriteConfig(); err != nil {
		if err := viper.SafeWriteConfig(); err != nil {
			return fmt.Errorf("label init could not save tag vocabulary to %s: %w;"+
				" check the file is writable", target, err)
		}
	}

	fmt.Printf("Recorded %d domain tag(s) in task.tags.allowed (written to: %s)\n",
		added, target)
	return nil
}

// mergeAllowedTags unions the seeded domains into the existing list and
// reports how many were new.
//
// Merge, never replace: entries in `allowed` are the user's own
// vocabulary, and a seeding that overwrote them would silently revoke
// tags they had chosen. Sorted so the file is byte-identical across two
// projects seeded from one template, and so a re-seed shows no diff.
// Matching is case-insensitive because core.TagVocabulary.Admits is —
// a user who wrote `Domain:CLI` already has that tag, and adding
// `domain:cli` beside it would list one tag twice.
func mergeAllowedTags(existing, seeded []string) (merged []string, added int) {
	seen := make(map[string]struct{}, len(existing)+len(seeded))
	merged = make([]string, 0, len(existing)+len(seeded))

	for _, e := range existing {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		key := strings.ToLower(e)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		merged = append(merged, e)
	}

	for _, s := range seeded {
		key := strings.ToLower(s)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		merged = append(merged, s)
		added++
	}

	sort.Strings(merged)
	return merged, added
}

var labelListCmd = &cobra.Command{
	Use:   "list",
	Short: "List labels in the current project",
	Long:  "Show every label defined for the current project's storage.",
	Annotations: map[string]string{
		"kit/side-effect": "read",
	},
	RunE: func(_ *cobra.Command, _ []string) error {
		fmt.Println("Project labels:")
		// Logic to list labels from storage
		return nil
	},
}

var labelTemplatesCmd = &cobra.Command{
	Use:   "templates",
	Short: "List available label templates",
	Long:  "List every built-in label template grouped by project type.",
	Annotations: map[string]string{
		"kit/side-effect": "read",
		"kit/idempotent":  "yes",
	},
	RunE: func(_ *cobra.Command, _ []string) error {
		for _, pt := range labels.AllProjectTypes() {
			fmt.Printf("[%s]\n", pt)
			for _, l := range labels.GetTemplates(pt) {
				fmt.Printf("  - %s\n", l.Name)
			}
			fmt.Println()
		}
		return nil
	},
}

func init() {
	labelInitCmd.Flags().StringVar(&projectType, "type", "", "Force project type ("+projectTypeList()+")")

	labelCmd.AddCommand(labelInitCmd)
	labelCmd.AddCommand(labelListCmd)
	labelCmd.AddCommand(labelTemplatesCmd)
	RootCmd.AddCommand(labelCmd)
}
