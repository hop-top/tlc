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
		{"flow", "flow"},
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
		name     string
		setup    func()
		parent   string
		leaf     string
		flag     string
		expKey   string
		expOk    bool
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
