package config

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestFilesystemConfigBoolTrue(t *testing.T) {
	input := `filesystem: true`
	var sc StorageConfig
	if err := yaml.Unmarshal([]byte(input), &sc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !sc.Filesystem.Enabled {
		t.Error("expected Enabled=true")
	}
	if len(sc.Filesystem.GroupBy) != 1 || sc.Filesystem.GroupBy[0] != "status" {
		t.Errorf("GroupBy = %v, want [status]", sc.Filesystem.GroupBy)
	}
	if len(sc.Filesystem.SortBy) != 1 || sc.Filesystem.SortBy[0] != "id" {
		t.Errorf("SortBy = %v, want [id]", sc.Filesystem.SortBy)
	}
}

func TestFilesystemConfigBoolFalse(t *testing.T) {
	input := `filesystem: false`
	var sc StorageConfig
	if err := yaml.Unmarshal([]byte(input), &sc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if sc.Filesystem.Enabled {
		t.Error("expected Enabled=false")
	}
}

func TestFilesystemConfigObject(t *testing.T) {
	input := `
filesystem:
  group_by: [status, tag]
  sort_by: [priority, id]
`
	var sc StorageConfig
	if err := yaml.Unmarshal([]byte(input), &sc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !sc.Filesystem.Enabled {
		t.Error("expected Enabled=true for object form")
	}
	if len(sc.Filesystem.GroupBy) != 2 {
		t.Errorf("GroupBy = %v, want [status tag]", sc.Filesystem.GroupBy)
	}
	if len(sc.Filesystem.SortBy) != 2 {
		t.Errorf("SortBy = %v, want [priority id]", sc.Filesystem.SortBy)
	}
}

func TestFilesystemConfigOmitted(t *testing.T) {
	input := `backend: sqlite`
	var sc StorageConfig
	if err := yaml.Unmarshal([]byte(input), &sc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if sc.Filesystem.Enabled {
		t.Error("expected Enabled=false when omitted")
	}
}

func TestFilesystemConfigObjectDefaults(t *testing.T) {
	input := `
filesystem:
  group_by: [tag]
`
	var sc StorageConfig
	if err := yaml.Unmarshal([]byte(input), &sc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !sc.Filesystem.Enabled {
		t.Error("expected Enabled=true")
	}
	// sort_by should default to [id]
	if len(sc.Filesystem.SortBy) != 1 || sc.Filesystem.SortBy[0] != "id" {
		t.Errorf("SortBy = %v, want [id]", sc.Filesystem.SortBy)
	}
}

func TestFilesystemConfigValidateInvalidGroupBy(t *testing.T) {
	sc := StorageConfig{
		Backend: "sqlite",
		Filesystem: FilesystemConfig{
			Enabled: true,
			GroupBy: []string{"status", "invalid_field"},
			SortBy:  []string{"id"},
		},
	}
	err := sc.Validate()
	if err == nil {
		t.Fatal("expected validation error for invalid group_by")
	}
}

func TestFilesystemConfigValidateInvalidSortBy(t *testing.T) {
	sc := StorageConfig{
		Backend: "sqlite",
		Filesystem: FilesystemConfig{
			Enabled: true,
			GroupBy: []string{"status"},
			SortBy:  []string{"invalid_sort"},
		},
	}
	err := sc.Validate()
	if err == nil {
		t.Fatal("expected validation error for invalid sort_by")
	}
}

func TestFilesystemConfigValidateDisabledSkips(t *testing.T) {
	sc := StorageConfig{
		Backend: "sqlite",
		Filesystem: FilesystemConfig{
			Enabled: false,
			GroupBy: []string{"bad"},
			SortBy:  []string{"bad"},
		},
	}
	if err := sc.Validate(); err != nil {
		t.Errorf("disabled filesystem should skip validation, got: %v", err)
	}
}
