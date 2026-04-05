package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestGenerateTaskSchema_KnownSubcommands(t *testing.T) {
	schemas := GenerateTaskSchema()
	if len(schemas) == 0 {
		t.Fatal("expected at least one subcommand schema")
	}

	required := []string{"create", "list", "show", "update", "delete", "claim", "complete"}
	index := make(map[string]CommandSchema, len(schemas))
	for _, s := range schemas {
		index[s.Name] = s
	}

	for _, name := range required {
		if _, ok := index[name]; !ok {
			t.Errorf("missing required subcommand %q in schema", name)
		}
	}
}

func TestGenerateTaskSchema_NonEmptyFields(t *testing.T) {
	schemas := GenerateTaskSchema()
	for _, s := range schemas {
		if s.Name == "" {
			t.Error("schema entry has empty Name")
		}
		if s.Description == "" {
			t.Errorf("schema %q has empty Description", s.Name)
		}
	}
}

func TestGenerateTaskSchemaJSON_ValidJSON(t *testing.T) {
	data, err := GenerateTaskSchemaJSON()
	if err != nil {
		t.Fatalf("GenerateTaskSchemaJSON returned error: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("GenerateTaskSchemaJSON returned empty bytes")
	}

	var parsed []CommandSchema
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("JSON output is not parseable: %v", err)
	}
	if len(parsed) == 0 {
		t.Error("parsed JSON has zero entries")
	}
}

func TestGenerateTaskSchema_FlagDetails(t *testing.T) {
	schemas := GenerateTaskSchema()
	index := make(map[string]CommandSchema, len(schemas))
	for _, s := range schemas {
		index[s.Name] = s
	}

	tests := []struct {
		cmd  string
		flag string
	}{
		{"list", "mine"},
		{"complete", "no-verify"},
	}

	for _, tc := range tests {
		t.Run(tc.cmd+"/"+tc.flag, func(t *testing.T) {
			cs, ok := index[tc.cmd]
			if !ok {
				t.Fatalf("subcommand %q not found in schema", tc.cmd)
			}
			found := false
			for _, f := range cs.Flags {
				if f.Name == tc.flag {
					found = true
					if f.Description == "" {
						t.Errorf("flag %q on %q has empty description", tc.flag, tc.cmd)
					}
					if f.Type == "" {
						t.Errorf("flag %q on %q has empty type", tc.flag, tc.cmd)
					}
					break
				}
			}
			if !found {
				t.Errorf("flag %q not found on subcommand %q", tc.flag, tc.cmd)
			}
		})
	}
}

func TestGenerateTaskSchema_PersistentFlags(t *testing.T) {
	schemas := GenerateTaskSchema()
	if len(schemas) == 0 {
		t.Fatal("no schemas returned")
	}

	// Every subcommand should inherit --no-prompt from TaskCmd.
	for _, s := range schemas {
		found := false
		for _, f := range s.Flags {
			if f.Name == "no-prompt" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("subcommand %q missing inherited persistent flag --no-prompt", s.Name)
		}
	}
}

// TestGenerateSchema_RootCoversAllTopLevel verifies GenerateSchema covers
// subcommands from multiple top-level commands (task, track, flow, etc.).
func TestGenerateSchema_RootCoversAllTopLevel(t *testing.T) {
	schemas := GenerateSchema(RootCmd)
	if len(schemas) == 0 {
		t.Fatal("expected entries from GenerateSchema(RootCmd)")
	}

	// Name format must be "top sub" (two words).
	for _, s := range schemas {
		parts := strings.Fields(s.Name)
		if len(parts) != 2 {
			t.Errorf("schema Name %q should be 'top sub' (2 words), got %d", s.Name, len(parts))
		}
	}
}

// TestGenerateSchema_IncludesTaskAndTrack verifies multi-domain coverage.
func TestGenerateSchema_IncludesTaskAndTrack(t *testing.T) {
	schemas := GenerateSchema(RootCmd)
	index := make(map[string]bool, len(schemas))
	for _, s := range schemas {
		index[s.Name] = true
	}

	required := []string{"task list", "task create", "track list", "track create"}
	for _, name := range required {
		if !index[name] {
			t.Errorf("missing expected command %q in GenerateSchema(RootCmd)", name)
		}
	}
}

// TestGenerateSchema_NonEmptyFields verifies all entries have name + description.
func TestGenerateSchema_NonEmptyFields(t *testing.T) {
	schemas := GenerateSchema(RootCmd)
	for _, s := range schemas {
		if s.Name == "" {
			t.Error("GenerateSchema entry has empty Name")
		}
		if s.Description == "" {
			t.Errorf("GenerateSchema %q has empty Description", s.Name)
		}
	}
}

// TestGenerateSchemaJSON_ValidJSON verifies GenerateSchemaJSON returns valid JSON.
func TestGenerateSchemaJSON_ValidJSON(t *testing.T) {
	data, err := GenerateSchemaJSON(RootCmd)
	if err != nil {
		t.Fatalf("GenerateSchemaJSON returned error: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("GenerateSchemaJSON returned empty bytes")
	}

	var parsed []CommandSchema
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("JSON output is not parseable: %v", err)
	}
	if len(parsed) == 0 {
		t.Error("parsed JSON has zero entries")
	}
}
