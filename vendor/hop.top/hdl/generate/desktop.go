package generate

import (
	"bytes"
	"fmt"
	"text/template"
)

var desktopTmpl = template.Must(template.New("desktop").Parse(`[Desktop Entry]
Type=Application
Name={{.Name}}
Exec={{.Exec}} %u
MimeType=x-scheme-handler/{{.Scheme}};
NoDisplay=true
`))

// DesktopFile returns a .desktop file for registering the given URL scheme on Linux.
func DesktopFile(scheme, execPath, name string) (string, error) {
	if scheme == "" {
		return "", fmt.Errorf("generate: scheme must not be empty")
	}
	var buf bytes.Buffer
	err := desktopTmpl.Execute(&buf, struct{ Scheme, Exec, Name string }{scheme, execPath, name})
	return buf.String(), err
}
