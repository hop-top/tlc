package config

import (
	"os"
	"path/filepath"
	"testing"
)

// layoutFixture creates a temp root holding dirs and empty files
// (relative paths) and returns it.
func layoutFixture(t *testing.T, dirs, files []string) string {
	t.Helper()
	tmp := t.TempDir()
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(tmp, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range files {
		p := filepath.Join(tmp, f)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(""), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return tmp
}

func TestHasHopConfig(t *testing.T) {
	tests := []struct {
		name  string
		dirs  []string
		files []string
		want  bool
	}{
		{name: "nothing", want: false},
		{name: "bare .hop", dirs: []string{".hop"}, want: false},
		{name: "repair artifacts only", dirs: []string{filepath.Join(".hop", "backups")}, files: []string{filepath.Join(".hop", "repair.lock")}, want: false},
		{name: ".hop/tlc dir", dirs: []string{filepath.Join(".hop", "tlc")}, want: true},
		{name: ".hop/tlc.yaml", dirs: []string{".hop"}, files: []string{filepath.Join(".hop", "tlc.yaml")}, want: true},
		{name: ".hop/tlc as a file is not a config dir", dirs: []string{".hop"}, files: []string{filepath.Join(".hop", "tlc")}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmp := layoutFixture(t, tt.dirs, tt.files)
			if got := HasHopConfig(tmp); got != tt.want {
				t.Errorf("HasHopConfig() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExistingLocalConfig(t *testing.T) {
	tests := []struct {
		name     string
		files    []string
		wantRel  string
		wantMode EntryMode
		wantOK   bool
	}{
		{name: "none", wantOK: false},
		{name: "bare .hop dir only", files: []string{filepath.Join(".hop", "repair.lock")}, wantOK: false},
		{name: "empty .tlc dir has no config", files: []string{filepath.Join(".tlc", "tasks", "x.md")}, wantOK: false},
		{name: "standalone dir", files: []string{filepath.Join(".tlc", "config.yaml")}, wantRel: filepath.Join(".tlc", "config.yaml"), wantMode: ModeStandalone, wantOK: true},
		{name: "standalone flat", files: []string{".tlc.yaml"}, wantRel: ".tlc.yaml", wantMode: ModeStandalone, wantOK: true},
		{name: "hop dir", files: []string{filepath.Join(".hop", "tlc", "config.yaml")}, wantRel: filepath.Join(".hop", "tlc", "config.yaml"), wantMode: ModeHop, wantOK: true},
		{name: "hop flat", files: []string{filepath.Join(".hop", "tlc.yaml")}, wantRel: filepath.Join(".hop", "tlc.yaml"), wantMode: ModeHop, wantOK: true},
		{
			name:     "both present reports hop first",
			files:    []string{filepath.Join(".tlc", "config.yaml"), filepath.Join(".hop", "tlc", "config.yaml")},
			wantRel:  filepath.Join(".hop", "tlc", "config.yaml"),
			wantMode: ModeHop,
			wantOK:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmp := layoutFixture(t, nil, tt.files)
			got, mode, ok := ExistingLocalConfig(tmp)
			if ok != tt.wantOK {
				t.Fatalf("ExistingLocalConfig() ok = %v, want %v (path %q)", ok, tt.wantOK, got)
			}
			if !ok {
				return
			}
			if want := filepath.Join(tmp, tt.wantRel); got != want {
				t.Errorf("ExistingLocalConfig() path = %q, want %q", got, want)
			}
			if mode != tt.wantMode {
				t.Errorf("ExistingLocalConfig() mode = %q, want %q", mode, tt.wantMode)
			}
		})
	}
}

func TestConfigDirForFile(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"/r/.tlc/config.yaml", "/r/.tlc"},
		{"/r/.tlc.yaml", "/r/.tlc"},
		{"/r/.hop/tlc/config.yaml", "/r/.hop/tlc"},
		{"/r/.hop/tlc.yaml", "/r/.hop/tlc"},
		{"/r/.tlc/other.yaml", "/r/.tlc"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := ConfigDirForFile(filepath.FromSlash(tt.in)); got != filepath.FromSlash(tt.want) {
				t.Errorf("ConfigDirForFile(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestProjectRootFromConfigDir(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"/r/.tlc", "/r"},
		{"/r/.hop/tlc", "/r"},
		{"/r/hops/main/.tlc", "/r/hops/main"},
		{"/r/tlc", "/r"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := ProjectRootFromConfigDir(filepath.FromSlash(tt.in)); got != filepath.FromSlash(tt.want) {
				t.Errorf("ProjectRootFromConfigDir(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
