package labels

import (
	"os"
	"path/filepath"
)

// DetectProjectType attempts to detect the project type based on files in the path.
func DetectProjectType(path string) ProjectType {
	if exists(filepath.Join(path, "go.mod")) {
		if exists(filepath.Join(path, "cmd")) {
			return TypeGoBinary
		}
		// Could be go-socket if we see specific imports, but simplified for now
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
