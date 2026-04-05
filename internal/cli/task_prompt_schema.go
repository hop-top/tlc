package cli

import (
	"encoding/json"

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

// CommandSchema describes a task subcommand for schema export.
type CommandSchema struct {
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Args        string       `json:"args,omitempty"`
	Flags       []FlagSchema `json:"flags,omitempty"`
}

// GenerateTaskSchema introspects TaskCmd's subcommands and returns
// a structured slice describing each one.
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

// GenerateTaskSchemaJSON returns indented JSON of the task subcommand schema.
func GenerateTaskSchemaJSON() ([]byte, error) {
	return json.MarshalIndent(GenerateTaskSchema(), "", "  ")
}
