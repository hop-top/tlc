package workspace

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/core"
)

// ProjectLister abstracts the storage layer for the filesystem adapter.
type ProjectLister interface {
	ListProjectsBySpace(ctx context.Context, spaceURI string) ([]core.RegisteredProject, error)
}

// FilesystemAdapter discovers projects in local filesystem spaces.
// It queries the global projects table rather than scanning directories.
type FilesystemAdapter struct {
	lister ProjectLister
}

// NewFilesystemAdapter creates a filesystem adapter backed by the given lister.
func NewFilesystemAdapter(lister ProjectLister) *FilesystemAdapter {
	return &FilesystemAdapter{lister: lister}
}

// Name returns the adapter name.
func (a *FilesystemAdapter) Name() string { return "filesystem" }

// Schemes returns URI schemes handled by this adapter.
func (a *FilesystemAdapter) Schemes() []string { return []string{"file", ""} }

// Discover returns projects belonging to the space by querying the project
// registry filtered on the space URI.
func (a *FilesystemAdapter) Discover(space config.SpaceConfig) ([]core.RegisteredProject, error) {
	uri := normalizeFileURI(space.URI)
	return a.lister.ListProjectsBySpace(context.Background(), uri)
}

// Contains reports whether localPath is a subdirectory of the space URI path.
// Returns the empty string when the path does not belong to this space.
func (a *FilesystemAdapter) Contains(space config.SpaceConfig, localPath string) string {
	spacePath := resolveFilePath(space.URI)
	if spacePath == "" {
		return ""
	}

	absLocal, err := filepath.Abs(localPath)
	if err != nil {
		return ""
	}

	absSpace, err := filepath.Abs(spacePath)
	if err != nil {
		return ""
	}

	// Ensure trailing separator for prefix check.
	prefix := absSpace
	if !strings.HasSuffix(prefix, string(os.PathSeparator)) {
		prefix += string(os.PathSeparator)
	}

	if absLocal == absSpace || strings.HasPrefix(absLocal, prefix) {
		// Return the relative path from space root as project identifier.
		rel, err := filepath.Rel(absSpace, absLocal)
		if err != nil {
			return ""
		}
		return rel
	}
	return ""
}

// normalizeFileURI strips the file:// prefix to return a plain path,
// or returns the URI as-is if it has no scheme.
func normalizeFileURI(uri string) string {
	if strings.HasPrefix(uri, "file://") {
		parsed, err := url.Parse(uri)
		if err != nil {
			return uri
		}
		return parsed.Path
	}
	return uri
}

// resolveFilePath converts a space URI to a filesystem path.
func resolveFilePath(uri string) string {
	path := normalizeFileURI(uri)
	if path == "" {
		return ""
	}
	// Expand ~ to home directory.
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		path = filepath.Join(home, path[2:])
	}
	return path
}
