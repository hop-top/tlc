package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/core"
)

func init() {
	core.SetTaskConfigProvider(loadTaskConfig)
}

// loadTaskConfig unmarshals the merged `task` config section for the
// workflow engine. Returns nil when the section cannot be decoded, which
// leaves core on its built-in statuses and state machine.
//
// State-machine key normalisation happens inside unmarshalConfigKey, so
// this provider sees the same rules every other config consumer does.
func loadTaskConfig() *config.TaskConfig {
	var cfg config.TaskConfig
	if err := unmarshalConfigKey("task", &cfg); err != nil {
		return nil
	}
	return &cfg
}

// workflowConfigError validates the merged task workflow config for the
// command about to run. It returns nil for init and the config commands,
// which must keep working so a bad value can be repaired.
func workflowConfigError(c *cobra.Command) error {
	if c.Name() == "init" {
		return nil
	}
	for p := c; p != nil; p = p.Parent() {
		if p.Name() == "config" {
			return nil
		}
	}
	cfg := loadTaskConfig()
	if cfg == nil {
		return nil
	}
	if err := cfg.ValidateWorkflow(); err != nil {
		return fmt.Errorf("invalid task config: %w", err)
	}
	return nil
}
