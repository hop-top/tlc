package cli

import (
	"strings"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"
	"hop.top/tlc/internal/config"
)

// The config structs in internal/config carry `yaml` tags only, because
// they are also decoded straight from YAML by config.LoadConfig via
// kit/config. mapstructure — the decoder viper uses — defaults to its own
// `mapstructure` tag and, finding none, falls back to case-insensitive
// matching on the Go FIELD NAME. Single-word keys survive that fallback by
// coincidence (Statuses↔statuses); every snake_case key does not
// (DefaultTimeout vs default_timeout), and is silently dropped.
//
// Pointing the decoder at the `yaml` tag makes viper agree with the YAML
// loader on one spelling of every key. Route every viper decode of a
// config struct through the helpers below so a new call site cannot
// reintroduce the drop by forgetting the option.
//
// Note this deliberately keeps `yaml:"-"` fields (config.FilesystemConfig)
// unset, matching the YAML path where they are populated by a custom
// UnmarshalYAML rather than by field-name matching.
func useYAMLTag(dc *mapstructure.DecoderConfig) {
	dc.TagName = "yaml"
}

// unmarshalConfig decodes the whole merged viper config into cfg.
// The viper error passes through unwrapped: callers already phrase their
// own context, and existing messages must not shift.
func unmarshalConfig(cfg *config.Config) error {
	if err := viper.Unmarshal(cfg, useYAMLTag); err != nil {
		return err //nolint:wrapcheck // thin passthrough; callers add context
	}
	normalizeStateMachineKeys(&cfg.Task)
	return nil
}

// unmarshalConfigKey decodes a single viper subtree into out.
// Decoding a task section additionally normalises state-machine keys, so
// every decode path — whole config or `task` subtree — yields rules whose
// `from` keys carry the case the user wrote.
func unmarshalConfigKey(key string, out any) error {
	if err := viper.UnmarshalKey(key, out, useYAMLTag); err != nil {
		return err //nolint:wrapcheck // thin passthrough; callers add context
	}
	if taskCfg, ok := out.(*config.TaskConfig); ok {
		normalizeStateMachineKeys(taskCfg)
	}
	return nil
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
