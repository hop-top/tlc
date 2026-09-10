package cli

import (
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
	return viper.Unmarshal(cfg, useYAMLTag) //nolint:wrapcheck // thin passthrough; callers add context
}

// unmarshalConfigKey decodes a single viper subtree into out.
func unmarshalConfigKey(key string, out any) error {
	return viper.UnmarshalKey(key, out, useYAMLTag) //nolint:wrapcheck // thin passthrough; callers add context
}
