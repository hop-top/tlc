package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

const systemConfigDir = "/etc/tlc"

func UserConfigDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config directory: %w", err)
	}

	return filepath.Join(dir, "tlc"), nil
}

func UserConfigPath() (string, error) {
	dir, err := UserConfigDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, "config.yaml"), nil
}

func SystemConfigDir() string {
	return systemConfigDir
}

func SystemConfigPath() string {
	return filepath.Join(systemConfigDir, "config.yaml")
}

func WritableConfigPath(current string) (string, error) {
	if current != "" && !samePath(current, SystemConfigPath()) {
		return current, nil
	}

	return UserConfigPath()
}

func PrepareViperForWrite(v *viper.Viper) (string, error) {
	target, err := WritableConfigPath(v.ConfigFileUsed())
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		return "", fmt.Errorf("create config directory: %w", err)
	}

	v.SetConfigFile(target)
	v.SetConfigType("yaml")

	return target, nil
}

func samePath(a, b string) bool {
	a = filepath.Clean(a)
	b = filepath.Clean(b)

	if abs, err := filepath.Abs(a); err == nil {
		a = abs
	}
	if abs, err := filepath.Abs(b); err == nil {
		b = abs
	}

	return a == b
}
