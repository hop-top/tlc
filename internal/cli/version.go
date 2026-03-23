package cli

import (
	"encoding/json"
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Long:  `Print detailed version information including build metadata.`,
	Run: func(cmd *cobra.Command, _ []string) {
		short, _ := cmd.Flags().GetBool("short")
		jsonOut, _ := cmd.Flags().GetBool("json")

		if short {
			fmt.Fprintln(cmd.OutOrStdout(), tlcVersion)
			return
		}

		if jsonOut {
			info := map[string]string{
				"version":   tlcVersion,
				"goVersion": runtime.Version(),
				"os":        runtime.GOOS,
				"arch":      runtime.GOARCH,
			}
			data, _ := json.MarshalIndent(info, "", "  ")
			fmt.Fprintln(cmd.OutOrStdout(), string(data))
			return
		}

		fmt.Fprintf(cmd.OutOrStdout(), "tlc version %s\n", tlcVersion)
	},
}

func init() {
	versionCmd.Flags().Bool("short", false, "Output only the version number")
	versionCmd.Flags().Bool("json", false, "Output version info as JSON")
	RootCmd.AddCommand(versionCmd)
}
