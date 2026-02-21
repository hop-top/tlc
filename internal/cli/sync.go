package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/plugin"
	"hop.top/tlc/internal/sync"
)

const (
	syncSystemGitHub   = "github"
	syncDirectionPull  = "pull"
	syncDirectionBidir = "bidirectional"
	syncDirectionPush  = "push"
	syncSystemUnknown  = "unknown"
)

var (
	syncPushDryRun   bool
	syncPushForce    bool
	syncPullStrategy string
	syncConfigForce  bool
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Synchronize tasks with external systems",
}

// autoConfigureGitHub detects GitHub repo and auto-configures sync
// directionHint: suggested direction ("pull", "push", or "bidirectional")
//
//	if current config has different direction, it will be upgraded to bidirectional
func autoConfigureGitHub(directionHint string) error {
	ctx := context.Background()
	// Check if we're in a git repo
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--is-inside-work-tree")
	if err := cmd.Run(); err != nil {
		return nil //nolint:nilerr // Not in a git repo, skip auto-config
	}

	// Get remote URL
	cmd = exec.CommandContext(ctx, "git", "remote", "get-url", "origin")
	output, err := cmd.Output()
	if err != nil {
		return nil //nolint:nilerr // No origin remote, skip auto-config
	}

	remoteURL := strings.TrimSpace(string(output))

	// Parse GitHub repo from URL
	// Supports: https://github.com/owner/repo.git or git@github.com:owner/repo.git
	repo := ""
	if strings.Contains(remoteURL, "github.com") {
		// HTTPS format
		if strings.HasPrefix(remoteURL, "https://github.com/") {
			repo = strings.TrimPrefix(remoteURL, "https://github.com/")
			repo = strings.TrimSuffix(repo, ".git")
		} else if strings.HasPrefix(remoteURL, "git@github.com:") {
			// SSH format
			repo = strings.TrimPrefix(remoteURL, "git@github.com:")
			repo = strings.TrimSuffix(repo, ".git")
		}
	}

	if repo == "" {
		return nil // Not a GitHub repo
	}

	// Check existing configuration
	existingRepo := viper.GetString("sync.github.repo")
	existingDirection := viper.GetString("sync.github.direction")

	// Determine if we need to update configuration
	needsUpdate := false
	newDirection := existingDirection

	if existingRepo == "" {
		// No repo configured, set it
		viper.Set("sync.github.repo", repo)
		needsUpdate = true
		newDirection = directionHint
	} else if existingRepo != repo {
		// Different repo configured, skip
		return nil
	}

	if existingDirection == "" {
		// No direction configured, set from hint
		newDirection = directionHint
		needsUpdate = true
	} else if existingDirection != syncDirectionBidir {
		// Check if we need to upgrade to bidirectional
		if existingDirection == syncDirectionPull && directionHint == syncDirectionPush {
			newDirection = syncDirectionBidir
			needsUpdate = true
		} else if existingDirection == syncDirectionPush && directionHint == syncDirectionPull {
			newDirection = syncDirectionBidir
			needsUpdate = true
		}
	}

	if newDirection != existingDirection {
		viper.Set("sync.github.direction", newDirection)
		needsUpdate = true
	}

	if !needsUpdate {
		return nil // Already configured with correct settings
	}

	// Try to get token from gh CLI if available
	if _, err := exec.LookPath("gh"); err == nil {
		cmd = exec.CommandContext(ctx, "gh", "auth", "token")
		if tokenOutput, err := cmd.Output(); err == nil {
			token := strings.TrimSpace(string(tokenOutput))
			if token != "" {
				_ = os.Setenv("GITHUB_TOKEN", token)
				viper.Set("sync.github.use_gh_auth", true)
			}
		}
	}

	// Save to config file
	// Prefer .tlc/config.yaml in current directory
	configFile := viper.ConfigFileUsed()
	if configFile == "" {
		// Check if .tlc directory exists
		if _, err := os.Stat(".tlc"); os.IsNotExist(err) {
			// No .tlc directory, use .tlc.yaml in current directory
			configFile = ".tlc.yaml"
		} else {
			// .tlc directory exists, use config.yaml inside it
			configFile = filepath.Join(".tlc", "config.yaml")
		}
		viper.SetConfigFile(configFile)
	}

	// Write message BEFORE attempting to save (in case write fails silently)
	if existingRepo == "" && existingDirection == "" {
		fmt.Printf("✓ Auto-configured GitHub sync for repo: %s (%s)\n", repo, newDirection)
	} else if newDirection != existingDirection {
		fmt.Printf("✓ Updated GitHub sync direction from %s to %s\n", existingDirection, newDirection)
	}

	if err := viper.WriteConfig(); err != nil {
		// If config doesn't exist, create it
		if os.IsNotExist(err) || strings.Contains(err.Error(), "Not Found") {
			if err := viper.SafeWriteConfig(); err != nil {
				// Don't fail on config write errors - just warn
				fmt.Fprintf(os.Stderr, "Warning: Could not save config: %v\n", err)
			}
		} else {
			// Don't fail on config write errors - just warn
			fmt.Fprintf(os.Stderr, "Warning: Could not update config: %v\n", err)
		}
	}

	return nil
}

func runSyncPull(cmd *cobra.Command, system string) error {
	// Auto-configure GitHub if needed
	if system == syncSystemGitHub {
		if err := autoConfigureGitHub(syncDirectionPull); err != nil {
			return err
		}
		// Always try to get token from gh if available and not already set
		if os.Getenv("GITHUB_TOKEN") == "" {
			if _, err := exec.LookPath("gh"); err == nil {
				ghCmd := exec.CommandContext(context.Background(), "gh", "auth", "token")
				if tokenOutput, err := ghCmd.Output(); err == nil {
					token := strings.TrimSpace(string(tokenOutput))
					_ = os.Setenv("GITHUB_TOKEN", token)
				}
			}
		}
	}

	s, err := getStorage()
	if err != nil {
		return err
	}
	defer func() { _ = s.Close() }()

	ctx := context.Background()

	// Get max last_sync_at for this system to pass to plugin
	var lastSyncAt string
	tasksForSystem, _ := s.ListTasks(ctx, core.Query{
		Filters: []core.FieldFilter{
			{Field: "origin_system", Value: system},
		},
		SortBy:        "last_sync_at",
		SortDirection: "desc",
		Limit:         1,
	})
	if len(tasksForSystem) > 0 && tasksForSystem[0].LastSyncAt != nil {
		lastSyncAt = tasksForSystem[0].LastSyncAt.Format(time.RFC3339)
	}

	binPath := getPluginPath(system)
	client, err := plugin.NewRPCClient(binPath)
	if err != nil {
		return fmt.Errorf("failed to start plugin %s: %w", system, err)
	}
	defer func() { _ = client.Close() }()

	params := map[string]interface{}{
		"repo":         viper.GetString(fmt.Sprintf("sync.%s.repo", system)),
		"last_sync_at": lastSyncAt,
	}

	var result struct {
		Tasks []core.Task `json:"tasks"`
	}

	err = client.Call("sync.pull", params, &result)
	if err != nil {
		for _, t := range tasksForSystem {
			if logErr := s.AddLog(ctx, &core.LogEntry{
				TaskID:    t.ID,
				Timestamp: time.Now().UTC(),
				By:        "system",
				Action:    "SYNC_ERROR",
				Note:      fmt.Sprintf("Sync pull failed: %v", err),
			}); logErr != nil {
				fmt.Printf("Warning: failed to add log for task %s: %v\n", t.ID, logErr)
			}
		}
		return fmt.Errorf("sync pull RPC failed: %w", err)
	}

	now := time.Now().UTC()
	createdCount := 0
	updatedCount := 0
	conflictCount := 0

	for _, remoteTask := range result.Tasks {
		// Extract origin_system from meta and set it on the task
		if originSystem, ok := remoteTask.Meta["origin_system"].(string); ok {
			remoteTask.OriginSystem = &originSystem
		}

		originID, ok := remoteTask.Meta["origin_id"].(string)
		if !ok {
			continue
		}

		existing, err := s.FindTaskByOrigin(ctx, system, originID)
		if err != nil {
			fmt.Printf("Error searching for existing task: %v\n", err)
			continue
		}

		// Auto-assign project_id if in a project context
		// This ensures tasks from sync pull are properly scoped
		if proj := core.DetectProject(); proj != nil && proj.ProjectID != "" {
			remoteTask.ProjectID = &proj.ProjectID
		}

		if existing == nil {
			// Create new task
			// Assign a new ID if not present
			if remoteTask.ID == "" {
				allTasks, _ := s.ListTasks(ctx, core.Query{})
				remoteTask.ID = fmt.Sprintf("T-%04d", len(allTasks)+1)
			}
			remoteTask.LastSyncAt = &now
			if err := s.CreateTask(ctx, &remoteTask); err != nil {
				fmt.Printf("Error creating task %s: %v\n", remoteTask.ID, err)
			} else {
				createdCount++
				logEntry := &core.LogEntry{
					TaskID:    remoteTask.ID,
					Timestamp: now,
					By:        "system",
					Action:    "SYNC_IMPORTED",
					Note:      fmt.Sprintf("Imported from %s", system),
				}
				if logErr := s.AddLog(ctx, logEntry); logErr != nil {
					fmt.Printf("Warning: failed to add log for task %s: %v\n", remoteTask.ID, logErr)
				}
			}
			continue
		}

		// Detect conflict
		conflict := sync.DetectConflict(existing, &remoteTask)
		if conflict != nil {
			conflictCount++
			fmt.Printf("! Conflict for %s (%s): %s\n", existing.ID, existing.Title, conflict.Description)

			strategy := sync.ConflictStrategy(syncPullStrategy)
			if strategy == "" {
				strategy = sync.StrategyRemoteWins // Default
			}

			var resolved *core.Task
			var updatedLocally bool

			if strategy == sync.StrategyManual {
				resolved, updatedLocally = resolveConflictInteractive(conflict)
			} else {
				resolved, updatedLocally = sync.ResolveConflict(conflict, strategy)
			}

			if updatedLocally {
				resolved.ID = existing.ID               // Preserve local ID
				resolved.ProjectID = existing.ProjectID // Preserve existing project_id
				resolved.LastSyncAt = &now
				if err := s.UpdateTask(ctx, resolved); err != nil {
					fmt.Printf("Error updating task %s: %v\n", resolved.ID, err)
				} else {
					updatedCount++
					logEntry := &core.LogEntry{
						TaskID:    existing.ID,
						Timestamp: now,
						By:        "system",
						Action:    "SYNC_CONFLICT",
						Note:      fmt.Sprintf("Resolved conflict using %s", strategy),
					}
					if logErr := s.AddLog(ctx, logEntry); logErr != nil {
						fmt.Printf("Warning: failed to add log for task %s: %v\n", existing.ID, logErr)
					}
				}
			}
		} else {
			// No conflict, check if remote is newer
			if remoteTask.UpdatedAt.After(existing.UpdatedAt.Add(time.Second)) {
				remoteTask.ID = existing.ID               // Preserve local ID
				remoteTask.ProjectID = existing.ProjectID // Preserve existing project_id
				remoteTask.LastSyncAt = &now
				if err := s.UpdateTask(ctx, &remoteTask); err != nil {
					fmt.Printf("Error updating task %s: %v\n", existing.ID, err)
				} else {
					updatedCount++
					logEntry := &core.LogEntry{
						TaskID:    existing.ID,
						Timestamp: now,
						By:        "system",
						Action:    "SYNC_PULLED",
						Note:      fmt.Sprintf("Updated from %s", system),
					}
					if logErr := s.AddLog(ctx, logEntry); logErr != nil {
						fmt.Printf("Warning: failed to add log for task %s: %v\n", existing.ID, logErr)
					}
				}
			}
		}
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "✓ Sync pull from %s complete: %d created, %d updated, %d conflicts detected\n",
		system, createdCount, updatedCount, conflictCount)

	return syncTODOAll()
}

var syncPullCmd = &cobra.Command{
	Use:   "pull <system>",
	Short: "Pull updates from an external system",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSyncPull(cmd, args[0])
	},
}

var syncPushCmd = &cobra.Command{
	Use:   "push <system>",
	Short: "Push local changes to an external system",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		system := args[0]
		ctx := context.Background()

		// Auto-configure GitHub if needed
		if system == syncSystemGitHub {
			if err := autoConfigureGitHub(syncDirectionPush); err != nil {
				return err
			}
			// Always try to get token from gh if available and not already set
			if os.Getenv("GITHUB_TOKEN") == "" {
				if _, err := exec.LookPath("gh"); err == nil {
					ghCmd := exec.CommandContext(ctx, "gh", "auth", "token")
					if tokenOutput, err := ghCmd.Output(); err == nil {
						token := strings.TrimSpace(string(tokenOutput))
						_ = os.Setenv("GITHUB_TOKEN", token)
					}
				}
			}
		}

		s, err := getStorage()
		if err != nil {
			return err
		}
		defer func() { _ = s.Close() }()

		var tasks []*core.Task
		if syncPushForce {
			// Get all tasks for this system
			query := core.Query{
				Filters: []core.FieldFilter{
					{Field: "origin_system", Value: system},
				},
			}
			tasks, err = s.ListTasks(ctx, query)
			if err != nil {
				return fmt.Errorf("failed to list tasks: %w", err)
			}
		} else {
			allNeedingPush, err := s.GetTasksNeedingPush(ctx)
			if err != nil {
				return fmt.Errorf("failed to get tasks needing push: %w", err)
			}
			for _, t := range allNeedingPush {
				if t.OriginSystem != nil && *t.OriginSystem == system {
					tasks = append(tasks, t)
				}
			}
		}

		if len(tasks) == 0 {
			fmt.Printf("No tasks need pushing to %s\n", system)
			return nil
		}

		if syncPushDryRun {
			fmt.Printf("Dry-run: Would push %d tasks to %s\n", len(tasks), system)
			for _, t := range tasks {
				fmt.Printf("  - %s: %s\n", t.ID, t.Title)
			}
			return nil
		}

		fmt.Printf("Pushing %d tasks to %s...\n", len(tasks), system)

		// Operation-level logging
		for _, t := range tasks {
			if logErr := s.AddLog(ctx, &core.LogEntry{
				TaskID:    t.ID,
				Timestamp: time.Now().UTC(),
				By:        core.GetCurrentUser(),
				Action:    "COMMENT",
				Note:      fmt.Sprintf("Starting sync push to %s", system),
			}); logErr != nil {
				fmt.Printf("Warning: failed to add log for task %s: %v\n", t.ID, logErr)
			}
		}

		binPath := getPluginPath(system)
		client, err := plugin.NewRPCClient(binPath)
		if err != nil {
			return fmt.Errorf("failed to start plugin %s: %w", system, err)
		}
		defer func() { _ = client.Close() }()

		params := map[string]interface{}{
			"repo":  viper.GetString(fmt.Sprintf("sync.%s.repo", system)),
			"tasks": tasks,
		}

		var result struct {
			Updated []string          `json:"updated"`
			Failed  map[string]string `json:"failed"`
		}

		err = client.Call("sync.push", params, &result)
		if err != nil {
			return fmt.Errorf("sync push RPC failed: %w", err)
		}

		now := time.Now().UTC()
		successCount := 0
		for _, id := range result.Updated {
			task, err := s.GetTask(ctx, id)
			if err != nil {
				continue
			}
			task.LastSyncAt = &now
			if err := s.UpdateTask(ctx, task); err != nil {
				fmt.Printf("Error updating task %s: %v\n", id, err)
				continue
			}

			logEntry := &core.LogEntry{
				TaskID:    id,
				Timestamp: now,
				By:        core.GetCurrentUser(),
				Action:    "SYNC_PUSHED",
				Note:      fmt.Sprintf("Changes pushed to %s", system),
			}
			if logErr := s.AddLog(ctx, logEntry); logErr != nil {
				fmt.Printf("Warning: failed to add log for task %s: %v\n", id, logErr)
			}
			successCount++
		}

		fmt.Printf("✓ Successfully pushed %d tasks to %s\n", successCount, system)
		if len(result.Failed) > 0 {
			fmt.Printf("✗ Failed to push %d tasks:\n", len(result.Failed))
			for id, errStr := range result.Failed {
				fmt.Printf("  - %s: %s\n", id, errStr)
				if logErr := s.AddLog(ctx, &core.LogEntry{
					TaskID:    id,
					Timestamp: time.Now().UTC(),
					By:        core.GetCurrentUser(),
					Action:    "SYNC_ERROR",
					Note:      fmt.Sprintf("Failed to push to %s: %s", system, errStr),
				}); logErr != nil {
					fmt.Printf("Warning: failed to add log for task %s: %v\n", id, logErr)
				}
			}
		}

		return syncTODOAll()
	},
}

var syncConfigCmd = &cobra.Command{
	Use:   "config <system>",
	Short: "Configure sync settings for a system",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		system := args[0]
		out := cmd.OutOrStdout()

		// Check if already configured
		repo := viper.GetString(fmt.Sprintf("sync.%s.repo", system))
		direction := viper.GetString(fmt.Sprintf("sync.%s.direction", system))

		if repo != "" && direction != "" && !syncConfigForce {
			return fmt.Errorf("sync for %s is already configured. Use --force to overwrite.\nCurrent config: repo=%s, direction=%s",
				system, repo, direction)
		}

		// Auto-configure for GitHub
		if system == syncSystemGitHub {
			if err := autoConfigureGitHub(syncDirectionBidir); err != nil {
				return err
			}
		}

		// Display configuration
		_, _ = fmt.Fprintf(out, "Sync configuration for %s:\n", system)
		_, _ = fmt.Fprintf(out, "  Repo: %s\n", viper.GetString(fmt.Sprintf("sync.%s.repo", system)))
		_, _ = fmt.Fprintf(out, "  Direction: %s\n", viper.GetString(fmt.Sprintf("sync.%s.direction", system)))

		if system == syncSystemGitHub {
			_, _ = fmt.Fprintf(out, "\nPulling issues from GitHub...\n")
			if err := runSyncPull(cmd, system); err != nil {
				_, _ = fmt.Fprintf(out, "Warning: Initial pull failed: %v\n", err)
			}
		}

		return nil
	},
}

var syncStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show sync status and push queue",
	RunE: func(_ *cobra.Command, _ []string) error {
		s, err := getStorage()
		if err != nil {
			return err
		}
		defer func() { _ = s.Close() }()

		ctx := context.Background()
		tasks, err := s.GetTasksNeedingPush(ctx)
		if err != nil {
			return fmt.Errorf("failed to get tasks needing push: %w", err)
		}

		if len(tasks) == 0 {
			fmt.Println("Push queue is empty. All tasks are in sync.")
			return nil
		}

		fmt.Printf("Push Queue (%d tasks):\n", len(tasks))

		// Group by system
		bySystem := make(map[string][]*core.Task)
		for _, t := range tasks {
			system := syncSystemUnknown
			if t.OriginSystem != nil {
				system = *t.OriginSystem
			}
			bySystem[system] = append(bySystem[system], t)
		}

		for system, tasks := range bySystem {
			fmt.Printf("\n[%s]\n", system)
			for _, t := range tasks {
				fmt.Printf("  %-10s %s\n", t.ID, t.Title)
			}
		}

		return nil
	},
}

func getPluginPath(system string) string {
	// For dev, check local plugins directory relative to binary
	// Get the path to the currently running executable
	execPath, err := os.Executable()
	if err == nil {
		// Resolve symlinks to get the actual binary path
		realPath, err := filepath.EvalSymlinks(execPath)
		if err == nil {
			execPath = realPath
		}
		// Get the directory containing the binary
		binDir := filepath.Dir(execPath)
		// Check if binary is in a "bin" directory (repo structure: repo/bin/tlc)
		if filepath.Base(binDir) == "bin" {
			// Navigate up to repo root, then to plugins
			localPath := filepath.Join(filepath.Dir(binDir), "plugins", system+"-sync", "bin", system+"-sync")
			if _, err := os.Stat(localPath); err == nil {
				return localPath
			}
		}
	}

	// Fallback to config or standard location
	return filepath.Join(os.Getenv("HOME"), ".config", "tlc", "plugins", system+"-sync", "bin", system+"-sync")
}

func resolveConflictInteractive(conflict *sync.Conflict) (*core.Task, bool) {
	var choice string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title(fmt.Sprintf("Conflict detected for %s", conflict.TaskID)).
				Description(fmt.Sprintf("Local: %s (updated: %s)\nRemote: %s (updated: %s)",
					conflict.LocalTask.Title, conflict.LocalTask.UpdatedAt.Format(time.RFC3339),
					conflict.RemoteTask.Title, conflict.RemoteTask.UpdatedAt.Format(time.RFC3339))),
			huh.NewSelect[string]().
				Title("Choose resolution strategy").
				Options(
					huh.NewOption("Remote Wins (Overwrite local changes)", "remote"),
					huh.NewOption("Local Wins (Keep local changes)", "local"),
				).
				Value(&choice),
		),
	)

	if err := form.Run(); err != nil {
		fmt.Printf("Error running form, defaulting to Remote Wins: %v\n", err)
		return conflict.RemoteTask, true
	}

	if choice == "local" {
		return conflict.LocalTask, false
	}
	return conflict.RemoteTask, true
}

func init() {
	syncPullCmd.Flags().StringVar(&syncPullStrategy, "strategy", "remote-wins", "Conflict resolution strategy (remote-wins, local-wins, last-write-wins)")

	syncPushCmd.Flags().BoolVar(&syncPushDryRun, "dry-run", false, "Preview changes without pushing")
	syncPushCmd.Flags().BoolVar(&syncPushForce, "force", false, "Push all tasks even if not modified locally")

	syncConfigCmd.Flags().BoolVar(&syncConfigForce, "force", false, "Overwrite existing configuration")

	syncCmd.AddCommand(syncPullCmd)
	syncCmd.AddCommand(syncPushCmd)
	syncCmd.AddCommand(syncConfigCmd)
	syncCmd.AddCommand(syncStatusCmd)
	rootCmd.AddCommand(syncCmd)
}
