package core

import (
	"context"
	"encoding/json"
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
	return exec.CommandContext(ctx, name, args...).Output() //nolint:wrapcheck // callers add the command context; keeps *exec.ExitError assertable
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

	out, err := run("aps", "profile", "list", "--format", "json")
	if err != nil {
		return r
	}

	profiles, err := parseAPSProfiles(out)
	if err != nil {
		return r
	}

	for _, p := range profiles {
		r.profiles[strings.ToLower(p.ID)] = p.ID
	}

	// Display names come from the same listing; a name equal to the id
	// adds nothing.
	for _, p := range profiles {
		if p.DisplayName != "" && p.DisplayName != p.ID {
			r.profiles[strings.ToLower(p.DisplayName)] = p.ID
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

// apsProfile is the part of an `aps profile list --format json` row that
// the resolver reads; aps emits more columns, which decoding ignores.
type apsProfile struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
}

// parseAPSProfiles decodes `aps profile list --format json` output.
// Rows without an id are dropped. Anything that is not a JSON array of
// objects is an error, so error text or a table never yields ids.
func parseAPSProfiles(output []byte) ([]apsProfile, error) {
	var rows []apsProfile
	if err := json.Unmarshal(output, &rows); err != nil {
		return nil, fmt.Errorf("decode aps profile list: %w", err)
	}
	profiles := rows[:0]
	for _, p := range rows {
		if p.ID != "" {
			profiles = append(profiles, p)
		}
	}
	return profiles, nil
}
