package core

import (
	"context"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// ProfileResolver maps assignee identifiers to aps profile IDs.
type ProfileResolver struct {
	profiles map[string]string // map[lowercased-alias]profileID
}

// commandRunner abstracts command execution for testing.
type commandRunner func(name string, args ...string) ([]byte, error)

// defaultRunner executes real shell commands with a timeout.
func defaultRunner(name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, name, args...).Output()
}

var (
	globalResolver     *ProfileResolver
	globalResolverOnce sync.Once
)

// NewProfileResolver creates a resolver by querying aps profiles.
// Falls back gracefully if aps is not available.
func NewProfileResolver() *ProfileResolver {
	return newProfileResolverWith(defaultRunner)
}

// newProfileResolverWith creates a resolver using the given command runner.
func newProfileResolverWith(run commandRunner) *ProfileResolver {
	r := &ProfileResolver{
		profiles: make(map[string]string),
	}

	if _, err := exec.LookPath("aps"); err != nil {
		return r
	}

	out, err := run("aps", "profile", "list")
	if err != nil {
		return r
	}

	ids := parseProfileIDs(string(out))
	for _, id := range ids {
		r.profiles[strings.ToLower(id)] = id
	}

	// Fetch display names for each profile.
	for _, id := range ids {
		out, err := run("aps", "profile", "show", id)
		if err != nil {
			continue
		}
		if dn := parseDisplayName(string(out)); dn != "" && dn != id {
			r.profiles[strings.ToLower(dn)] = id
		}
	}

	return r
}

// Resolve takes an assignee string and returns the canonical profile ID.
// Returns the input unchanged if no mapping found.
func (r *ProfileResolver) Resolve(assignee string) string {
	if r == nil || len(r.profiles) == 0 {
		return assignee
	}
	if id, ok := r.profiles[strings.ToLower(assignee)]; ok {
		return id
	}
	return assignee
}

// IsKnownProfile checks if the assignee maps to a known aps profile.
func (r *ProfileResolver) IsKnownProfile(assignee string) bool {
	if r == nil || len(r.profiles) == 0 {
		return false
	}
	_, ok := r.profiles[strings.ToLower(assignee)]
	return ok
}

// ListProfiles returns all known profile IDs (deduplicated).
func (r *ProfileResolver) ListProfiles() []string {
	if r == nil || len(r.profiles) == 0 {
		return nil
	}
	seen := make(map[string]bool)
	var result []string
	for _, id := range r.profiles {
		if !seen[id] {
			seen[id] = true
			result = append(result, id)
		}
	}
	return result
}

// getGlobalResolver returns a process-lifetime cached ProfileResolver.
func getGlobalResolver() *ProfileResolver {
	globalResolverOnce.Do(func() {
		globalResolver = NewProfileResolver()
	})
	return globalResolver
}

// parseProfileIDs extracts profile IDs from `aps profile list` output.
// Each non-empty line is treated as a profile ID.
func parseProfileIDs(output string) []string {
	var ids []string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			ids = append(ids, line)
		}
	}
	return ids
}

// parseDisplayName extracts the display_name from `aps profile show` output.
func parseDisplayName(output string) string {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "display_name:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "display_name:"))
		}
	}
	return ""
}
