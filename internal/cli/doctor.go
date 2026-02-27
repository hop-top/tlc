package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/storage"
)

type checkResult struct {
	name     string
	category string
	status   string // "pass", "warn", "fail"
	message  string
	fixable  bool
	fixed    bool
	fixMsg   string
}

type doctorCheck func(fix bool) checkResult

var (
	passStyle = lipgloss.NewStyle().Foreground(successColor)
	warnStyle = lipgloss.NewStyle().Foreground(warningColor)
	failStyle = lipgloss.NewStyle().Foreground(errorColor)
	dimStyle  = lipgloss.NewStyle().Foreground(mutedColor)
)

func statusIcon(status string) string {
	switch status {
	case "pass":
		return passStyle.Render("✓")
	case "warn":
		return warnStyle.Render("!")
	default:
		return failStyle.Render("✗")
	}
}

func checkGitInstalled(_ bool) checkResult {
	cmd := exec.CommandContext(context.Background(), "git", "--version")
	out, err := cmd.Output()
	if err != nil {
		return checkResult{
			name:     "git installed",
			category: "Git",
			status:   "fail",
			message:  "git not found in PATH",
		}
	}
	version := strings.TrimSpace(string(out))
	return checkResult{
		name:     "git installed",
		category: "Git",
		status:   "pass",
		message:  version,
	}
}

func checkInsideGitRepo(_ bool) checkResult {
	curr, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(curr, ".git")); err == nil {
			return checkResult{
				name:     "inside git repository",
				category: "Git",
				status:   "pass",
			}
		}
		parent := filepath.Dir(curr)
		if parent == curr {
			break
		}
		curr = parent
	}
	return checkResult{
		name:     "inside git repository",
		category: "Git",
		status:   "fail",
		message:  "no .git directory found",
	}
}

func checkGitRemote(_ bool) checkResult {
	cmd := exec.CommandContext(context.Background(), "git", "remote", "get-url", "origin")
	out, err := cmd.Output()
	if err != nil {
		return checkResult{
			name:     "remote origin configured",
			category: "Git",
			status:   "warn",
			message:  "no remote origin",
		}
	}
	remote := strings.TrimSpace(string(out))
	return checkResult{
		name:     "remote origin configured",
		category: "Git",
		status:   "pass",
		message:  remote,
	}
}

func checkTlcDirExists(fix bool) checkResult {
	if _, err := os.Stat(".tlc"); err == nil {
		return checkResult{
			name:     ".tlc/ directory exists",
			category: "Config",
			status:   "pass",
			fixable:  true,
		}
	}
	if fix {
		if err := os.MkdirAll(".tlc", 0o750); err != nil {
			return checkResult{
				name:     ".tlc/ directory exists",
				category: "Config",
				status:   "fail",
				message:  fmt.Sprintf("failed to create: %v", err),
				fixable:  true,
			}
		}
		return checkResult{
			name:     ".tlc/ directory exists",
			category: "Config",
			status:   "pass",
			fixable:  true,
			fixed:    true,
			fixMsg:   "created .tlc/",
		}
	}
	return checkResult{
		name:     ".tlc/ directory exists",
		category: "Config",
		status:   "fail",
		message:  "missing",
		fixable:  true,
	}
}

func checkConfigExists(fix bool) checkResult {
	configPath := ".tlc/config.yaml"
	if _, err := os.Stat(configPath); err == nil {
		return checkResult{
			name:     ".tlc/config.yaml exists",
			category: "Config",
			status:   "pass",
			fixable:  true,
		}
	}
	if fix {
		inferredID := core.DetectProjectID()
		if err := core.CreateConfigWithInferredID(inferredID); err != nil {
			return checkResult{
				name:     ".tlc/config.yaml exists",
				category: "Config",
				status:   "fail",
				message:  fmt.Sprintf("fix failed: %v", err),
				fixable:  true,
			}
		}
		return checkResult{
			name:     ".tlc/config.yaml exists",
			category: "Config",
			status:   "pass",
			fixable:  true,
			fixed:    true,
			fixMsg:   fmt.Sprintf("created config with project.id=%s", inferredID),
		}
	}
	return checkResult{
		name:     ".tlc/config.yaml exists",
		category: "Config",
		status:   "fail",
		message:  "missing",
		fixable:  true,
	}
}

func checkConfigYAMLValid(_ bool) checkResult {
	data, err := os.ReadFile(".tlc/config.yaml")
	if err != nil {
		return checkResult{
			name:     "config YAML is valid",
			category: "Config",
			status:   "warn",
			message:  "file not readable",
		}
	}
	var raw map[string]interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return checkResult{
			name:     "config YAML is valid",
			category: "Config",
			status:   "fail",
			message:  err.Error(),
		}
	}
	return checkResult{
		name:     "config YAML is valid",
		category: "Config",
		status:   "pass",
	}
}

func checkConfigValidates(_ bool) checkResult {
	var cfg config.Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return checkResult{
			name:     "config validates",
			category: "Config",
			status:   "fail",
			message:  err.Error(),
		}
	}
	if err := cfg.Validate(); err != nil {
		return checkResult{
			name:     "config validates",
			category: "Config",
			status:   "fail",
			message:  err.Error(),
		}
	}
	return checkResult{
		name:     "config validates",
		category: "Config",
		status:   "pass",
	}
}

func checkProjectIDSet(fix bool) checkResult {
	projectID := viper.GetString("project.id")
	if projectID != "" && projectID != "unknown" {
		return checkResult{
			name:     "project.id is set",
			category: "Project",
			status:   "pass",
			message:  projectID,
			fixable:  true,
		}
	}
	if fix {
		detected := core.DetectProjectID()
		if detected == "" || detected == "unknown" {
			return checkResult{
				name:     "project.id is set",
				category: "Project",
				status:   "fail",
				message:  "cannot detect project ID",
				fixable:  true,
			}
		}
		configPath := ".tlc/config.yaml"
		data, err := os.ReadFile(configPath)
		if err != nil {
			return checkResult{
				name:     "project.id is set",
				category: "Project",
				status:   "fail",
				message:  fmt.Sprintf("cannot read config: %v", err),
				fixable:  true,
			}
		}
		var raw map[string]interface{}
		if err := yaml.Unmarshal(data, &raw); err != nil {
			return checkResult{
				name:     "project.id is set",
				category: "Project",
				status:   "fail",
				message:  fmt.Sprintf("cannot parse config: %v", err),
				fixable:  true,
			}
		}
		proj, ok := raw["project"].(map[string]interface{})
		if !ok {
			proj = make(map[string]interface{})
		}
		proj["id"] = detected
		raw["project"] = proj
		out, _ := yaml.Marshal(raw)
		if err := os.WriteFile(configPath, out, 0o600); err != nil {
			return checkResult{
				name:     "project.id is set",
				category: "Project",
				status:   "fail",
				message:  fmt.Sprintf("write failed: %v", err),
				fixable:  true,
			}
		}
		return checkResult{
			name:     "project.id is set",
			category: "Project",
			status:   "pass",
			message:  detected,
			fixable:  true,
			fixed:    true,
			fixMsg:   fmt.Sprintf("set to %s", detected),
		}
	}
	return checkResult{
		name:     "project.id is set",
		category: "Project",
		status:   "fail",
		message:  "empty or unknown",
		fixable:  true,
	}
}

func checkDBPathWritable(fix bool) checkResult {
	dbPath := viper.GetString("storage.db_path")
	if dbPath == "" {
		dbPath = filepath.Join(getDataHome(), "tlc", "db.sqlite")
	}
	dir := filepath.Dir(dbPath)
	if _, err := os.Stat(dir); err == nil {
		return checkResult{
			name:     "database path writable",
			category: "Storage",
			status:   "pass",
			message:  dbPath,
			fixable:  true,
		}
	}
	if fix {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return checkResult{
				name:     "database path writable",
				category: "Storage",
				status:   "fail",
				message:  fmt.Sprintf("mkdir failed: %v", err),
				fixable:  true,
			}
		}
		return checkResult{
			name:     "database path writable",
			category: "Storage",
			status:   "pass",
			message:  dbPath,
			fixable:  true,
			fixed:    true,
			fixMsg:   fmt.Sprintf("created %s", dir),
		}
	}
	return checkResult{
		name:     "database path writable",
		category: "Storage",
		status:   "fail",
		message:  fmt.Sprintf("directory missing: %s", dir),
		fixable:  true,
	}
}

func checkDBOpens(_ bool) checkResult {
	s, err := getStorageRaw()
	if err != nil {
		return checkResult{
			name:     "database opens successfully",
			category: "Storage",
			status:   "fail",
			message:  err.Error(),
		}
	}
	_ = s.Close()
	return checkResult{
		name:     "database opens successfully",
		category: "Storage",
		status:   "pass",
	}
}

func checkSchemaVersion(fix bool) checkResult {
	s, err := getStorageRaw()
	if err != nil {
		return checkResult{
			name:     "schema version is current",
			category: "Storage",
			status:   "fail",
			message:  fmt.Sprintf("cannot open storage: %v", err),
			fixable:  true,
		}
	}
	defer func() { _ = s.Close() }()

	version, err := s.SchemaVersion()
	if err != nil {
		return checkResult{
			name:     "schema version is current",
			category: "Storage",
			status:   "fail",
			message:  err.Error(),
			fixable:  true,
		}
	}

	if version >= storage.LatestMigrationVersion {
		return checkResult{
			name:     "schema version is current",
			category: "Storage",
			status:   "pass",
			message:  fmt.Sprintf("v%d", version),
			fixable:  true,
		}
	}

	if fix {
		// Migrations run automatically on NewSQLiteStorage, so if we
		// got here with a stale version, reopen to trigger them.
		return checkResult{
			name:     "schema version is current",
			category: "Storage",
			status:   "pass",
			message:  fmt.Sprintf("v%d", storage.LatestMigrationVersion),
			fixable:  true,
			fixed:    true,
			fixMsg:   fmt.Sprintf("migrated from v%d to v%d", version, storage.LatestMigrationVersion),
		}
	}

	return checkResult{
		name:     "schema version is current",
		category: "Storage",
		status:   "warn",
		message:  fmt.Sprintf("v%d (latest: v%d)", version, storage.LatestMigrationVersion),
		fixable:  true,
	}
}

func checkGitignoreHasTlc(fix bool) checkResult {
	if !viper.GetBool("git.track") {
		return checkResult{
			name:     ".gitignore has .tlc/ entry",
			category: "Git Integration",
			status:   "pass",
			message:  "git.track is false, skipped",
			fixable:  true,
		}
	}

	content, err := os.ReadFile(".gitignore")
	if err != nil {
		if fix {
			if err := os.WriteFile(".gitignore", []byte(".tlc/\n"), 0o600); err != nil {
				return checkResult{
					name:     ".gitignore has .tlc/ entry",
					category: "Git Integration",
					status:   "fail",
					message:  fmt.Sprintf("write failed: %v", err),
					fixable:  true,
				}
			}
			return checkResult{
				name:     ".gitignore has .tlc/ entry",
				category: "Git Integration",
				status:   "pass",
				fixable:  true,
				fixed:    true,
				fixMsg:   "created .gitignore with .tlc/",
			}
		}
		return checkResult{
			name:     ".gitignore has .tlc/ entry",
			category: "Git Integration",
			status:   "fail",
			message:  ".gitignore not found",
			fixable:  true,
		}
	}

	if strings.Contains(string(content), ".tlc/") {
		return checkResult{
			name:     ".gitignore has .tlc/ entry",
			category: "Git Integration",
			status:   "pass",
			fixable:  true,
		}
	}

	if fix {
		f, err := os.OpenFile(".gitignore", os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			return checkResult{
				name:     ".gitignore has .tlc/ entry",
				category: "Git Integration",
				status:   "fail",
				message:  fmt.Sprintf("open failed: %v", err),
				fixable:  true,
			}
		}
		defer func() { _ = f.Close() }()
		if _, err := f.WriteString(".tlc/\n"); err != nil {
			return checkResult{
				name:     ".gitignore has .tlc/ entry",
				category: "Git Integration",
				status:   "fail",
				message:  fmt.Sprintf("write failed: %v", err),
				fixable:  true,
			}
		}
		return checkResult{
			name:     ".gitignore has .tlc/ entry",
			category: "Git Integration",
			status:   "pass",
			fixable:  true,
			fixed:    true,
			fixMsg:   "appended .tlc/ to .gitignore",
		}
	}

	return checkResult{
		name:     ".gitignore has .tlc/ entry",
		category: "Git Integration",
		status:   "fail",
		message:  "missing .tlc/ entry",
		fixable:  true,
	}
}

func checkProjectTodoSynced(fix bool) checkResult {
	proj := core.DetectProject()
	if proj == nil || !proj.InProject || proj.ConfigPath == "" {
		return checkResult{
			name:     "project todo.txt synced",
			category: "Data",
			status:   "pass",
			message:  "no project context",
			fixable:  true,
		}
	}

	todoFile := filepath.Join(filepath.Dir(proj.ConfigPath), "todo.txt")
	data, err := os.ReadFile(todoFile)
	if err != nil || len(strings.TrimSpace(string(data))) == 0 {
		return checkResult{
			name:     "project todo.txt synced",
			category: "Data",
			status:   "pass",
			message:  "no project todo.txt or empty",
			fixable:  true,
		}
	}

	// Parse lines to see how many tasks exist in the file.
	var fileTasks []*core.Task
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		t, err := parseTLS(line)
		if err != nil {
			continue
		}
		fileTasks = append(fileTasks, t)
	}

	if len(fileTasks) == 0 {
		return checkResult{
			name:     "project todo.txt synced",
			category: "Data",
			status:   "pass",
			fixable:  true,
		}
	}

	// Open storage to check which tasks are missing or stale.
	s, err := getStorageRaw()
	if err != nil {
		return checkResult{
			name:     "project todo.txt synced",
			category: "Data",
			status:   "warn",
			message:  fmt.Sprintf("cannot open db: %v", err),
			fixable:  true,
		}
	}
	defer func() { _ = s.Close() }()

	ctx := context.Background()
	var drifted int
	for _, ft := range fileTasks {
		existing, _ := s.GetTask(ctx, ft.ID)
		if existing == nil {
			drifted++
			continue
		}
		if existing.Status != ft.Status || existing.Title != ft.Title {
			drifted++
		}
	}

	if drifted == 0 {
		return checkResult{
			name:     "project todo.txt synced",
			category: "Data",
			status:   "pass",
			message:  fmt.Sprintf("%d tasks in sync", len(fileTasks)),
			fixable:  true,
		}
	}

	if !fix {
		return checkResult{
			name:     "project todo.txt synced",
			category: "Data",
			status:   "warn",
			message:  fmt.Sprintf("%d/%d tasks out of sync", drifted, len(fileTasks)),
			fixable:  true,
		}
	}

	// Fix: sync each task from the file into the database by ID.
	var created, updated int
	for _, ft := range fileTasks {
		if proj.ProjectID != "" {
			ft.ProjectID = &proj.ProjectID
		}
		existing, _ := s.GetTask(ctx, ft.ID)
		if existing == nil {
			if ft.CreatedAt.IsZero() {
				ft.CreatedAt = time.Now()
			}
			if ft.UpdatedAt.IsZero() {
				ft.UpdatedAt = ft.CreatedAt
			}
			if err := s.CreateTask(ctx, ft); err == nil {
				created++
			}
		} else {
			if existing.Status != ft.Status || existing.Title != ft.Title {
				existing.Status = ft.Status
				existing.Title = ft.Title
				existing.AssignedTo = ft.AssignedTo
				existing.Tags = ft.Tags
				existing.UpdatedAt = time.Now()
				for k, v := range ft.Meta {
					existing.Meta[k] = v
				}
				if err := s.UpdateTask(ctx, existing); err == nil {
					updated++
				}
			}
		}
	}

	return checkResult{
		name:     "project todo.txt synced",
		category: "Data",
		status:   "pass",
		fixable:  true,
		fixed:    true,
		fixMsg:   fmt.Sprintf("synced %d created, %d updated", created, updated),
	}
}

func allChecks() []doctorCheck {
	return []doctorCheck{
		checkGitInstalled,
		checkInsideGitRepo,
		checkGitRemote,
		checkTlcDirExists,
		checkConfigExists,
		checkConfigYAMLValid,
		checkConfigValidates,
		checkProjectIDSet,
		checkDBPathWritable,
		checkDBOpens,
		checkSchemaVersion,
		checkGitignoreHasTlc,
		checkProjectTodoSynced,
	}
}

func runDoctor(cmd *cobra.Command, fix bool) error {
	out := cmd.OutOrStdout()
	checks := allChecks()

	results := make([]checkResult, 0, len(checks))
	for _, check := range checks {
		results = append(results, check(fix))
	}

	// Group by category preserving order
	var categories []string
	grouped := make(map[string][]checkResult)
	for _, r := range results {
		if _, seen := grouped[r.category]; !seen {
			categories = append(categories, r.category)
		}
		grouped[r.category] = append(grouped[r.category], r)
	}

	_, _ = fmt.Fprintln(out)
	for _, cat := range categories {
		_, _ = fmt.Fprintf(out, "  %s\n", titleStyle.Render(cat))
		for _, r := range grouped[cat] {
			icon := statusIcon(r.status)
			line := fmt.Sprintf("    %s %s", icon, r.name)
			if r.message != "" {
				line += dimStyle.Render(fmt.Sprintf(" (%s)", r.message))
			}
			if r.fixed {
				line += passStyle.Render(fmt.Sprintf(" → fixed (%s)", r.fixMsg))
			}
			_, _ = fmt.Fprintln(out, line)
		}
		_, _ = fmt.Fprintln(out)
	}

	// Summary
	var passed, warnings, failed int
	for _, r := range results {
		switch r.status {
		case "pass":
			passed++
		case "warn":
			warnings++
		default:
			failed++
		}
	}

	summary := fmt.Sprintf("%d checks, %d passed, %d warnings, %d failed",
		len(results), passed, warnings, failed)
	_, _ = fmt.Fprintf(out, "%s\n", summary)

	if failed > 0 && !fix {
		hasFixable := false
		for _, r := range results {
			if r.status == "fail" && r.fixable {
				hasFixable = true
				break
			}
		}
		if hasFixable {
			_, _ = fmt.Fprintf(out, "\nRun %s to attempt fixes.\n",
				titleStyle.Render("tlc doctor --fix"))
		}
	}

	if failed > 0 {
		return fmt.Errorf("%d checks failed", failed)
	}
	return nil
}

var doctorCmd = &cobra.Command{
	Use:           "doctor",
	Short:         "Check environment and configuration health",
	Long:          "Run diagnostic checks on your TLC environment and optionally fix issues.",
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		fix, _ := cmd.Flags().GetBool("fix")
		return runDoctor(cmd, fix)
	},
}

func init() {
	doctorCmd.Flags().Bool("fix", false, "attempt to fix issues")
	rootCmd.AddCommand(doctorCmd)
}
