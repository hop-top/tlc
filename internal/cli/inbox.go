package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"charm.land/log/v2"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/inbox"
	"hop.top/tlc/internal/storage"
)

var InboxCmd = &cobra.Command{
	Use:   "inbox",
	Short: "Manage the file-based inbox",
}

var inboxProcessCmd = &cobra.Command{
	Use:   "process",
	Short: "Process pending inbox files",
	Long: `Scan inbox create/ and transition/ dirs for pending
files, create tasks or transition statuses, and move
processed files to processed/ or failed/.`,
	Annotations: map[string]string{
		"kit/side-effect": "write-local",
		"kit/idempotent":  "yes",
	},
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
	defer func() { _ = s.Close() }()

	result, err := processInbox(cmd, s)
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

// processInbox runs the inbox processor against the given storage.
func processInbox(
	cmd *cobra.Command, s *storage.SQLiteStorage,
) (*inbox.InboxResult, error) {
	svc := core.NewTaskService(s, s)
	dir := inboxDir()
	projectID := resolveProjectID()
	proc := inbox.NewFileInboxProcessor(dir, svc, projectID)
	return proc.Process(cmd.Context())
}

// inboxDir returns the inbox directory path using the detected
// config mode (supports .tlc/ and .hop/tlc/).
func inboxDir() string {
	dir := inboxDirName()
	cwd, err := os.Getwd()
	if err != nil {
		return filepath.Join(
			config.LocalConfigDir(config.DetectMode()),
			dir,
		)
	}
	return filepath.Join(
		cwd,
		config.LocalConfigDir(config.DetectMode()),
		dir,
	)
}

// resolveProjectID returns the current project ID if detected.
func resolveProjectID() string {
	det := core.DetectProject()
	if det != nil && det.InProject {
		return det.ProjectID
	}
	return ""
}

// autoProcessInbox runs inbox processing if auto_process is enabled
// and the inbox dirs are non-empty. Called from PersistentPreRunE.
// Skips when the current command is already `inbox process`.
func autoProcessInbox(
	cmd *cobra.Command, s *storage.SQLiteStorage,
) {
	if isInboxProcessCmd(cmd) {
		return
	}
	if !inboxAutoProcessEnabled() {
		return
	}

	dir := inboxDir()
	if isInboxEmpty(dir) {
		return
	}

	result, err := processInbox(cmd, s)
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

// isInboxProcessCmd returns true if cmd is `tlc inbox process`.
func isInboxProcessCmd(cmd *cobra.Command) bool {
	return strings.HasSuffix(cmd.CommandPath(), "inbox process")
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
