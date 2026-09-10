package cli

import (
	"strings"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"
	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/core"
)

func init() {
	core.SetTaskConfigProvider(loadTaskConfig)
}

// loadTaskConfig unmarshals the merged `task` config section for the
// workflow engine. Returns nil when the section cannot be decoded, which
// leaves core on its built-in statuses and state machine.
func loadTaskConfig() *config.TaskConfig {
	var cfg config.TaskConfig
	if err := viper.UnmarshalKey("task", &cfg, useYAMLTags); err != nil {
		return nil
	}
	normalizeStateMachineKeys(&cfg)
	return &cfg
}

// useYAMLTags makes mapstructure read the `yaml` struct tags. The config
// structs carry no `mapstructure` tags, so without this every snake_case
// key (default_status, state_machine, ...) decodes to its zero value.
// Passing it stays correct even if `mapstructure` tags are added later.
func useYAMLTags(dc *mapstructure.DecoderConfig) {
	dc.TagName = "yaml"
}

// normalizeStateMachineKeys restores the case of state-machine `from` keys.
//
// viper lower-cases every map KEY in its internal store, so a rule written
// as `TODO: [IN_PROGRESS]` unmarshals as `todo: [IN_PROGRESS]` — the values
// survive because they are slice elements, not keys. Left alone, config
// validation rejects the rules ("unknown status: todo") and, worse,
// NewWorkflowManager accepts them with a nil error and then refuses every
// transition. Re-marshaling from viper.Get does not help: the keys are
// already lower-cased in viper's map by then.
//
// Keys are re-cased against the declared status names, so custom statuses
// are handled as well as the built-ins. Keys matching no declared status
// are left untouched so validation still reports them.
func normalizeStateMachineKeys(cfg *config.TaskConfig) {
	if cfg == nil {
		return
	}
	statuses := cfg.Statuses
	if len(statuses) == 0 {
		statuses = config.GetDefaultStatuses()
	}
	canon := make(map[string]string, len(statuses))
	for _, s := range statuses {
		canon[strings.ToLower(s.Name)] = s.Name
	}

	normalizeRules(cfg.StateMachine, canon)
	for tag, override := range cfg.Workflows {
		normalizeRules(override.StateMachine, canon)
		cfg.Workflows[tag] = override
	}
}

// normalizeRules re-cases the `from` keys of one rule set in place.
func normalizeRules(def *config.WorkflowDefinition, canon map[string]string) {
	if def == nil || len(def.Rules) == 0 {
		return
	}
	rules := make(map[string][]string, len(def.Rules))
	for from, to := range def.Rules {
		if name, ok := canon[strings.ToLower(from)]; ok {
			from = name
		}
		rules[from] = to
	}
	def.Rules = rules
}
