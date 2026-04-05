//go:build linux

package hdl

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// ErrUnsupported satisfies the cross-platform contract; always nil on linux.
var ErrUnsupported error

// Register creates a .desktop file and sets it as the default xdg-mime handler.
func Register(scheme, appPath string) error {
	if scheme == "" {
		return fmt.Errorf("hdl: scheme must not be empty")
	}
	if appPath == "" {
		return fmt.Errorf("hdl: app must not be empty")
	}

	content := fmt.Sprintf("[Desktop Entry]\nType=Application\nName=%s\nExec=%s %%u\nMimeType=x-scheme-handler/%s;\nNoDisplay=true\n",
		scheme, appPath, scheme)

	dir := filepath.Join(os.Getenv("HOME"), ".local", "share", "applications")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("hdl: mkdir: %w", err)
	}

	name := fmt.Sprintf("%s-scheme-handler.desktop", scheme)
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		return fmt.Errorf("hdl: write desktop file: %w", err)
	}

	mimeType := fmt.Sprintf("x-scheme-handler/%s", scheme)
	if err := exec.Command("xdg-mime", "default", name, mimeType).Run(); err != nil {
		return fmt.Errorf("hdl: xdg-mime: %w", err)
	}
	exec.Command("update-desktop-database", dir).Run() // best-effort
	return nil
}
