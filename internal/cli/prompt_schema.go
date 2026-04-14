package cli

import (
	"encoding/json"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"hop.top/kit/toolspec"
)

// CobraToToolSpec builds a toolspec.ToolSpec from a cobra command tree.
// The resulting spec has hierarchical Commands mirroring the cobra tree.
func CobraToToolSpec(root *cobra.Command) *toolspec.ToolSpec {
	ts := &toolspec.ToolSpec{Name: root.Name()}
	for _, cmd := range root.Commands() {
		if cmd.Hidden {
			continue
		}
		ts.Commands = append(ts.Commands, cobraToCommand(cmd))
	}
	return ts
}

// cobraToCommand converts a single cobra.Command (and its children)
// into a toolspec.Command.
func cobraToCommand(cmd *cobra.Command) toolspec.Command {
	c := toolspec.Command{
		Name:    cmd.Name(),
		Aliases: cmd.Aliases,
		Flags:   collectToolspecFlags(cmd),
	}
	for _, sub := range cmd.Commands() {
		if sub.Hidden {
			continue
		}
		c.Children = append(c.Children, cobraToCommand(sub))
	}
	return c
}

// collectToolspecFlags gathers flags from cmd and its ancestors.
func collectToolspecFlags(cmd *cobra.Command) []toolspec.Flag {
	seen := make(map[string]struct{})
	var flags []toolspec.Flag

	add := func(f *pflag.Flag) {
		if f.Hidden {
			return
		}
		if _, ok := seen[f.Name]; ok {
			return
		}
		seen[f.Name] = struct{}{}
		flags = append(flags, toolspec.Flag{
			Name:        f.Name,
			Short:       f.Shorthand,
			Type:        f.Value.Type(),
			Description: f.Usage,
		})
	}

	cmd.Flags().VisitAll(add)
	for p := cmd.Parent(); p != nil; p = p.Parent() {
		p.PersistentFlags().VisitAll(add)
	}
	return flags
}

// --- Flat schema types for prompt router backward compat ---

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
	Name        string       `json:"name"` // e.g. "task list"
	Description string       `json:"description"`
	Args        string       `json:"args,omitempty"`
	Flags       []FlagSchema `json:"flags,omitempty"`
}

// GenerateSchema introspects the command tree rooted at root and returns
// a flat slice describing each runnable command using its full path.
func GenerateSchema(root *cobra.Command) []CommandSchema {
	var schemas []CommandSchema
	for _, cmd := range root.Commands() {
		walkSchema(cmd, cmd.Name(), &schemas)
	}
	return schemas
}

// walkSchema recursively visits cmd and its descendants, appending a
// CommandSchema for every runnable (non-hidden) command.
func walkSchema(cmd *cobra.Command, fullName string, schemas *[]CommandSchema) {
	if cmd.Hidden {
		return
	}
	if cmd.Runnable() {
		cs := CommandSchema{
			Name:        fullName,
			Description: cmd.Short,
			Args:        cmd.Use,
			Flags:       toolspecFlagsToSchema(collectToolspecFlags(cmd)),
		}
		*schemas = append(*schemas, cs)
	}
	for _, sub := range cmd.Commands() {
		walkSchema(sub, fullName+" "+sub.Name(), schemas)
	}
}

// toolspecFlagsToSchema converts toolspec flags to the flat FlagSchema
// format used by the prompt router. Default values are not available
// in toolspec.Flag, so that field is omitted.
func toolspecFlagsToSchema(flags []toolspec.Flag) []FlagSchema {
	if len(flags) == 0 {
		return nil
	}
	out := make([]FlagSchema, len(flags))
	for i, f := range flags {
		out[i] = FlagSchema{
			Name:        f.Name,
			Shorthand:   f.Short,
			Description: f.Description,
			Type:        f.Type,
		}
	}
	return out
}

// GenerateSchemaJSON returns indented JSON of the full CLI subcommand schema.
func GenerateSchemaJSON(root *cobra.Command) ([]byte, error) {
	return json.MarshalIndent(GenerateSchema(root), "", "  ")
}

// GenerateTaskSchema is a backward-compatible alias that introspects
// TaskCmd's subcommands.
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
			Flags:       toolspecFlagsToSchema(collectToolspecFlags(sub)),
		}
		schemas = append(schemas, cs)
	}
	return schemas
}

// GenerateTaskSchemaJSON is a backward-compatible alias.
func GenerateTaskSchemaJSON() ([]byte, error) {
	return json.MarshalIndent(GenerateTaskSchema(), "", "  ")
}
