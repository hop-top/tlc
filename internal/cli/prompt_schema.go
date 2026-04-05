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

// GenerateSchema introspects all top-level subcommands of root and returns
// a structured slice describing each nested subcommand as "top sub".
func GenerateSchema(root *cobra.Command) []CommandSchema {
	var schemas []CommandSchema
	for _, top := range root.Commands() {
		if top.Hidden {
			continue
		}
		for _, sub := range top.Commands() {
			if sub.Hidden {
				continue
			}
			cs := CommandSchema{
				Name:        top.Name() + " " + sub.Name(),
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
			// Include persistent flags from the parent (top-level cmd).
			top.PersistentFlags().VisitAll(func(f *pflag.Flag) {
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
	}
	return schemas
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
