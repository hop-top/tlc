package core

import (
	"context"
	"fmt"
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
	if _, err := exec.LookPath("aps"); err != nil {
		return &ProfileResolver{profiles: make(map[string]string)}
	}
	return newProfileResolverWith(defaultRunner)
}

// newProfileResolverWith creates a resolver using the given command runner.
// Trusts the injected runner — callers needing the on-PATH check must use
// NewProfileResolver instead.
func newProfileResolverWith(run commandRunner) *ProfileResolver {
	r := &ProfileResolver{
		profiles: make(map[string]string),
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

// GetGlobalResolver returns a process-lifetime cached ProfileResolver.
func GetGlobalResolver() *ProfileResolver {
	globalResolverOnce.Do(func() {
		globalResolver = NewProfileResolver()
	})
	return globalResolver
}

// ResolveSquadMembers returns the profile IDs of all members in an aps squad.
func ResolveSquadMembers(squadID string) ([]string, error) {
	if _, err := exec.LookPath("aps"); err != nil {
		return nil, fmt.Errorf("aps not found: %w", err)
	}
	return resolveSquadMembersWith(squadID, defaultRunner)
}

// resolveSquadMembersWith is the testable core. Trusts the injected runner —
// callers needing the on-PATH check must use ResolveSquadMembers instead.
func resolveSquadMembersWith(squadID string, run commandRunner) ([]string, error) {
	out, err := run("aps", "squad", "show", squadID)
	if err != nil {
		return nil, fmt.Errorf("aps squad show %s: %w", squadID, err)
	}
	return parseSquadMembers(string(out)), nil
}

// parseSquadMembers extracts member profile IDs from `aps squad show` output.
// Expects lines under a "members:" key, each prefixed with "- ".
func parseSquadMembers(output string) []string {
	var members []string
	inMembers := false
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "members:" {
			inMembers = true
			continue
		}
		if inMembers {
			if strings.HasPrefix(trimmed, "- ") {
				member := strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
				if member != "" {
					members = append(members, member)
				}
			} else if trimmed != "" && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
				break // new top-level key
			}
		}
	}
	return members
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
