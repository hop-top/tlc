package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/charmbracelet/log"
	"github.com/google/oss-tlc-cli/internal/plugin"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Synchronize tasks with external systems",
}

var syncPullCmd = &cobra.Command{
	Use:   "pull <system>",
	Short: "Pull updates from an external system",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		system := args[0]
		binPath := getPluginPath(system)

		client, err := plugin.NewRPCClient(binPath)
		if err != nil {
			log.Fatal("Failed to start plugin", "system", system, "error", err)
		}
		defer client.Close()

		params := map[string]interface{}{
			"repo": viper.GetString(fmt.Sprintf("sync.%s.repo", system)),
		}

		var result struct {
			Tasks []interface{} `json:"tasks"`
		}

		err = client.Call("sync.pull", params, &result)
		if err != nil {
			log.Fatal("Sync pull failed", "error", err)
		}

		fmt.Fprintf(cmd.OutOrStdout(), "✓ Pulled %d tasks from %s\n", len(result.Tasks), system)
	},
}

var syncPushCmd = &cobra.Command{
	Use:   "push <system>",
	Short: "Push local changes to an external system",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		system := args[0]
		fmt.Fprintf(cmd.OutOrStdout(), "Pushing updates to %s...\n", system)
		// Logic to fetch local changes and call sync.push
	},
}

var syncConfigCmd = &cobra.Command{
	Use:   "config <system>",
	Short: "Configure sync settings for a system",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		system := args[0]
		out := cmd.OutOrStdout()
		fmt.Fprintf(out, "Sync configuration for %s:\n", system)
		fmt.Fprintf(out, "  Repo: %s\n", viper.GetString(fmt.Sprintf("sync.%s.repo", system)))
		fmt.Fprintf(out, "  Direction: %s\n", viper.GetString(fmt.Sprintf("sync.%s.direction", system)))
	},
}

func getPluginPath(system string) string {
	// For dev, check local plugins directory
	localPath := filepath.Join("plugins", system+"-sync", "bin", system+"-sync")
	if _, err := os.Stat(localPath); err == nil {
		return localPath
	}
	
	// Fallback to config or standard location
	return filepath.Join(os.Getenv("HOME"), ".config", "tlc", "plugins", system+"-sync", "bin", system+"-sync")
}

func init() {
	syncCmd.AddCommand(syncPullCmd)
	syncCmd.AddCommand(syncPushCmd)
	syncCmd.AddCommand(syncConfigCmd)
	rootCmd.AddCommand(syncCmd)
}
