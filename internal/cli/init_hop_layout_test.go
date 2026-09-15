package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

const (
	initFullStandaloneCfg = "version: 0.1\nproject:\n  id: org/repo\nstorage:\n  backend: sqlite\n"
)

func writeInitFixture(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func mkdirInitFixture(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o750); err != nil {
		t.Fatal(err)
	}
}

// TestInitCmd_HopLayout pins which directory `tlc init` scaffolds into
// and when it refuses. A bare .hop/ (repair.lock, backups/ — what other
// tooling leaves behind) is not a hop config, so init scaffolds the
// standalone .tlc/. Any existing local config, in either layout, makes
// init refuse unless --force, instead of writing a second config next
// to the first.
func TestInitCmd_HopLayout(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(t *testing.T, dir string)
		args        []string
		wantErr     bool
		wantErrHas  []string
		wantExit    int
		wantExist   []string
		wantMissing []string
	}{
		{
			name: "bare .hop scaffolds standalone .tlc",
			setup: func(t *testing.T, dir string) {
				mkdirInitFixture(t, filepath.Join(dir, ".hop", "backups"))
				writeInitFixture(t, filepath.Join(dir, ".hop", "repair.lock"), "")
			},
			args:        []string{"init", "--no-track"},
			wantExist:   []string{filepath.Join(".tlc", "config.yaml")},
			wantMissing: []string{filepath.Join(".hop", "tlc")},
		},
		{
			name: "existing .tlc beside bare .hop refuses",
			setup: func(t *testing.T, dir string) {
				writeInitFixture(t, filepath.Join(dir, ".tlc", "config.yaml"), initFullStandaloneCfg)
				mkdirInitFixture(t, filepath.Join(dir, ".hop", "backups"))
				writeInitFixture(t, filepath.Join(dir, ".hop", "repair.lock"), "")
			},
			args:        []string{"init", "--no-track"},
			wantErr:     true,
			wantErrHas:  []string{"already initialized at", filepath.Join(".tlc", "config.yaml"), "--force"},
			wantExit:    ExitConflict,
			wantMissing: []string{filepath.Join(".hop", "tlc")},
		},
		{
			name: "existing .tlc.yaml refuses",
			setup: func(t *testing.T, dir string) {
				writeInitFixture(t, filepath.Join(dir, ".tlc.yaml"), initFullStandaloneCfg)
			},
			args:        []string{"init", "--no-track"},
			wantErr:     true,
			wantErrHas:  []string{"already initialized at", ".tlc.yaml"},
			wantExit:    ExitConflict,
			wantMissing: []string{".tlc", filepath.Join(".hop", "tlc")},
		},
		{
			name: "existing .hop/tlc refuses and leaves .tlc alone",
			setup: func(t *testing.T, dir string) {
				writeInitFixture(t, filepath.Join(dir, ".hop", "tlc", "config.yaml"), initFullStandaloneCfg)
			},
			args:        []string{"init", "--no-track"},
			wantErr:     true,
			wantErrHas:  []string{"already initialized at", filepath.Join(".hop", "tlc", "config.yaml")},
			wantExit:    ExitConflict,
			wantMissing: []string{".tlc", ".tlc.yaml"},
		},
		{
			name: "--force overwrites the existing standalone config in place",
			setup: func(t *testing.T, dir string) {
				writeInitFixture(t, filepath.Join(dir, ".tlc", "config.yaml"), initFullStandaloneCfg)
				mkdirInitFixture(t, filepath.Join(dir, ".hop", "backups"))
			},
			args:        []string{"init", "--no-track", "--force"},
			wantExist:   []string{filepath.Join(".tlc", "config.yaml")},
			wantMissing: []string{filepath.Join(".hop", "tlc")},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("TLC_MODE", "")
			dir := t.TempDir()
			t.Chdir(dir)
			resetInvocationDirForTest()
			viper.Reset()
			isolateInitTest(t)
			tt.setup(t, dir)

			cmd := newTestCmd()
			cmd.AddCommand(newTestInitCmd())
			buf := new(strings.Builder)
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			cmd.SetArgs(tt.args)

			err := cmd.Execute()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Execute() error = %v, wantErr %v\n%s", err, tt.wantErr, buf.String())
			}
			if err != nil {
				for _, want := range tt.wantErrHas {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("error should mention %q, got: %v", want, err)
					}
				}
				if got := exitCodeFor(err); got != tt.wantExit {
					t.Errorf("exitCodeFor(err) = %d, want %d (err: %v)", got, tt.wantExit, err)
				}
			}
			for _, rel := range tt.wantExist {
				if _, statErr := os.Stat(filepath.Join(dir, rel)); statErr != nil {
					t.Errorf("expected %s to exist: %v", rel, statErr)
				}
			}
			for _, rel := range tt.wantMissing {
				if _, statErr := os.Stat(filepath.Join(dir, rel)); statErr == nil {
					t.Errorf("expected %s to be absent, but it exists", rel)
				}
			}
		})
	}
}
