package labels

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectProjectType(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "tlc-labels-test-*")
	defer os.RemoveAll(tmpDir)

	tests := []struct {
		name     string
		files    []string
		expected ProjectType
	}{
		{
			name:     "Go project",
			files:    []string{"go.mod", "main.go"},
			expected: TypeGoBinary,
		},
		{
			name:     "React project",
			files:    []string{"package.json", "src/App.tsx"},
			expected: TypeReactFrontend,
		},
		{
			name:     "Python project",
			files:    []string{"requirements.txt", "app.py"},
			expected: TypePythonMVC,
		},
		{
			name:     "Unknown project",
			files:    []string{"README.md"},
			expected: TypeGeneric,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			projDir := filepath.Join(tmpDir, tt.name)
			os.MkdirAll(projDir, 0o755)
			for _, f := range tt.files {
				fPath := filepath.Join(projDir, f)
				os.MkdirAll(filepath.Dir(fPath), 0o755)
				os.WriteFile(fPath, []byte(""), 0o644)
			}

			got := DetectProjectType(projDir)
			if got != tt.expected {
				t.Errorf("DetectProjectType() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestGetTemplates(t *testing.T) {
	templates := GetTemplates(TypeGoBinary)
	if len(templates) == 0 {
		t.Error("expected at least one template")
	}

	found := false
	for _, l := range templates {
		if l.Name == "domain:cli" {
			found = true
			break
		}
	}
	if !found {
		t.Error("missing domain:cli label in go-binary template")
	}
}
