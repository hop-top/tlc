package flowtest

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed shims/bin/*
var shimsFS embed.FS

// Helper shim binary names. Helper shims are symlink targets, not tool
// names, so they carry the tlc-shim- prefix to stay out of the tool
// namespace inside the sandbox bin/ dir.
const (
	shimPassthroughName = "tlc-shim-passthrough"
	shimCatchallName    = "tlc-shim-catchall"
)

// passthroughAlways are tools that always exec the real binary without cassettes.
// Symlinked to tlc-shim-passthrough (if present) or tlc-shim-catchall.
var passthroughAlways = []string{
	"node", "vite", "next",
	"ls", "cat", "head", "tail", "wc", "grep", "pwd",
	"cp", "mkdir", "chmod", "ps", "lsof", "sleep",
}

// ExtractShims extracts embedded shim binaries into s.BinDir and creates
// passthrough-always and catch-all symlinks for tools not explicitly embedded.
func (s *Sandbox) ExtractShims() error {
	entries, err := fs.ReadDir(shimsFS, "shims/bin")
	if err != nil {
		return fmt.Errorf("flowtest: read embedded shims: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == ".gitkeep" {
			continue
		}
		data, err := shimsFS.ReadFile("shims/bin/" + entry.Name())
		if err != nil {
			return fmt.Errorf("flowtest: read shim %s: %w", entry.Name(), err)
		}
		dst := filepath.Join(s.BinDir, entry.Name())
		if err := os.WriteFile(dst, data, 0o755); err != nil {
			return fmt.Errorf("flowtest: write shim %s: %w", entry.Name(), err)
		}
	}

	passthroughBin := resolvePassthroughBin(s.BinDir)
	if passthroughBin == "" {
		// Neither helper binary embedded yet — skip passthrough symlinks.
		return nil
	}

	for _, name := range passthroughAlways {
		dst := filepath.Join(s.BinDir, name)
		if _, err := os.Lstat(dst); os.IsNotExist(err) {
			if err := os.Symlink(passthroughBin, dst); err != nil {
				return fmt.Errorf("flowtest: symlink %s → passthrough: %w", name, err)
			}
		}
	}

	return nil
}

// resolvePassthroughBin returns the passthrough-always symlink target in
// binDir. Prefers the dedicated passthrough shim; falls back to the catchall
// (which also handles passthrough via env override). Empty when neither
// helper binary exists.
func resolvePassthroughBin(binDir string) string {
	for _, name := range []string{shimPassthroughName, shimCatchallName} {
		p := filepath.Join(binDir, name)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}
