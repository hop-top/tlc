package flowtest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolvePassthroughBin(t *testing.T) {
	t.Parallel()

	write := func(t *testing.T, dir, name string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte("bin"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("prefers dedicated passthrough shim", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		write(t, dir, shimPassthroughName)
		write(t, dir, shimCatchallName)

		got := resolvePassthroughBin(dir)
		if want := filepath.Join(dir, shimPassthroughName); got != want {
			t.Errorf("resolvePassthroughBin = %q, want %q", got, want)
		}
	})

	t.Run("falls back to catchall", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		write(t, dir, shimCatchallName)

		got := resolvePassthroughBin(dir)
		if want := filepath.Join(dir, shimCatchallName); got != want {
			t.Errorf("resolvePassthroughBin = %q, want %q", got, want)
		}
	})

	t.Run("empty when neither helper exists", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		write(t, dir, "passthrough") // bare tool-style name must NOT match

		if got := resolvePassthroughBin(dir); got != "" {
			t.Errorf("resolvePassthroughBin = %q, want empty", got)
		}
	})
}

// TestExtractShimsPassthroughSymlinkTarget asserts that when the built shim
// set includes the dedicated passthrough shim, passthrough-always tools are
// symlinked to it — not silently downgraded to the catchall (which records
// cassettes for tools that should always run for real).
func TestExtractShimsPassthroughSymlinkTarget(t *testing.T) {
	t.Parallel()

	entries, err := shimsFS.ReadDir("shims/bin")
	if err != nil {
		t.Fatalf("read embedded shims: %v", err)
	}
	built := map[string]bool{}
	for _, e := range entries {
		built[e.Name()] = true
	}
	if len(built) <= 1 { // only .gitkeep/.gitignore → shims not built
		t.Skip("shim binaries not embedded; run `make build-shims` first")
	}

	if built["passthrough"] {
		t.Errorf("embedded shim set contains bare %q; helper shims must use the %q name",
			"passthrough", shimPassthroughName)
	}
	if !built[shimPassthroughName] {
		t.Fatalf("embedded shim set missing %s; got %v", shimPassthroughName, keys(built))
	}

	sb := &Sandbox{BinDir: t.TempDir()}
	if err := sb.ExtractShims(); err != nil {
		t.Fatalf("ExtractShims: %v", err)
	}

	want := filepath.Join(sb.BinDir, shimPassthroughName)
	for _, name := range passthroughAlways {
		target, err := os.Readlink(filepath.Join(sb.BinDir, name))
		if err != nil {
			t.Fatalf("readlink %s: %v", name, err)
		}
		if target != want {
			t.Errorf("symlink %s → %s, want %s", name, target, want)
		}
	}
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
