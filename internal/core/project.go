package core

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/log"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

type ProjectDetection struct {
	ProjectID  string
	ConfigPath string
	InProject  bool
}

func DetectProject() *ProjectDetection {
	checkConfigPath := viper.GetString("config")
	if checkConfigPath != "" {
		viper.SetConfigFile(checkConfigPath)
		viper.SetConfigType("yaml")
		viper.ReadInConfig()
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
		projectID = detectProjectID()
		viper.Set("project.id", projectID)
		viper.WriteConfig()
	}

	return &ProjectDetection{
		ProjectID:  projectID,
		ConfigPath: tlcConfigPath,
		InProject:  true,
	}
}

func detectProjectID() string {
	if detected := DetectFromGitRemote(); detected != "" {
		return detected
	}
	if detected := detectFromGitConfig(); detected != "" {
		return detected
	}
	if detected := detectFromDirectory(); detected != "" {
		return detected
	}
	return "unknown"
}

func DetectFromGitRemote() string {
	cmd := exec.Command("git", "remote", "get-url", "origin")
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

func detectFromGitConfig() string {
	cmd := exec.Command("git", "config", "--get", "remote.origin.url")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	url := strings.TrimSpace(string(output))

	if url == "" {
		cmd := exec.Command("git", "config", "--get", "user.name")
		name, _ := cmd.Output()
		cmd = exec.Command("git", "config", "--get", "user.email")
		email, _ := cmd.Output()
		if name != nil && email != nil {
			return strings.TrimSpace(string(name) + "-" + string(email))
		}
	}
	return ""
}

func detectFromDirectory() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "unknown"
	}

	cwd = filepath.Dir(cwd)

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

	return "unknown"
}

func handleFallbackMode() *ProjectDetection {
	mode := viper.GetString("project.fallback_mode")
	if mode == "" {
		mode = "auto"
	}

	inferredID := DetectFromGitRemote()
	if inferredID == "" {
		return &ProjectDetection{InProject: false}
	}

	switch mode {
	case "auto":
		if err := createConfigWithInferredID(inferredID); err == nil {
			return &ProjectDetection{
				ProjectID:  inferredID,
				ConfigPath: ".tlc/config.yaml",
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
			if createErr := createConfigWithInferredID(inferredID); createErr == nil {
				return &ProjectDetection{
					ProjectID:  inferredID,
					ConfigPath: ".tlc/config.yaml",
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

func createConfigWithInferredID(projectID string) error {
	if err := os.MkdirAll(".tlc", 0755); err != nil {
		return err
	}

	config := map[string]interface{}{
		"version": 0.1,
		"project": map[string]interface{}{
			"id": projectID,
		},
	}

	data, err := yaml.Marshal(config)
	if err != nil {
		return err
	}

	if err := os.WriteFile(".tlc/config.yaml", data, 0644); err != nil {
		return err
	}

	log.Info("Detected project from git remote, created .tlc/config.yaml", "project_id", projectID)
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
	return choice, err
}
