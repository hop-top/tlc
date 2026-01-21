package core

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/viper"
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
		return &ProjectDetection{
			InProject: false,
		}
	}

	dotTlcDir := filepath.Dir(configPath)
	tlcConfigPath := filepath.Join(dotTlcDir, "config.yaml")

	if _, err := os.Stat(tlcConfigPath); os.IsNotExist(err) {
		return &ProjectDetection{
			InProject: false,
		}
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
	if detected := detectFromGitRemote(); detected != "" {
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

func detectFromGitRemote() string {
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
