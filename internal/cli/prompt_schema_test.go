package cli

import (
	"encoding/json"
	"testing"

	"hop.top/kit/go/ai/toolspec"
)

func TestCobraToToolSpec_HasCommands(t *testing.T) {
	spec := CobraToToolSpec(RootCmd)
	if spec == nil {
		t.Fatal("CobraToToolSpec returned nil")
	}
	if spec.Name == "" {
		t.Error("ToolSpec.Name is empty")
	}
	if len(spec.Commands) == 0 {
		t.Fatal("expected at least one top-level command")
	}
}

func TestCobraToToolSpec_TaskChildren(t *testing.T) {
	spec := CobraToToolSpec(RootCmd)
	var taskCmd *toolspec.Command
	for i := range spec.Commands {
		if spec.Commands[i].Name == "task" {
			taskCmd = &spec.Commands[i]
			break
		}
	}
	if taskCmd == nil {
		t.Fatal("expected 'task' command in ToolSpec")
	}
	if len(taskCmd.Children) == 0 {
		t.Fatal("expected children under 'task' command")
	}

	required := map[string]bool{
		"create": false, "list": false, "show": false,
		"update": false, "claim": false, "complete": false,
	}
	for _, child := range taskCmd.Children {
		if _, ok := required[child.Name]; ok {
			required[child.Name] = true
		}
	}
	for name, found := range required {
		if !found {
			t.Errorf("missing required child command %q under 'task'", name)
		}
	}
}

func TestCobraToToolSpec_FlagsPopulated(t *testing.T) {
	spec := CobraToToolSpec(RootCmd)
	var taskCmd *toolspec.Command
	for i := range spec.Commands {
		if spec.Commands[i].Name == "task" {
			taskCmd = &spec.Commands[i]
			break
		}
	}
	if taskCmd == nil {
		t.Fatal("expected 'task' command")
	}
	for _, child := range taskCmd.Children {
		if child.Name == "list" {
			found := false
			for _, f := range child.Flags {
				if f.Name == "mine" {
					found = true
					break
				}
			}
			if !found {
				t.Error("expected --mine flag on 'task list'")
			}
			return
		}
	}
	t.Error("'list' subcommand not found under 'task'")
}

func TestCobraToToolSpec_HiddenExcluded(t *testing.T) {
	spec := CobraToToolSpec(RootCmd)
	for _, cmd := range spec.Commands {
		if cmd.Name == "completion" {
			t.Error("hidden command 'completion' should be excluded")
		}
	}
}

// --- Flat schema backward compat tests ---

func TestGenerateTaskSchema_KnownSubcommands(t *testing.T) {
	schemas := GenerateTaskSchema()
	if len(schemas) == 0 {
		t.Fatal("expected at least one subcommand schema")
	}

	required := []string{
		"create", "list", "show", "update", "delete", "claim", "complete",
	}
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
						t.Errorf(
							"flag %q on %q has empty description",
							tc.flag, tc.cmd,
						)
					}
					if f.Type == "" {
						t.Errorf(
							"flag %q on %q has empty type",
							tc.flag, tc.cmd,
						)
					}
					break
				}
			}
			if !found {
				t.Errorf(
					"flag %q not found on subcommand %q",
					tc.flag, tc.cmd,
				)
			}
		})
	}
}

// FlagSchema.Default is populated from pflag defaults.
//
// Vehicle is `task list --sort-by`, whose "created_at" is a genuine
// pflag default. This used to assert `task create --status` == "TODO",
// but that default was the bug: supplied on every run, it outranked
// task.default_status and made create unusable under a renamed status
// vocabulary. --status now registers empty and resolves from config, so
// asserting a literal here would re-pin the defect rather than the
// requirement (that DefValue reaches the schema at all).
func TestFlagSchema_DefaultPopulatedFromPflag(t *testing.T) {
	schemas := GenerateTaskSchema()
	index := make(map[string]CommandSchema, len(schemas))
	for _, s := range schemas {
		index[s.Name] = s
	}

	cs, ok := index["list"]
	if !ok {
		t.Fatal("subcommand 'list' not found in task schema")
	}

	tests := []struct {
		flag    string
		wantDef string
	}{
		{"sort-by", "created_at"},
		{"sort-direction", "desc"},
	}
	for _, tc := range tests {
		t.Run(tc.flag, func(t *testing.T) {
			var found *FlagSchema
			for i := range cs.Flags {
				if cs.Flags[i].Name == tc.flag {
					found = &cs.Flags[i]
					break
				}
			}
			if found == nil {
				t.Fatalf("flag %q not found on 'list'", tc.flag)
			}
			if found.Default != tc.wantDef {
				t.Errorf(
					"FlagSchema.Default for --%s = %q, want %q",
					tc.flag, found.Default, tc.wantDef,
				)
			}
		})
	}
}

// T-0597 (cont): Bool flag --mine has DefValue "false" — verify it is captured.
func TestFlagSchema_BoolDefaultCaptured(t *testing.T) {
	schemas := GenerateTaskSchema()
	index := make(map[string]CommandSchema, len(schemas))
	for _, s := range schemas {
		index[s.Name] = s
	}

	cs, ok := index["list"]
	if !ok {
		t.Fatal("subcommand 'list' not found in task schema")
	}

	// --mine is a bool flag with default "false"; since DefValue is "false"
	// it should be populated.
	for _, f := range cs.Flags {
		if f.Name == "mine" {
			if f.Default != "false" {
				t.Errorf(
					"FlagSchema.Default for --mine = %q, want %q",
					f.Default, "false",
				)
			}
			return
		}
	}
	t.Error("--mine flag not found on 'list' subcommand")
}

func TestGenerateTaskSchema_PersistentFlags(t *testing.T) {
	schemas := GenerateTaskSchema()
	if len(schemas) == 0 {
		t.Fatal("no schemas returned")
	}

	for _, s := range schemas {
		found := false
		for _, f := range s.Flags {
			if f.Name == "no-prompt" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf(
				"subcommand %q missing inherited persistent flag --no-prompt",
				s.Name,
			)
		}
	}
}

func TestGenerateSchema_RootCoversAllTopLevel(t *testing.T) {
	schemas := GenerateSchema(RootCmd)
	if len(schemas) == 0 {
		t.Fatal("expected entries from GenerateSchema(RootCmd)")
	}
	for _, s := range schemas {
		if s.Name == "" {
			t.Error("schema entry has empty Name")
		}
	}
}

func TestGenerateSchema_IncludesTaskAndTrack(t *testing.T) {
	schemas := GenerateSchema(RootCmd)
	index := make(map[string]bool, len(schemas))
	for _, s := range schemas {
		index[s.Name] = true
	}

	required := []string{
		"task list", "task create", "track list", "track create",
	}
	for _, name := range required {
		if !index[name] {
			t.Errorf(
				"missing expected command %q in GenerateSchema(RootCmd)",
				name,
			)
		}
	}
}

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

// TestBuildToolSpec_SpecIntegrity verifies the canonical tool spec.
func TestBuildToolSpec_SpecIntegrity(t *testing.T) {
	spec := buildToolSpec()
	if spec.Name != toolName {
		t.Errorf("expected name %q, got %q", toolName, spec.Name)
	}
	if len(spec.Commands) == 0 {
		t.Error("expected at least one command in tool spec")
	}
	if len(spec.Flags) == 0 {
		t.Error("expected at least one flag in tool spec")
	}

	// Verify action flag exists and its enum values (via specProperties)
	// match the spec's command names.
	actionFlag := findFlag(spec.Flags, "action")
	if actionFlag == nil {
		t.Fatal("missing 'action' flag in tool spec")
	}

	props := specProperties(spec)
	actionProp, ok := props["action"].(map[string]interface{})
	if !ok {
		t.Fatal("action property missing from specProperties output")
	}
	enumVals, ok := actionProp["enum"].([]string)
	if !ok {
		t.Fatal("action property missing enum values")
	}
	cmdNames := make(map[string]bool, len(spec.Commands))
	for _, c := range spec.Commands {
		cmdNames[c.Name] = true
	}
	for _, v := range enumVals {
		if !cmdNames[v] {
			t.Errorf("enum value %q not found in command names", v)
		}
	}
	if len(enumVals) != len(spec.Commands) {
		t.Errorf(
			"enum count %d != command count %d",
			len(enumVals), len(spec.Commands),
		)
	}
}

// TestRenderToolSpec_AllFormats verifies all formats produce valid JSON.
func TestRenderToolSpec_AllFormats(t *testing.T) {
	spec := buildToolSpec()
	for _, fmt := range []string{"json", "mcp", "openai", "anthropic"} {
		t.Run(fmt, func(t *testing.T) {
			out := renderToolSpec(spec, fmt)
			if out == nil {
				t.Fatalf("renderToolSpec returned nil for format %q", fmt)
			}
			data, err := json.Marshal(out)
			if err != nil {
				t.Fatalf("json.Marshal failed: %v", err)
			}
			if len(data) == 0 {
				t.Error("empty JSON output")
			}
		})
	}
}

func TestRenderToolSpec_UnknownFormat(t *testing.T) {
	spec := buildToolSpec()
	if out := renderToolSpec(spec, "yaml"); out != nil {
		t.Error("expected nil for unknown format")
	}
}

func findFlag(flags []toolspec.Flag, name string) *toolspec.Flag {
	for i := range flags {
		if flags[i].Name == name {
			return &flags[i]
		}
	}
	return nil
}
