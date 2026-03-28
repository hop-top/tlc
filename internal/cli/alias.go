package cli

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

// AliasCmd is the root alias command.
var AliasCmd = &cobra.Command{
	Use:   "alias",
	Short: "Manage command aliases",
	Long: `Define short aliases for longer tlc commands.

Aliases are stored in the local project config (.tlc/config.yaml) or the
global user config (~/.config/tlc/config.yaml) when --global is given.

Examples:
  tlc alias add tl "task list"          # tlc tl → tlc task list
  tlc alias add tl "task list" --global # global alias
  tlc alias list
  tlc alias remove tl`,
}

var aliasAddCmd = &cobra.Command{
	Use:   "add <name> <expansion>",
	Short: "Add or update an alias",
	Args:  cobra.ExactArgs(2),
	RunE:  runAliasAdd,
}

var aliasListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all aliases",
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runAliasList(cmd)
	},
}

var aliasRemoveCmd = &cobra.Command{
	Use:     "remove <name>",
	Aliases: []string{"rm", "delete", "del"},
	Short:   "Remove an alias",
	Args:    cobra.ExactArgs(1),
	RunE:    runAliasRemove,
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

func runAliasAdd(cmd *cobra.Command, args []string) error {
	name, expansion := args[0], args[1]

	if err := validateAliasName(name); err != nil {
		return err
	}

	path := localAliasPath()
	if aliasGlobal {
		path = globalAliasPath()
	}

	aliases, err := loadAliasesFrom(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("load aliases: %w", err)
	}
	if aliases == nil {
		aliases = aliasMap{}
	}

	aliases[name] = expansion
	if err := saveAliasesTo(path, aliases); err != nil {
		return fmt.Errorf("save alias: %w", err)
	}

	scope := "local"
	if aliasGlobal {
		scope = "global"
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Added %s alias: %s = %q\n", scope, name, expansion)
	return nil
}

func runAliasList(cmd *cobra.Command) error {
	globalPath := globalAliasPath()
	localPath := localAliasPath()

	globalAliases, err := loadAliasesFrom(globalPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("load global aliases: %w", err)
	}

	localAliases, err := loadAliasesFrom(localPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("load local aliases: %w", err)
	}

	if len(globalAliases) == 0 && len(localAliases) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No aliases defined.")
		return nil
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)

	printSection := func(label string, m aliasMap) {
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
	return w.Flush()
}

func runAliasRemove(cmd *cobra.Command, args []string) error {
	name := args[0]

	path := localAliasPath()
	if aliasGlobal {
		path = globalAliasPath()
	}

	aliases, err := loadAliasesFrom(path)
	if err != nil && errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("alias %q not found", name)
	}
	if err != nil {
		return fmt.Errorf("load aliases: %w", err)
	}

	if _, ok := aliases[name]; !ok {
		return fmt.Errorf("alias %q not found", name)
	}

	delete(aliases, name)
	if err := saveAliasesTo(path, aliases); err != nil {
		return fmt.Errorf("save aliases: %w", err)
	}

	scope := "local"
	if aliasGlobal {
		scope = "global"
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
