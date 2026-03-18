package cli

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"hop.top/tlc/internal/config"
)

var ConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Configuration management",
}

var ConfigValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate configuration",
	RunE: func(_ *cobra.Command, _ []string) error {
		var cfg config.Config
		if err := viper.Unmarshal(&cfg); err != nil {
			return fmt.Errorf("failed to unmarshal config: %w", err)
		}
		if err := cfg.Validate(); err != nil {
			return fmt.Errorf("configuration invalid: %w", err)
		}
		fmt.Println("✓ Configuration is valid")
		return nil
	},
}

var ConfigListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all configuration",
	Run: func(_ *cobra.Command, _ []string) {
		keys := viper.AllKeys()
		sort.Strings(keys)

		for _, key := range keys {
			fmt.Printf("%s: %v\n", key, viper.Get(key))
		}
	},
}

var ConfigGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get config value",
	Args:  cobra.ExactArgs(1),
	Run: func(_ *cobra.Command, args []string) {
		key := args[0]
		if !viper.IsSet(key) {
			fmt.Printf("Error: key %s not set\n", key)
			return
		}
		fmt.Println(viper.Get(key))
	},
}

var ConfigSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set config value",
	Args:  cobra.ExactArgs(2),
	RunE: func(_ *cobra.Command, args []string) error {
		key := args[0]
		value := args[1]

		viper.Set(key, value)
		target, err := config.PrepareViperForWrite(viper.GetViper())
		if err != nil {
			return fmt.Errorf("failed to prepare config file: %w", err)
		}

		if err := viper.WriteConfig(); err != nil {
			if err := viper.SafeWriteConfig(); err != nil {
				return fmt.Errorf("failed to save config: %w", err)
			}
		}

		fmt.Printf("Set %s = %s (written to: %s)\n", key, value, target)
		return nil
	},
}

func init() {
	ConfigCmd.AddCommand(ConfigValidateCmd)
	ConfigCmd.AddCommand(ConfigListCmd)
	ConfigCmd.AddCommand(ConfigGetCmd)
	ConfigCmd.AddCommand(ConfigSetCmd)
	RootCmd.AddCommand(ConfigCmd)
}
