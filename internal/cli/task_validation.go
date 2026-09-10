package cli

import (
	"hop.top/tlc/internal/config"
)

// getValidationConfig unmarshals the validation section from the active viper config.
// Returns a zero-value ValidationConfig (no-op) on any error.
func getValidationConfig() config.ValidationConfig {
	var cfg config.Config
	if err := unmarshalConfig(&cfg); err != nil {
		return config.ValidationConfig{}
	}
	return cfg.Validation
}
