package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/charmbracelet/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/inbox"
)

var InboxCmd = &cobra.Command{
	Use:   "inbox",
	Short: "Manage the file-based inbox",
}

var inboxProcessCmd = &cobra.Command{
	Use:   "process",
	Short: "Process pending inbox files",
	Long: `Scan .tlc/inbox/create/ and .tlc/inbox/transition/
for pending files, create tasks or transition statuses,
and move processed files to processed/ or failed/.`,
	RunE: runInboxProcess,
}

func runInboxProcess(cmd *cobra.Command, _ []string) error {
	s, err := getStorage()
	if err != nil {
		return fmt.Errorf(
			"inbox process: cannot open storage; "+
				"run 'tlc init' first",
		)
	}

	svc := core.NewTaskService(s, s)
	dir := inboxDir()

	proc := inbox.NewFileInboxProcessor(dir, svc, s)
	result, err := proc.Process(cmd.Context())
	if err != nil {
		return fmt.Errorf("inbox process: %w", err)
	}

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "%d created, %d transitioned, %d failed\n",
		len(result.Created),
		len(result.Transitioned),
		len(result.Failed),
	)
	return nil
}

// inboxDir returns the inbox directory path (.tlc/inbox/).
func inboxDir() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ".tlc/inbox"
	}
	return filepath.Join(cwd, ".tlc", "inbox")
}

// autoProcessInbox runs inbox processing if auto_process is enabled
// and the inbox dirs are non-empty. Called from PersistentPreRunE.
func autoProcessInbox(cmd *cobra.Command) {
	if !inboxAutoProcessEnabled() {
		return
	}

	dir := inboxDir()
	if isInboxEmpty(dir) {
		return
	}

	s, err := getStorage()
	if err != nil {
		return
	}

	svc := core.NewTaskService(s, s)
	proc := inbox.NewFileInboxProcessor(dir, svc, s)
	result, err := proc.Process(cmd.Context())
	if err != nil {
		log.Debug("auto inbox process failed", "error", err)
		return
	}

	total := len(result.Created) + len(result.Transitioned)
	if total > 0 {
		log.Debug("auto inbox processed",
			"created", len(result.Created),
			"transitioned", len(result.Transitioned),
			"failed", len(result.Failed),
		)
	}
}

// inboxAutoProcessEnabled checks viper config for
// storage.inbox.auto_process.
func inboxAutoProcessEnabled() bool {
	return viper.GetBool("storage.inbox.auto_process")
}

// isInboxEmpty does a fast check: returns true if both create/ and
// transition/ dirs are empty or don't exist.
func isInboxEmpty(dir string) bool {
	for _, sub := range []string{"create", "transition"} {
		entries, err := os.ReadDir(filepath.Join(dir, sub))
		if err != nil {
			continue
		}
		if len(entries) > 0 {
			return false
		}
	}
	return true
}

func init() {
	InboxCmd.AddCommand(inboxProcessCmd)
	RootCmd.AddCommand(InboxCmd)
}
