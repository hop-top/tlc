package cli

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func TestConfigDomain(t *testing.T) {
	tests := []struct {
		parent   string
		expected string
	}{
		{"task", "task"},
		{"track", "tracks"},
		{"recipe", "recipe"},
	}

	for _, tt := range tests {
		t.Run(tt.parent, func(t *testing.T) {
			got := configDomain(tt.parent)
			if got != tt.expected {
				t.Errorf("configDomain(%q) = %q, want %q", tt.parent, got, tt.expected)
			}
		})
	}
}

func TestResolveFlagDefaultKey_Precedence(t *testing.T) {
	tests := []struct {
		name   string
		setup  func()
		parent string
		leaf   string
		flag   string
		expKey string
		expOk  bool
	}{
		{
			name: "defaults.verb.flag precedence",
			setup: func() {
				viper.Reset()
				viper.Set("defaults.list.columns", "id,title")
			},
			parent: "task",
			leaf:   "list",
			flag:   "columns",
			expKey: "defaults.list.columns",
			expOk:  true,
		},
		{
			name: "domain.verb.flag overrides defaults.verb.flag",
			setup: func() {
				viper.Reset()
				viper.Set("defaults.list.columns", "id,title")
				viper.Set("task.list.columns", "id,title,status")
			},
			parent: "task",
			leaf:   "list",
			flag:   "columns",
			expKey: "task.list.columns",
			expOk:  true,
		},
		{
			name: "defaults.flag fallback",
			setup: func() {
				viper.Reset()
				viper.Set("defaults.columns", "id,title")
			},
			parent: "task",
			leaf:   "list",
			flag:   "columns",
			expKey: "defaults.columns",
			expOk:  true,
		},
		{
			name: "no match returns false",
			setup: func() {
				viper.Reset()
			},
			parent: "task",
			leaf:   "list",
			flag:   "columns",
			expKey: "",
			expOk:  false,
		},
		{
			name: "root command skips domain level",
			setup: func() {
				viper.Reset()
				viper.Set("defaults.list.columns", "id,title")
			},
			parent: "",
			leaf:   "list",
			flag:   "columns",
			expKey: "defaults.list.columns",
			expOk:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()

			// Build cobra tree
			root := &cobra.Command{Use: "root"}
			parent := &cobra.Command{Use: tt.parent}
			leaf := &cobra.Command{Use: tt.leaf}

			if tt.parent != "" {
				root.AddCommand(parent)
				parent.AddCommand(leaf)
			} else {
				root.AddCommand(leaf)
			}

			key, ok := resolveFlagDefaultKey(leaf, tt.flag)
			if ok != tt.expOk {
				t.Errorf("resolveFlagDefaultKey() ok = %v, want %v", ok, tt.expOk)
			}
			if key != tt.expKey {
				t.Errorf("resolveFlagDefaultKey() key = %q, want %q", key, tt.expKey)
			}
		})
	}
}

func buildTaskListCmd() *cobra.Command {
	root := &cobra.Command{Use: "root"}
	task := &cobra.Command{Use: "task"}
	list := &cobra.Command{Use: "list"}
	root.AddCommand(task)
	task.AddCommand(list)
	list.Flags().StringSlice("columns", nil, "columns")
	list.Flags().Bool("archived", false, "archived")
	list.Flags().Int("limit", 0, "limit")
	return list
}

func TestApplyConfigDefaults_SeedsUnsetFlags(t *testing.T) {
	viper.Reset()
	viper.Set("task.list.columns", []string{"id", "title"})
	viper.Set("task.list.limit", 50)

	list := buildTaskListCmd()

	fromConfig, err := applyConfigDefaults(list)
	if err != nil {
		t.Fatalf("applyConfigDefaults() error = %v", err)
	}

	cols, err := list.Flags().GetStringSlice("columns")
	if err != nil {
		t.Fatalf("GetStringSlice: %v", err)
	}
	if len(cols) != 2 || cols[0] != "id" || cols[1] != "title" {
		t.Errorf("columns = %v, want [id title]", cols)
	}

	lim, err := list.Flags().GetInt("limit")
	if err != nil {
		t.Fatalf("GetInt: %v", err)
	}
	if lim != 50 {
		t.Errorf("limit = %d, want 50", lim)
	}

	arch, err := list.Flags().GetBool("archived")
	if err != nil {
		t.Fatalf("GetBool: %v", err)
	}
	if arch {
		t.Errorf("archived = true, want false (not set in config)")
	}

	if !fromConfig["columns"] {
		t.Errorf("fromConfig[columns] = false, want true")
	}
	if !fromConfig["limit"] {
		t.Errorf("fromConfig[limit] = false, want true")
	}
	if fromConfig["archived"] {
		t.Errorf("fromConfig[archived] = true, want false (not set in config)")
	}
}

func TestApplyConfigDefaults_PreservesChangedFalse(t *testing.T) {
	viper.Reset()
	viper.Set("task.list.columns", []string{"id", "title"})

	list := buildTaskListCmd()

	if _, err := applyConfigDefaults(list); err != nil {
		t.Fatalf("applyConfigDefaults() error = %v", err)
	}

	if list.Flags().Changed("columns") {
		t.Errorf("Changed(columns) = true, want false (config must not flip Changed bit)")
	}
}

func TestApplyConfigDefaults_CLIWins(t *testing.T) {
	viper.Reset()
	viper.Set("task.list.columns", []string{"id", "title"})

	list := buildTaskListCmd()

	// Simulate explicit user flag (sets Changed=true)
	if err := list.Flags().Set("columns", "explicit"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	fromConfig, err := applyConfigDefaults(list)
	if err != nil {
		t.Fatalf("applyConfigDefaults() error = %v", err)
	}

	cols, err := list.Flags().GetStringSlice("columns")
	if err != nil {
		t.Fatalf("GetStringSlice: %v", err)
	}
	if len(cols) != 1 || cols[0] != "explicit" {
		t.Errorf("columns = %v, want [explicit]", cols)
	}

	if fromConfig["columns"] {
		t.Errorf("fromConfig[columns] = true, want false (CLI wins, config must not record it)")
	}
}

func TestResolveFlagDefaultKey_Miss(t *testing.T) {
	viper.Reset()

	root := &cobra.Command{Use: "root"}
	task := &cobra.Command{Use: "task"}
	list := &cobra.Command{Use: "list"}

	root.AddCommand(task)
	task.AddCommand(list)

	key, ok := resolveFlagDefaultKey(list, "columns")
	if ok {
		t.Errorf("resolveFlagDefaultKey() ok = true, want false (no config set)")
	}
	if key != "" {
		t.Errorf("resolveFlagDefaultKey() key = %q, want empty string", key)
	}
}
