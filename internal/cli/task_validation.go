package cli

import (
	"github.com/spf13/viper"
	"hop.top/tlc/internal/config"
)

// getValidationConfig unmarshals the validation section from the active viper config.
// Returns a zero-value ValidationConfig (no-op) on any error.
func getValidationConfig() config.ValidationConfig {
	var cfg config.Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return config.ValidationConfig{}
	}
	return cfg.Validation
}
