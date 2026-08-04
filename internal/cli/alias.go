// Package cli — alias.go wires the kit primitive hop.top/kit/go/console/alias
// (a YAML-backed Store) into TLC's cobra command tree, preserving the
// --global flag semantics inherited from the legacy viper-backed aliases.
//
// Storage:
//   - Global: $XDG_CONFIG_HOME/tlc/aliases.yaml (resolved via xdg.ConfigDir).
//   - Project (when in a tlc project): <project-root>/.tlc/aliases.yaml.
//
// Precedence: project > global. ExpandAliases (alias_expand.go) merges both
// stores at expansion time with project entries overriding global.
//
// Migration history: the `aliases:` key was deprecated when this package
// was introduced; auto-strip behaviour was added later (see CHANGELOG.md
// "Unreleased" section, T-1342).
package cli

import (
	"fmt"
	"sort"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"hop.top/kit/go/console/alias"
)

// AliasCmd is the root alias command.
var AliasCmd = &cobra.Command{
	Use:   "alias",
	Short: "Manage command aliases",
	Long: `Define short aliases for longer tlc commands.

Aliases are stored in YAML files:
  - global: $XDG_CONFIG_HOME/tlc/aliases.yaml
  - local:  <project-root>/.tlc/aliases.yaml (when in a project)

Project aliases override global aliases on collision.

Examples:
  tlc alias add tl "task list"          # tlc tl → tlc task list
  tlc alias add tl "task list" --global # global alias
  tlc alias list
  tlc alias remove tl`,
}

var aliasAddCmd = &cobra.Command{
	Use:   "add <name> <expansion>",
	Short: "Add or update an alias",
	Long: `Add or update an alias mapping <name> to <expansion>.

The alias is stored in the project alias file by default. Pass --global
to store it in the user-wide alias file instead.`,
	Args: cobra.ExactArgs(2),
	RunE: runAliasAdd,
	Annotations: map[string]string{
		"kit/side-effect": "write-local",
		"kit/idempotent":  "yes",
	},
}

var aliasListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all aliases",
	Long: `List all aliases, grouped by scope.

Global aliases are read from $XDG_CONFIG_HOME/tlc/aliases.yaml.
Local aliases are read from <project-root>/.tlc/aliases.yaml when
invoked inside a tlc project.`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runAliasList(cmd)
	},
	Annotations: map[string]string{
		"kit/side-effect": "read",
	},
}

var aliasRemoveCmd = &cobra.Command{
	Use:     "remove <name>",
	Aliases: []string{"rm", "delete", "del"},
	Short:   "Remove an alias",
	Long: `Remove an existing alias by name.

Without --global, removes from the project alias file when available,
otherwise from the global alias file.`,
	Args: cobra.ExactArgs(1),
	RunE: runAliasRemove,
	Annotations: map[string]string{
		"kit/side-effect": "destructive-local",
		"kit/idempotent":  "yes",
	},
}

var aliasGlobal bool

func init() {
	aliasAddCmd.Flags().BoolVarP(&aliasGlobal, "global", "g", false, "store in global config")
	aliasRemoveCmd.Flags().BoolVarP(&aliasGlobal, "global", "g", false, "remove from global config")

	AliasCmd.AddCommand(aliasAddCmd)
	AliasCmd.AddCommand(aliasListCmd)
	AliasCmd.AddCommand(aliasRemoveCmd)
	RootCmd.AddCommand(AliasCmd)
}

// pickStore returns the project store when --global is unset and a project
// store path is resolvable; otherwise the global store. Falls back to the
// global store if no project root is detected.
func pickStore() (*alias.Store, string, error) {
	if !aliasGlobal {
		path := localAliasPath()
		if path != "" {
			s, err := loadStore(path)
			return s, "local", err
		}
	}
	path, err := globalAliasPath()
	if err != nil {
		return nil, "", err
	}
	s, err := loadStore(path)
	return s, "global", err
}

// loadStore returns an alias.Store with its YAML file loaded (missing file
// is not an error — the store stays empty).
func loadStore(path string) (*alias.Store, error) {
	s := alias.NewStore(path)
	if err := s.Load(); err != nil {
		return nil, fmt.Errorf("load alias store %s: %w", path, err)
	}
	return s, nil
}

func runAliasAdd(cmd *cobra.Command, args []string) error {
	name, expansion := args[0], args[1]

	if err := validateAliasName(name); err != nil {
		return err
	}

	store, scope, err := pickStore()
	if err != nil {
		return fmt.Errorf("resolve alias store: %w", err)
	}
	if err := store.Set(name, expansion); err != nil {
		return fmt.Errorf("set alias %q: %w", name, err)
	}
	if err := store.Save(); err != nil {
		return fmt.Errorf("save alias: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Added %s alias: %s = %q\n", scope, name, expansion)
	return nil
}

func runAliasList(cmd *cobra.Command) error {
	globalPath, err := globalAliasPath()
	if err != nil {
		return fmt.Errorf("resolve global alias path: %w", err)
	}
	gs, err := loadStore(globalPath)
	if err != nil {
		return fmt.Errorf("load global aliases: %w", err)
	}
	globalAliases := gs.All()

	var localAliases map[string]string
	if lp := localAliasPath(); lp != "" {
		ls, err := loadStore(lp)
		if err != nil {
			return fmt.Errorf("load local aliases: %w", err)
		}
		localAliases = ls.All()
	}

	if len(globalAliases) == 0 && len(localAliases) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No aliases defined.")
		return nil
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	printSection := func(label string, m map[string]string) {
		if len(m) == 0 {
			return
		}
		fmt.Fprintf(w, "[%s]\n", label)
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(w, "  %s\t= %s\n", k, m[k])
		}
	}
	printSection("global", globalAliases)
	printSection("local", localAliases)
	if err := w.Flush(); err != nil {
		return fmt.Errorf("flush alias table: %w", err)
	}
	return nil
}

func runAliasRemove(cmd *cobra.Command, args []string) error {
	name := args[0]

	store, scope, err := pickStore()
	if err != nil {
		return fmt.Errorf("resolve alias store: %w", err)
	}
	if _, ok := store.Get(name); !ok {
		return fmt.Errorf("alias %q not found", name)
	}
	if err := store.Remove(name); err != nil {
		return fmt.Errorf("remove alias %q: %w", name, err)
	}
	if err := store.Save(); err != nil {
		return fmt.Errorf("save aliases: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Removed %s alias: %s\n", scope, name)
	return nil
}

// validateAliasName rejects names that would shadow built-in cobra commands
// or contain invalid characters.
func validateAliasName(name string) error {
	if name == "" {
		return fmt.Errorf("alias name must not be empty")
	}
	for _, c := range name {
		if c == ' ' || c == '\t' || c == '\n' {
			return fmt.Errorf("alias name must not contain whitespace")
		}
	}
	// Prevent direct shadowing of built-in top-level commands.
	builtins := map[string]bool{
		"alias": true, "task": true, "config": true, "sync": true,
		"init": true, "doctor": true, "log": true, "flow": true,
		"project": true, "tag": true, "workspace": true,
		"upgrade": true, "version": true, "help": true,
		"auth": true, "summary": true, "uri": true,
	}
	if builtins[name] {
		return fmt.Errorf("alias %q conflicts with a built-in command", name)
	}
	return nil
}
