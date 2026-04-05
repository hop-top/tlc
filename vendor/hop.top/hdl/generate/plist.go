package generate

import (
	"fmt"
	"io"
	"strings"
)

const plistFragment = `<key>CFBundleURLTypes</key>
<array>
	<dict>
		<key>CFBundleURLName</key>
		<string>%s</string>
		<key>CFBundleURLSchemes</key>
		<array>
			<string>%s</string>
		</array>
	</dict>
</array>`

// PlistSnippet returns the CFBundleURLTypes XML fragment for use in Info.plist.
func PlistSnippet(scheme string) (string, error) {
	if scheme == "" {
		return "", fmt.Errorf("generate: scheme must not be empty")
	}
	return fmt.Sprintf(plistFragment, scheme, scheme), nil
}

// PatchPlist reads an existing Info.plist and injects CFBundleURLTypes before </dict>\n</plist>.
func PatchPlist(r io.Reader, scheme string) (string, error) {
	if scheme == "" {
		return "", fmt.Errorf("generate: scheme must not be empty")
	}
	b, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}
	snippet, _ := PlistSnippet(scheme)
	src := string(b)
	if out := strings.Replace(src, "</dict>\n</plist>", snippet+"\n</dict>\n</plist>", 1); out != src {
		return out, nil
	}
	return strings.Replace(src, "</dict></plist>", snippet+"\n</dict></plist>", 1), nil
}
