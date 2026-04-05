package cli

import (
	"encoding/json"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// FlagSchema describes a single CLI flag for schema export.
type FlagSchema struct {
	Name        string `json:"name"`
	Shorthand   string `json:"shorthand,omitempty"`
	Description string `json:"description"`
	Type        string `json:"type"`
	Default     string `json:"default,omitempty"`
}

// CommandSchema describes a CLI subcommand for schema export.
type CommandSchema struct {
	Name        string       `json:"name"`        // e.g. "task list" or "track create"
	Description string       `json:"description"`
	Args        string       `json:"args,omitempty"`
	Flags       []FlagSchema `json:"flags,omitempty"`
}

// GenerateSchema introspects the command tree rooted at root and returns
// a structured slice describing each runnable command using its full path.
func GenerateSchema(root *cobra.Command) []CommandSchema {
	var schemas []CommandSchema
	for _, cmd := range root.Commands() {
		walkSchema(cmd, cmd.Name(), &schemas)
	}
	return schemas
}

// walkSchema recursively visits cmd and its descendants, appending a
// CommandSchema for every runnable (non-hidden) command. fullName is the
// space-joined path from root, e.g. "task list" or "sync pull".
func walkSchema(cmd *cobra.Command, fullName string, schemas *[]CommandSchema) {
	if cmd.Hidden {
		return
	}

	if cmd.Runnable() {
		cs := CommandSchema{
			Name:        fullName,
			Description: cmd.Short,
			Args:        cmd.Use,
			Flags:       collectSchemaFlags(cmd),
		}
		*schemas = append(*schemas, cs)
	}

	for _, sub := range cmd.Commands() {
		walkSchema(sub, fullName+" "+sub.Name(), schemas)
	}
}

// collectSchemaFlags returns flag schemas for cmd, including its own flags
// and inherited persistent flags from ancestors.
func collectSchemaFlags(cmd *cobra.Command) []FlagSchema {
	seen := make(map[string]struct{})
	var flags []FlagSchema

	addFlag := func(f *pflag.Flag) {
		if f.Hidden {
			return
		}
		if _, ok := seen[f.Name]; ok {
			return
		}
		seen[f.Name] = struct{}{}
		flags = append(flags, FlagSchema{
			Name:        f.Name,
			Shorthand:   f.Shorthand,
			Description: f.Usage,
			Type:        f.Value.Type(),
			Default:     f.DefValue,
		})
	}

	cmd.Flags().VisitAll(addFlag)

	// Walk up the parent chain collecting persistent flags.
	for p := cmd.Parent(); p != nil; p = p.Parent() {
		p.PersistentFlags().VisitAll(addFlag)
	}

	return flags
}

// GenerateSchemaJSON returns indented JSON of the full CLI subcommand schema.
func GenerateSchemaJSON(root *cobra.Command) ([]byte, error) {
	return json.MarshalIndent(GenerateSchema(root), "", "  ")
}

// GenerateTaskSchema is a backward-compatible alias that introspects TaskCmd's
// subcommands. The Name field is just the subcommand name (no "task" prefix).
func GenerateTaskSchema() []CommandSchema {
	var schemas []CommandSchema
	for _, sub := range TaskCmd.Commands() {
		if sub.Hidden {
			continue
		}
		cs := CommandSchema{
			Name:        sub.Name(),
			Description: sub.Short,
			Args:        sub.Use,
		}
		sub.Flags().VisitAll(func(f *pflag.Flag) {
			if f.Hidden {
				return
			}
			cs.Flags = append(cs.Flags, FlagSchema{
				Name:        f.Name,
				Shorthand:   f.Shorthand,
				Description: f.Usage,
				Type:        f.Value.Type(),
				Default:     f.DefValue,
			})
		})
		// Include persistent flags from the parent (TaskCmd).
		TaskCmd.PersistentFlags().VisitAll(func(f *pflag.Flag) {
			if f.Hidden {
				return
			}
			cs.Flags = append(cs.Flags, FlagSchema{
				Name:        f.Name,
				Shorthand:   f.Shorthand,
				Description: f.Usage,
				Type:        f.Value.Type(),
				Default:     f.DefValue,
			})
		})
		schemas = append(schemas, cs)
	}
	return schemas
}

// GenerateTaskSchemaJSON is a backward-compatible alias.
func GenerateTaskSchemaJSON() ([]byte, error) {
	return json.MarshalIndent(GenerateTaskSchema(), "", "  ")
}
