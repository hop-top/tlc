package cli

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"hop.top/tlc/internal/config"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Configuration management",
}

var configValidateCmd = &cobra.Command{
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

var configListCmd = &cobra.Command{
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

var configGetCmd = &cobra.Command{
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

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set config value",
	Args:  cobra.ExactArgs(2),
	RunE: func(_ *cobra.Command, args []string) error {
		key := args[0]
		value := args[1]

		viper.Set(key, value)

		if err := viper.WriteConfig(); err != nil {
			if err := viper.SafeWriteConfig(); err != nil {
				return fmt.Errorf("failed to save config: %w", err)
			}
		}

		fmt.Printf("Set %s = %s\n", key, value)
		return nil
	},
}

func init() {
	configCmd.AddCommand(configValidateCmd)
	configCmd.AddCommand(configListCmd)
	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configSetCmd)
	rootCmd.AddCommand(configCmd)
}
