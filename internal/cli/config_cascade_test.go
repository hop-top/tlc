package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hop.top/tlc/internal/config"
)

const (
	cascadeCfgOne = "version: 0.1\nproject:\n  id: org/one\nstorage:\n  db_path: /srv/one/db.sqlite\n"
	cascadeCfgTwo = "version: 0.1\nproject:\n  id: org/two\nstorage:\n  db_path: /srv/two/db.sqlite\n"
)

func writeCascadeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestResolveProjectConfigs pins the cascade the process actually
// merges. The standalone config must load when it is the only config on
// the walk even though the mode says hop (forced by TLC_MODE, or
// inherited from a .hop/ that carries no tlc config); a mirrored pair
// that agrees loads quietly; a pair that disagrees is refused with both
// files named, on any directory of the walk, not only cwd.
func TestResolveProjectConfigs(t *testing.T) {
	tests := []struct {
		name     string
		files    map[string]string
		dirs     []string
		start    string
		mode     config.EntryMode
		wantRel  []string
		wantErr  []string
		wantNone bool
	}{
		{
			name:    "hop mode with only .tlc falls back to standalone",
			files:   map[string]string{filepath.Join(".tlc", "config.yaml"): cascadeCfgOne},
			dirs:    []string{filepath.Join(".hop", "backups")},
			start:   ".",
			mode:    config.ModeHop,
			wantRel: []string{filepath.Join(".tlc", "config.yaml")},
		},
		{
			name:    "hop mode with only .tlc.yaml falls back to standalone flat",
			files:   map[string]string{".tlc.yaml": cascadeCfgOne},
			start:   ".",
			mode:    config.ModeHop,
			wantRel: []string{".tlc.yaml"},
		},
		{
			name:    "hop config wins when present",
			files:   map[string]string{filepath.Join(".hop", "tlc", "config.yaml"): cascadeCfgOne},
			start:   ".",
			mode:    config.ModeHop,
			wantRel: []string{filepath.Join(".hop", "tlc", "config.yaml")},
		},
		{
			name: "agreeing pair loads the hop copy",
			files: map[string]string{
				filepath.Join(".tlc", "config.yaml"):        cascadeCfgOne,
				filepath.Join(".hop", "tlc", "config.yaml"): cascadeCfgOne,
			},
			start:   ".",
			mode:    config.ModeHop,
			wantRel: []string{filepath.Join(".hop", "tlc", "config.yaml")},
		},
		{
			name: "disagreeing pair at cwd is refused",
			files: map[string]string{
				filepath.Join(".tlc", "config.yaml"):        cascadeCfgOne,
				filepath.Join(".hop", "tlc", "config.yaml"): cascadeCfgTwo,
			},
			start:   ".",
			mode:    config.ModeHop,
			wantErr: []string{filepath.Join(".tlc", "config.yaml"), filepath.Join(".hop", "tlc", "config.yaml"), "org/one", "org/two"},
		},
		{
			name: "disagreeing pair on an ancestor is refused from a subdirectory",
			files: map[string]string{
				filepath.Join(".tlc", "config.yaml"):        cascadeCfgOne,
				filepath.Join(".hop", "tlc", "config.yaml"): cascadeCfgTwo,
			},
			dirs:    []string{filepath.Join("hops", "main", "src")},
			start:   filepath.Join("hops", "main", "src"),
			mode:    config.ModeHop,
			wantErr: []string{"org/one", "org/two"},
		},
		{
			name:     "standalone mode ignores a hop config",
			files:    map[string]string{filepath.Join(".hop", "tlc", "config.yaml"): cascadeCfgOne},
			start:    ".",
			mode:     config.ModeStandalone,
			wantNone: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// EvalSymlinks: the cascade reports resolved paths, and on
			// macOS t.TempDir() is a /var symlink into /private/var.
			root, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			for _, d := range tt.dirs {
				if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			for rel, body := range tt.files {
				writeCascadeFile(t, filepath.Join(root, rel), body)
			}
			start := filepath.Join(root, tt.start)

			got, err := resolveProjectConfigs(start, root, tt.mode)
			if len(tt.wantErr) > 0 {
				if err == nil {
					t.Fatalf("resolveProjectConfigs() = %v, want error", got)
				}
				for _, want := range tt.wantErr {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("error should mention %q, got: %v", want, err)
					}
				}
				if code := exitCodeFor(err); code != ExitConflict {
					t.Errorf("exitCodeFor(err) = %d, want %d", code, ExitConflict)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveProjectConfigs() error = %v", err)
			}
			if tt.wantNone {
				if len(got) != 0 {
					t.Fatalf("resolveProjectConfigs() = %v, want none", got)
				}
				return
			}
			if len(got) != len(tt.wantRel) {
				t.Fatalf("resolveProjectConfigs() = %v, want %d entries %v", got, len(tt.wantRel), tt.wantRel)
			}
			for i, rel := range tt.wantRel {
				if want := filepath.Join(root, rel); got[i] != want {
					t.Errorf("configs[%d] = %q, want %q", i, got[i], want)
				}
			}
		})
	}
}
