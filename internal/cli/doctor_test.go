package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func setupDoctorTest(t *testing.T) (string, func()) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "tlc-doctor-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)

	viper.Reset()

	cleanup := func() {
		os.Chdir(oldWd)
		os.RemoveAll(tmpDir)
	}
	return tmpDir, cleanup
}

// execDoctorDirect calls runDoctor directly, bypassing cobra initialization
// hooks that trigger initConfig/ingestTODO/DB opens.
func execDoctorDirect(t *testing.T, fix bool) (string, error) {
	t.Helper()
	cmd := &cobra.Command{Use: "doctor"}
	buf := new(strings.Builder)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	err := runDoctor(cmd, fix)
	return buf.String(), err
}

func TestDoctorCmd_AllPass(t *testing.T) {
	tmpDir, cleanup := setupDoctorTest(t)
	defer cleanup()

	os.Mkdir(".git", 0755)
	os.MkdirAll(".tlc", 0755)
	config := "version: 0.1\nproject:\n  id: org/repo\n"
	os.WriteFile(".tlc/config.yaml", []byte(config), 0644)

	dbPath := filepath.Join(tmpDir, "db.sqlite")
	viper.Set("storage.db_path", dbPath)
	viper.Set("storage.backend", "sqlite")
	viper.Set("project.id", "org/repo")
	viper.Set("git.track", false)

	output, err := execDoctorDirect(t, false)
	if err != nil {
		t.Logf("output: %s", output)
	}

	if !strings.Contains(output, "checks") {
		t.Errorf("expected summary line, got: %s", output)
	}
	if !strings.Contains(output, "passed") {
		t.Errorf("expected 'passed' in output, got: %s", output)
	}
}

func TestDoctorCmd_MissingTlcDir(t *testing.T) {
	_, cleanup := setupDoctorTest(t)
	defer cleanup()

	os.Mkdir(".git", 0755)
	viper.Set("git.track", false)
	viper.Set("storage.backend", "sqlite")

	output, err := execDoctorDirect(t, false)
	if err == nil {
		t.Error("expected error for missing .tlc/")
	}
	if !strings.Contains(output, ".tlc/ directory exists") {
		t.Errorf("expected .tlc dir check in output, got: %s", output)
	}
}

func TestDoctorCmd_MissingTlcDir_Fix(t *testing.T) {
	_, cleanup := setupDoctorTest(t)
	defer cleanup()

	os.Mkdir(".git", 0755)
	viper.Set("git.track", false)
	viper.Set("storage.backend", "sqlite")

	output, _ := execDoctorDirect(t, true)
	if !strings.Contains(output, "fixed") {
		t.Errorf("expected 'fixed' in output, got: %s", output)
	}
	if _, err := os.Stat(".tlc"); os.IsNotExist(err) {
		t.Error(".tlc/ was not created by --fix")
	}
}

func TestDoctorCmd_MissingConfig(t *testing.T) {
	_, cleanup := setupDoctorTest(t)
	defer cleanup()

	os.Mkdir(".git", 0755)
	os.MkdirAll(".tlc", 0755)
	viper.Set("git.track", false)
	viper.Set("storage.backend", "sqlite")

	output, err := execDoctorDirect(t, false)
	if err == nil {
		t.Error("expected error for missing config.yaml")
	}
	if !strings.Contains(output, ".tlc/config.yaml exists") {
		t.Errorf("expected config check in output, got: %s", output)
	}
}

func TestDoctorCmd_MissingConfig_Fix(t *testing.T) {
	_, cleanup := setupDoctorTest(t)
	defer cleanup()

	os.Mkdir(".git", 0755)
	os.MkdirAll(".tlc", 0755)
	viper.Set("git.track", false)
	viper.Set("storage.backend", "sqlite")

	output, _ := execDoctorDirect(t, true)
	if !strings.Contains(output, ".tlc/config.yaml exists") {
		t.Errorf("expected config check in output, got: %s", output)
	}
}

func TestDoctorCmd_InvalidYAML(t *testing.T) {
	tmpDir, cleanup := setupDoctorTest(t)
	defer cleanup()

	os.Mkdir(".git", 0755)
	os.MkdirAll(".tlc", 0755)
	os.WriteFile(".tlc/config.yaml", []byte("{{{invalid yaml"), 0644)
	viper.Set("git.track", false)
	viper.Set("storage.backend", "sqlite")
	viper.Set("storage.db_path", filepath.Join(tmpDir, "db.sqlite"))

	output, err := execDoctorDirect(t, false)
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
	if !strings.Contains(output, "config YAML is valid") {
		t.Errorf("expected YAML check in output, got: %s", output)
	}
}

func TestDoctorCmd_NoGit(t *testing.T) {
	_, cleanup := setupDoctorTest(t)
	defer cleanup()

	viper.Set("git.track", false)
	viper.Set("storage.backend", "sqlite")

	output, err := execDoctorDirect(t, false)
	if err == nil {
		t.Error("expected error for no git repo")
	}
	if !strings.Contains(output, "inside git repository") {
		t.Errorf("expected git repo check in output, got: %s", output)
	}
}

func TestDoctorCmd_GitignoreMissing(t *testing.T) {
	tmpDir, cleanup := setupDoctorTest(t)
	defer cleanup()

	os.Mkdir(".git", 0755)
	os.MkdirAll(".tlc", 0755)
	os.WriteFile(".tlc/config.yaml", []byte("version: 0.1\nproject:\n  id: test\n"), 0644)
	viper.Set("git.track", true)
	viper.Set("project.id", "test")
	viper.Set("storage.backend", "sqlite")
	viper.Set("storage.db_path", filepath.Join(tmpDir, "db.sqlite"))

	output, err := execDoctorDirect(t, false)
	if err == nil {
		t.Error("expected error for missing .gitignore entry")
	}
	if !strings.Contains(output, ".gitignore") {
		t.Errorf("expected gitignore check in output, got: %s", output)
	}
}

func TestDoctorCmd_GitignoreFix(t *testing.T) {
	tmpDir, cleanup := setupDoctorTest(t)
	defer cleanup()

	os.Mkdir(".git", 0755)
	os.MkdirAll(".tlc", 0755)
	os.WriteFile(".tlc/config.yaml", []byte("version: 0.1\nproject:\n  id: test\n"), 0644)
	os.WriteFile(".gitignore", []byte("*.log\n"), 0644)
	viper.Set("git.track", true)
	viper.Set("project.id", "test")
	viper.Set("storage.backend", "sqlite")
	viper.Set("storage.db_path", filepath.Join(tmpDir, "db.sqlite"))

	output, _ := execDoctorDirect(t, true)
	if !strings.Contains(output, "fixed") {
		t.Logf("output: %s", output)
	}

	content, err := os.ReadFile(".gitignore")
	if err != nil {
		t.Fatalf("failed to read .gitignore: %v", err)
	}
	if !strings.Contains(string(content), ".tlc/") {
		t.Error(".gitignore should contain .tlc/ after fix")
	}
}

func TestDoctorCmd_OutputFormat(t *testing.T) {
	_, cleanup := setupDoctorTest(t)
	defer cleanup()

	os.Mkdir(".git", 0755)
	viper.Set("git.track", false)
	viper.Set("storage.backend", "sqlite")

	output, _ := execDoctorDirect(t, false)

	if !strings.Contains(output, "Git") {
		t.Errorf("expected Git category in output, got: %s", output)
	}
	if !strings.Contains(output, "Config") {
		t.Errorf("expected Config category in output, got: %s", output)
	}
	if !strings.Contains(output, "Storage") {
		t.Errorf("expected Storage category in output, got: %s", output)
	}
	if !strings.Contains(output, "checks") {
		t.Errorf("expected summary with 'checks' in output, got: %s", output)
	}
}
