//go:build windows

package hdl

import (
	"fmt"

	"golang.org/x/sys/windows/registry"
)

// ErrUnsupported satisfies the cross-platform contract; always nil on windows.
var ErrUnsupported error

// Register writes the URL scheme handler to HKCU\Software\Classes\<scheme>.
func Register(scheme, appPath string) error {
	if scheme == "" {
		return fmt.Errorf("hdl: scheme must not be empty")
	}
	if appPath == "" {
		return fmt.Errorf("hdl: app must not be empty")
	}

	base := `Software\Classes\` + scheme
	k, _, err := registry.CreateKey(registry.CURRENT_USER, base, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("hdl: create key: %w", err)
	}
	defer k.Close()

	if err := k.SetStringValue("", "URL:"+scheme+" Protocol"); err != nil {
		return err
	}
	if err := k.SetStringValue("URL Protocol", ""); err != nil {
		return err
	}

	cmd, _, err := registry.CreateKey(registry.CURRENT_USER,
		base+`\shell\open\command`, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("hdl: create command key: %w", err)
	}
	defer cmd.Close()

	return cmd.SetStringValue("", fmt.Sprintf(`"%s" "%%1"`, appPath))
}
