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
	"github.com/google/oss-tlc-cli/internal/core"
	"github.com/google/oss-tlc-cli/internal/plugin"
	"github.com/google/oss-tlc-cli/internal/sync"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	syncPushDryRun   bool
	syncPushForce    bool
	syncPullStrategy string
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Synchronize tasks with external systems",
}

// autoConfigureGitHub detects GitHub repo and auto-configures sync if not already set
func autoConfigureGitHub() error {
	// Check if already configured (must have both repo and direction set)
	repo := viper.GetString("sync.github.repo")
	direction := viper.GetString("sync.github.direction")
	if repo != "" && direction != "" {
		return nil // Already configured
	}

	// Check if we're in a git repo
	cmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	if err := cmd.Run(); err != nil {
		return nil // Not in a git repo, skip auto-config
	}

	// Get remote URL
	cmd = exec.Command("git", "remote", "get-url", "origin")
	output, err := cmd.Output()
	if err != nil {
		return nil // No origin remote, skip auto-config
	}

	remoteURL := strings.TrimSpace(string(output))

	// Parse GitHub repo from URL
	// Supports: https://github.com/owner/repo.git or git@github.com:owner/repo.git
	repo = "" // Reset repo for parsing
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

	// Auto-configure with bidirectional sync
	viper.Set("sync.github.repo", repo)
	viper.Set("sync.github.direction", "bidirectional")

	// Try to get token from gh CLI if available
	if _, err := exec.LookPath("gh"); err == nil {
		cmd = exec.Command("gh", "auth", "token")
		if tokenOutput, err := cmd.Output(); err == nil {
			token := strings.TrimSpace(string(tokenOutput))
			if token != "" {
				// Store token in environment for this session
				os.Setenv("GITHUB_TOKEN", token)
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
	fmt.Printf("✓ Auto-configured GitHub sync for repo: %s (bidirectional)\n", repo)

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

var syncPullCmd = &cobra.Command{
	Use:   "pull <system>",
	Short: "Pull updates from an external system",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		system := args[0]

		// Auto-configure GitHub if needed
		if system == "github" {
			if err := autoConfigureGitHub(); err != nil {
				return err
			}
			// Always try to get token from gh if available and not already set
			if os.Getenv("GITHUB_TOKEN") == "" {
				if _, err := exec.LookPath("gh"); err == nil {
					ghCmd := exec.Command("gh", "auth", "token")
					if tokenOutput, err := ghCmd.Output(); err == nil {
						token := strings.TrimSpace(string(tokenOutput))
						os.Setenv("GITHUB_TOKEN", token)
					}
				}
			}
		}

		s, err := getStorage()
		if err != nil {
			return err
		}
		defer s.Close()

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
		defer client.Close()

		params := map[string]interface{}{
			"repo":         viper.GetString(fmt.Sprintf("sync.%s.repo", system)),
			"last_sync_at": lastSyncAt,
		}

		var result struct {
			Tasks []core.Task `json:"tasks"`
		}

		err = client.Call("sync.pull", params, &result)
		if err != nil {
			// Log error for tasks of this system
			for _, t := range tasksForSystem {
				s.AddLog(ctx, &core.LogEntry{
					TaskID:    t.ID,
					Timestamp: time.Now().UTC(),
					By:        "system",
					Action:    "SYNC_ERROR",
					Note:      fmt.Sprintf("Sync pull failed: %v", err),
				})
			}
			return fmt.Errorf("sync pull RPC failed: %w", err)
		}

		now := time.Now().UTC()
		createdCount := 0
		updatedCount := 0
		conflictCount := 0

		for _, remoteTask := range result.Tasks {
			originID, ok := remoteTask.Meta["origin_id"].(string)
			if !ok {
				continue
			}

			existing, err := s.FindTaskByOrigin(ctx, system, originID)
			if err != nil {
				fmt.Printf("Error searching for existing task: %v\n", err)
				continue
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
					s.AddLog(ctx, logEntry)
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
					resolved.ID = existing.ID // Preserve local ID
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
						s.AddLog(ctx, logEntry)
					}
				}
			} else {
				// No conflict, check if remote is newer
				if remoteTask.UpdatedAt.After(existing.UpdatedAt.Add(time.Second)) {
					remoteTask.ID = existing.ID // Preserve local ID
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
						s.AddLog(ctx, logEntry)
					}
				}
			}
		}

		fmt.Fprintf(cmd.OutOrStdout(), "✓ Sync pull from %s complete: %d created, %d updated, %d conflicts detected\n", 
			system, createdCount, updatedCount, conflictCount)
		
		return syncToTODO()
	},
}

var syncPushCmd = &cobra.Command{
	Use:   "push <system>",
	Short: "Push local changes to an external system",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		system := args[0]

		// Auto-configure GitHub if needed
		if system == "github" {
			if err := autoConfigureGitHub(); err != nil {
				return err
			}
			// Always try to get token from gh if available and not already set
			if os.Getenv("GITHUB_TOKEN") == "" {
				if _, err := exec.LookPath("gh"); err == nil {
					ghCmd := exec.Command("gh", "auth", "token")
					if tokenOutput, err := ghCmd.Output(); err == nil {
						token := strings.TrimSpace(string(tokenOutput))
						os.Setenv("GITHUB_TOKEN", token)
					}
				}
			}
		}

		s, err := getStorage()
		if err != nil {
			return err
		}
		defer s.Close()

		ctx := context.Background()
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
				return err
			}
		} else {
			allNeedingPush, err := s.GetTasksNeedingPush(ctx)
			if err != nil {
				return err
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
			s.AddLog(ctx, &core.LogEntry{
				TaskID:    t.ID,
				Timestamp: time.Now().UTC(),
				By:        core.GetCurrentUser(),
				Action:    "COMMENT",
				Note:      fmt.Sprintf("Starting sync push to %s", system),
			})
		}

		binPath := getPluginPath(system)
		client, err := plugin.NewRPCClient(binPath)
		if err != nil {
			return fmt.Errorf("failed to start plugin %s: %w", system, err)
		}
		defer client.Close()

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
			s.AddLog(ctx, logEntry)
			successCount++
		}

		fmt.Printf("✓ Successfully pushed %d tasks to %s\n", successCount, system)
		if len(result.Failed) > 0 {
			fmt.Printf("✗ Failed to push %d tasks:\n", len(result.Failed))
			for id, errStr := range result.Failed {
				fmt.Printf("  - %s: %s\n", id, errStr)
				s.AddLog(ctx, &core.LogEntry{
					TaskID:    id,
					Timestamp: time.Now().UTC(),
					By:        core.GetCurrentUser(),
					Action:    "SYNC_ERROR",
					Note:      fmt.Sprintf("Failed to push to %s: %s", system, errStr),
				})
			}
		}

		return syncToTODO()
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

var syncStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show sync status and push queue",
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := getStorage()
		if err != nil {
			return err
		}
		defer s.Close()

		ctx := context.Background()
		tasks, err := s.GetTasksNeedingPush(ctx)
		if err != nil {
			return err
		}

		if len(tasks) == 0 {
			fmt.Println("Push queue is empty. All tasks are in sync.")
			return nil
		}

		fmt.Printf("Push Queue (%d tasks):\n", len(tasks))
		
		// Group by system
		bySystem := make(map[string][]*core.Task)
		for _, t := range tasks {
			system := "unknown"
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
	// For dev, check local plugins directory
	localPath := filepath.Join("plugins", system+"-sync", "bin", system+"-sync")
	if _, err := os.Stat(localPath); err == nil {
		return localPath
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

	syncCmd.AddCommand(syncPullCmd)
	syncCmd.AddCommand(syncPushCmd)
	syncCmd.AddCommand(syncConfigCmd)
	syncCmd.AddCommand(syncStatusCmd)
	rootCmd.AddCommand(syncCmd)
}
