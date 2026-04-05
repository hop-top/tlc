package generate

import "fmt"

// WindowsRegSnippet returns .reg file content for registering the URL scheme on Windows.
func WindowsRegSnippet(scheme, appPath string) (string, error) {
	if scheme == "" {
		return "", fmt.Errorf("generate: scheme must not be empty")
	}
	return fmt.Sprintf(
		"Windows Registry Editor Version 5.00\r\n\r\n[HKEY_CURRENT_USER\\Software\\Classes\\%s]\r\n@=\"URL:%s Protocol\"\r\n\"URL Protocol\"=\"\"\r\n\r\n[HKEY_CURRENT_USER\\Software\\Classes\\%s\\shell\\open\\command]\r\n@=\"\\\"%s\\\" \\\"%%1\\\"\"\r\n",
		scheme, scheme, scheme, appPath,
	), nil
}
