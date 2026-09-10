package labels

import (
	"os"
	"path/filepath"
)

// DetectProjectType attempts to detect the project type based on files in the path.
//
// Detection reaches every type GetTemplates has a case for, which is the
// property that keeps the two in step: a type worth offering under
// --type is a type worth guessing, and a type nothing can guess is a
// type a user only finds by reading the source.
//
// The go.mod branch returns TypeGoBinary whether or not `cmd/` exists.
// It previously tested for `cmd/` and returned TypeGoBinary either way —
// a branch left over from an intended go-socket split that never
// happened. A Go module without `cmd/` is a library, not a socket
// server, and library is a shape this file does not yet distinguish.
func DetectProjectType(path string) ProjectType {
	if exists(filepath.Join(path, "go.mod")) {
		return TypeGoBinary
	}

	if exists(filepath.Join(path, "package.json")) {
		if exists(filepath.Join(path, "src", "App.tsx")) || exists(filepath.Join(path, "src", "App.jsx")) {
			return TypeReactFrontend
		}
	}

	if exists(filepath.Join(path, "manage.py")) || exists(filepath.Join(path, "requirements.txt")) {
		return TypePythonMVC
	}

	return TypeGeneric
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}
