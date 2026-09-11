package cli

import (
	"strings"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"
	"hop.top/tlc/internal/config"
)

// The config structs in internal/config carry `yaml` tags only, because
// config files on disk are YAML and those tags name the on-disk keys.
// mapstructure — the decoder viper uses — defaults to its own
// `mapstructure` tag and, finding none, falls back to case-insensitive
// matching on the Go FIELD NAME. Single-word keys survive that fallback by
// coincidence (Statuses↔statuses); every snake_case key does not
// (DefaultTimeout vs default_timeout), and is silently dropped.
//
// Pointing the decoder at the `yaml` tag makes viper agree with the
// on-disk YAML on one spelling of every key. Route every viper decode of a
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
	normalizeTaskConfigKeys(&cfg.Task)
	return nil
}

// unmarshalConfigKey decodes a single viper subtree into out.
// Decoding a task section additionally normalises the task section's map
// keys, so every decode path — whole config or `task` subtree — yields
// keys carrying the case the user wrote.
func unmarshalConfigKey(key string, out any) error {
	if err := viper.UnmarshalKey(key, out, useYAMLTag); err != nil {
		return err //nolint:wrapcheck // thin passthrough; callers add context
	}
	if taskCfg, ok := out.(*config.TaskConfig); ok {
		normalizeTaskConfigKeys(taskCfg)
	}
	return nil
}

// normalizeTaskConfigKeys restores the case of every map key in the task
// section that names a status or a priority.
//
// One entry point rather than a normaliser per surface: viper lower-cases
// map keys globally, so every current and future `map[<vocabulary
// name>]...` in this config hits the identical bug, and the version that
// handled only the state machine is why `task.scheduling.by_priority`
// silently never fired.
func normalizeTaskConfigKeys(cfg *config.TaskConfig) {
	normalizeStateMachineKeys(cfg)
	normalizeSchedulingKeys(cfg)
}

// normalizeSchedulingKeys re-cases `task.scheduling.by_priority` keys
// against the declared priority names.
//
// This is the other half of making by_priority work at all. The lookup in
// applySchedulingConfig interpolates a CANONICAL priority ("P0") into the
// viper key path, while viper stored the user's key lower-cased ("p0"),
// so the two could never meet — the rule was unreachable for every
// priority whose canonical spelling is not already lower-case. Re-casing
// here means the decoded config carries "P0", and the config validator
// can then treat a key that still does not match as the user error it is
// rather than as decoder noise.
func normalizeSchedulingKeys(cfg *config.TaskConfig) {
	if cfg == nil || len(cfg.Scheduling.ByPriority) == 0 {
		return
	}
	canon := make(map[string]string, len(cfg.Priorities)+4)
	for _, p := range cfg.EffectivePriorities() {
		canon[strings.ToLower(p.Name)] = p.Name
	}
	out := make(map[string]config.PriorityScheduleRule, len(cfg.Scheduling.ByPriority))
	for key, rule := range cfg.Scheduling.ByPriority {
		if name, ok := canon[strings.ToLower(key)]; ok {
			key = name
		}
		out[key] = rule
	}
	cfg.Scheduling.ByPriority = out
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
