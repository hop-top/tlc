package cli

import (
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
