package generate

import "fmt"

// Snippet returns a platform-specific configuration snippet for registering
// the given URL scheme. Use this when runtime registration is not possible.
//
// Supported platforms: "macos", "ios", "linux", "windows".
func Snippet(platform, scheme, appPath string) (string, error) {
	switch platform {
	case "macos", "ios":
		return PlistSnippet(scheme)
	case "linux":
		return DesktopFile(scheme, appPath, scheme)
	case "windows":
		return WindowsRegSnippet(scheme, appPath)
	default:
		return "", fmt.Errorf("generate: unknown platform %q", platform)
	}
}
