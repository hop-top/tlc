package uri

import (
	"fmt"
	"net/url"
	"strings"
)

// URI represents a structured identifier (e.g., tlc://project/task).
type URI struct {
	Scheme string
	Space  string // Host/Org/User
	ID     string // ProjectID/TaskID or slug
}

// Parse converts a string into a URI. It handles both full URIs and
// shorthand identifiers (e.g., "project/task" -> space: project, id: task).
func Parse(s string) (*URI, error) {
	if s == "" {
		return nil, fmt.Errorf("empty URI")
	}

	// Shorthand handling: "space/id" or "id"
	if !strings.Contains(s, "://") {
		parts := strings.SplitN(s, "/", 2)
		if len(parts) == 2 {
			return &URI{
				Space: parts[0],
				ID:    parts[1],
			}, nil
		}
		return &URI{
			ID: s,
		}, nil
	}

	u, err := url.Parse(s)
	if err != nil {
		return nil, fmt.Errorf("invalid URI: %w", err)
	}

	return &URI{
		Scheme: u.Scheme,
		Space:  u.Host,
		ID:     strings.TrimPrefix(u.Path, "/"),
	}, nil
}

// String returns the canonical string representation.
func (u *URI) String() string {
	if u.Scheme == "" {
		if u.Space != "" {
			return fmt.Sprintf("%s/%s", u.Space, u.ID)
		}
		return u.ID
	}
	return fmt.Sprintf("%s://%s/%s", u.Scheme, u.Space, u.ID)
}
