package core

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"charm.land/huh/v2"
	"charm.land/log/v2"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"

	"hop.top/tlc/internal/config"
)

const (
	projectIDUnknown = "unknown"
	fallbackModeAuto = "auto"
)

var (
	cachedDetection *ProjectDetection
	detectionOnce   sync.Once
)

type ProjectDetection struct {
	ProjectID  string
	ConfigPath string
	InProject  bool
}

func DetectProject() *ProjectDetection {
	detectionOnce.Do(func() {
		cachedDetection = detectProjectOnce()
	})
	return cachedDetection
}

// ResetDetectionCache clears the cached detection result for testing.
func ResetDetectionCache() {
	detectionOnce = sync.Once{}
	cachedDetection = nil
}

func detectProjectOnce() *ProjectDetection {
	checkConfigPath := viper.GetString("config")
	if checkConfigPath != "" {
		viper.SetConfigFile(checkConfigPath)
		viper.SetConfigType("yaml")
		_ = viper.ReadInConfig() //nolint:errcheck // best-effort config load
	}

	configPath := viper.ConfigFileUsed()

	if configPath == "" {
		return handleFallbackMode()
	}

	dotTlcDir := filepath.Dir(configPath)
	tlcConfigPath := filepath.Join(dotTlcDir, "config.yaml")

	if _, err := os.Stat(tlcConfigPath); os.IsNotExist(err) {
		return handleFallbackMode()
	}

	viper.SetConfigFile(tlcConfigPath)
	viper.SetConfigType("yaml")
	if err := viper.ReadInConfig(); err != nil {
		return &ProjectDetection{
			InProject: false,
		}
	}

	projectID := viper.GetString("project.id")
	if projectID == "" {
		projectID = DetectProjectID()
		viper.Set("project.id", projectID)
		_ = viper.WriteConfig() //nolint:errcheck // best-effort config persist
	}

	return &ProjectDetection{
		ProjectID:  projectID,
		ConfigPath: tlcConfigPath,
		InProject:  true,
	}
}

// DetectProjectID returns a project identifier using a fallback chain:
// git remote origin > git toplevel directory > filesystem walk.
func DetectProjectID() string {
	if detected := DetectFromGitRemote(); detected != "" {
		return detected
	}
	if detected := detectFromGitToplevel(); detected != "" {
		return detected
	}
	if detected := detectFromDirectory(); detected != "" && detected != projectIDUnknown {
		return detected
	}
	return projectIDUnknown
}

func DetectFromGitRemote() string {
	cmd := exec.CommandContext(context.Background(), "git", "remote", "get-url", "origin")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	remote := strings.TrimSpace(string(output))

	if strings.Contains(remote, "github.com") {
		re := regexp.MustCompile(`github\.com[:/](.+?)(?:\.git)?$`)
		matches := re.FindStringSubmatch(remote)
		if len(matches) > 1 {
			return matches[1]
		}
	}
	if strings.Contains(remote, "gitlab.com") {
		re := regexp.MustCompile(`gitlab\.com[:/](.+?)(?:\.git)?$`)
		matches := re.FindStringSubmatch(remote)
		if len(matches) > 1 {
			return matches[1]
		}
	}
	if strings.Contains(remote, "bitbucket.org") {
		re := regexp.MustCompile(`bitbucket\.org[:/](.+?)(?:\.git)?$`)
		matches := re.FindStringSubmatch(remote)
		if len(matches) > 1 {
			return matches[1]
		}
	}
	return ""
}

func detectFromGitToplevel() string {
	cmd := exec.CommandContext(context.Background(), "git", "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	toplevel := strings.TrimSpace(string(output))
	if toplevel == "" {
		return ""
	}
	return filepath.Base(toplevel)
}

func detectFromDirectory() string {
	cwd, err := os.Getwd()
	if err != nil {
		return projectIDUnknown
	}

	for {
		if _, err := os.Stat(filepath.Join(cwd, ".git")); err == nil {
			return filepath.Base(cwd)
		}

		newCwd := filepath.Dir(cwd)
		if newCwd == cwd {
			break
		}
		cwd = newCwd
	}

	return projectIDUnknown
}

func handleFallbackMode() *ProjectDetection {
	mode := viper.GetString("project.fallback_mode")
	if mode == "" {
		mode = fallbackModeAuto
	}

	inferredID := DetectProjectID()
	if inferredID == "" || inferredID == projectIDUnknown {
		return &ProjectDetection{InProject: false}
	}

	entryMode := config.DetectMode()
	configPath := filepath.Join(config.LocalConfigDir(entryMode), "config.yaml")

	switch mode {
	case fallbackModeAuto:
		if err := CreateConfigWithInferredID(inferredID); err == nil {
			return &ProjectDetection{
				ProjectID:  inferredID,
				ConfigPath: configPath,
				InProject:  true,
			}
		}
		return &ProjectDetection{
			ProjectID:  inferredID,
			ConfigPath: "",
			InProject:  true,
		}

	case "detected":
		return &ProjectDetection{
			ProjectID:  inferredID,
			ConfigPath: "",
			InProject:  true,
		}

	case "prompt":
		choice, err := promptFallbackMode(inferredID)
		if err != nil || choice == "auto" {
			if createErr := CreateConfigWithInferredID(inferredID); createErr == nil {
				return &ProjectDetection{
					ProjectID:  inferredID,
					ConfigPath: configPath,
					InProject:  true,
				}
			}
		}
		return &ProjectDetection{
			ProjectID:  inferredID,
			ConfigPath: "",
			InProject:  true,
		}

	default:
		return &ProjectDetection{InProject: false}
	}
}

// configHasProjectID reports whether the YAML at path declares
// project.id == projectID. Returns false for missing or unparseable
// files (the caller treats absence as "no claim").
func configHasProjectID(path, projectID string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var existing map[string]interface{}
	if yaml.Unmarshal(data, &existing) != nil {
		return false
	}
	if proj, ok := existing["project"].(map[interface{}]interface{}); ok {
		if id, _ := proj["id"].(string); id == projectID {
			return true
		}
	}
	if proj, ok := existing["project"].(map[string]interface{}); ok {
		if id, _ := proj["id"].(string); id == projectID {
			return true
		}
	}
	return false
}

// canonicalConfigClaimsProject walks up from cwd looking for any
// existing local config (across both .tlc/ and .hop/tlc/ layouts,
// flat-file and dir variants) that already declares projectID. Used
// by CreateConfigWithInferredID to suppress recreating a mode's
// config when another mode's canonical config exists at or above cwd
// with the same project ID (T-1303).
//
// Walk-up matters because tlc is frequently invoked from a project
// subdirectory; a check rooted only at cwd would miss the canonical
// config at the project root.
func canonicalConfigClaimsProject(projectID string) bool {
	cwd, err := os.Getwd()
	if err != nil {
		return false
	}

	candidates := []string{
		filepath.Join(config.LocalConfigDir(config.ModeStandalone), "config.yaml"),
		config.LocalConfigFile(config.ModeStandalone),
		filepath.Join(config.LocalConfigDir(config.ModeHop), "config.yaml"),
		config.LocalConfigFile(config.ModeHop),
	}

	for {
		for _, c := range candidates {
			if configHasProjectID(filepath.Join(cwd, c), projectID) {
				return true
			}
		}
		parent := filepath.Dir(cwd)
		if parent == cwd {
			return false
		}
		cwd = parent
	}
}

func CreateConfigWithInferredID(projectID string) error {
	mode := config.DetectMode()
	configDir := config.LocalConfigDir(mode)
	configPath := filepath.Join(configDir, "config.yaml")

	// Skip rewrite if any ancestor already has a config claiming this
	// project ID, regardless of the entry mode currently detected. This
	// prevents tlc from auto-recreating .hop/tlc/config.yaml in repos
	// whose canonical config is .tlc/config.yaml (and vice versa) when
	// both ancestors are present (e.g. .hop/ used by git-hop tooling
	// alongside a standalone .tlc/ dir; T-1303). The walk-up matters
	// when tlc is invoked from a project subdirectory.
	if canonicalConfigClaimsProject(projectID) {
		return nil
	}

	if err := os.MkdirAll(configDir, 0o750); err != nil {
		return fmt.Errorf("failed to create %s directory: %w", configDir, err)
	}

	cfg := map[string]interface{}{
		"version": 0.1,
		"project": map[string]interface{}{
			"id": projectID,
		},
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	log.Info("Detected project, created config", "path", configPath, "project_id", projectID)
	return nil
}

func promptFallbackMode(projectID string) (string, error) {
	var choice string

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(fmt.Sprintf("Detected project '%s' from git remote", projectID)).
				Description("How should TLC handle project detection?").
				Options(
					huh.NewOption("Create config file (recommended)", "auto"),
					huh.NewOption("Use detected mode without config", "detected"),
				).
				Value(&choice),
		),
	)

	err := form.Run()
	if err != nil {
		return "", fmt.Errorf("failed to run fallback mode prompt: %w", err)
	}
	return choice, nil
}
