package completions

import (
	"context"
	"strings"

	"github.com/spf13/cobra"
	"hop.top/uri"
)

// CobraCompleter provides Cobra-compatible completion logic for URI types.
type CobraCompleter struct {
	Registry *uri.Registry
}

// NewCobraCompleter creates a new Cobra completion helper.
func NewCobraCompleter(r *uri.Registry) *CobraCompleter {
	return &CobraCompleter{Registry: r}
}

// Complete returns a ValidArgsFunction for the given URI type.
func (c *CobraCompleter) Complete(typeName string) func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		prefix := toComplete
		scheme := ""
		if strings.Contains(toComplete, "://") {
			parts := strings.SplitN(toComplete, "://", 2)
			scheme = parts[0]
			prefix = parts[1]
		}

		suggestions, err := c.Registry.Complete(context.Background(), typeName, prefix)
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}

		if scheme != "" {
			for i, s := range suggestions {
				suggestions[i] = scheme + "://" + s
			}
		}

		return suggestions, cobra.ShellCompDirectiveNoFileComp
	}
}
