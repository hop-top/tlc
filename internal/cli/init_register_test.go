package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/storage"
)

// TestInitCmd_RegistersNewProject verifies that init registers a new
// project in the global projects table when no prior registration exists.
func TestInitCmd_RegistersNewProject(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tlc-init-register-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	viper.Reset()
	core.ResetDetectionCache()
	dbSyncOnce = sync.Once{}

	dbFile := filepath.Join(tmpDir, "test-global.sqlite")
	viper.Set("storage.backend", "sqlite")
	viper.Set("storage.db_path", dbFile)

	cmd := newTestCmd()
	cmd.AddCommand(newTestInitCmd())

	buf := new(strings.Builder)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"init"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() failed: %v", err)
	}

	// Read back the project ID from generated config
	data, err := os.ReadFile(filepath.Join(".tlc", "config.yaml"))
	if err != nil {
		t.Fatalf("failed to read config: %v", err)
	}
	var cfg map[string]interface{}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("failed to unmarshal config: %v", err)
	}
	projCfgRaw, ok := cfg["project"]
	if !ok {
		t.Fatalf("config missing 'project' key")
	}
	projCfg, ok := projCfgRaw.(map[string]interface{})
	if !ok {
		t.Fatalf("config 'project' is not a map, got %T", projCfgRaw)
	}
	idRaw, ok := projCfg["id"]
	if !ok {
		t.Fatalf("project config missing 'id' key")
	}
	projectID, ok := idRaw.(string)
	if !ok {
		t.Fatalf("project 'id' is not a string, got %T", idRaw)
	}

	// Verify the project was registered in the global DB
	s, err := storage.NewSQLiteStorage(dbFile)
	if err != nil {
		t.Fatalf("failed to open storage: %v", err)
	}
	defer s.Close()

	proj, err := s.LookupProject(context.Background(), projectID)
	if err != nil {
		t.Fatalf("LookupProject() failed: %v", err)
	}
	if proj == nil {
		t.Fatal("expected project to be registered, got nil")
	}
	if proj.ProjectID != projectID {
		t.Errorf("project_id = %q, want %q", proj.ProjectID, projectID)
	}
	if proj.Label == "" {
		t.Error("expected non-empty label")
	}
}

// TestInitCmd_ReconnectsExistingProject verifies that running init
// a second time reconnects to an already-registered project.
func TestInitCmd_ReconnectsExistingProject(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tlc-init-reconnect-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	dbFile := filepath.Join(tmpDir, "test-global.sqlite")

	// Pre-register a project with an old db_path
	s, err := storage.NewSQLiteStorage(dbFile)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	ctx := context.Background()
	projectID := "unknown" // DetectProjectID returns "unknown" in temp dirs
	oldPath := "/old/path/db.sqlite"
	if err := s.RegisterProject(ctx, projectID, oldPath, "", "unknown"); err != nil {
		t.Fatalf("RegisterProject() failed: %v", err)
	}
	s.Close()

	// Run init
	viper.Reset()
	core.ResetDetectionCache()
	dbSyncOnce = sync.Once{}

	viper.Set("storage.backend", "sqlite")
	viper.Set("storage.db_path", dbFile)

	cmd := newTestCmd()
	cmd.AddCommand(newTestInitCmd())

	buf := new(strings.Builder)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"init"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() failed: %v", err)
	}

	// Verify the path was updated (not the old path)
	s2, err := storage.NewSQLiteStorage(dbFile)
	if err != nil {
		t.Fatalf("failed to open storage: %v", err)
	}
	defer s2.Close()

	proj, err := s2.LookupProject(ctx, projectID)
	if err != nil {
		t.Fatalf("LookupProject() failed: %v", err)
	}
	if proj.DBPath == oldPath {
		t.Errorf("db_path was not updated, still %q", oldPath)
	}
	// The init command uses the configured db filename (from viper storage.db_path).
	dbName := filepath.Base(viper.GetString("storage.db_path"))
	if !strings.Contains(proj.DBPath, dbName) {
		t.Errorf("expected db_path to contain %q, got %q", dbName, proj.DBPath)
	}
}
