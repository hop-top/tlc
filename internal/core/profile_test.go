package core

import (
	"fmt"
	"testing"
)

func stubRunner(profiles map[string]string) commandRunner {
	return func(name string, args ...string) ([]byte, error) {
		if name != "aps" {
			return nil, fmt.Errorf("unexpected command: %s", name)
		}
		if len(args) >= 2 && args[0] == "profile" && args[1] == "list" {
			var out string
			for id := range profiles {
				out += id + "\n"
			}
			return []byte(out), nil
		}
		if len(args) >= 3 && args[0] == "profile" && args[1] == "show" {
			id := args[2]
			dn, ok := profiles[id]
			if !ok {
				return nil, fmt.Errorf("profile not found: %s", id)
			}
			return []byte(fmt.Sprintf("id: %s\ndisplay_name: %s\n", id, dn)), nil
		}
		return nil, fmt.Errorf("unhandled: %v", args)
	}
}

func TestResolve_NoProfiles(t *testing.T) {
	r := &ProfileResolver{profiles: make(map[string]string)}
	got := r.Resolve("alice")
	if got != "alice" {
		t.Errorf("expected 'alice', got %q", got)
	}
}

func TestResolve_NilResolver(t *testing.T) {
	var r *ProfileResolver
	got := r.Resolve("bob")
	if got != "bob" {
		t.Errorf("expected 'bob', got %q", got)
	}
}

func TestResolve_KnownProfile(t *testing.T) {
	r := newProfileResolverWith(stubRunner(map[string]string{
		"cojadb": "cojadb",
		"exo":    "Exo Framework",
	}))

	got := r.Resolve("cojadb")
	if got != "cojadb" {
		t.Errorf("expected 'cojadb', got %q", got)
	}

	got = r.Resolve("exo")
	if got != "exo" {
		t.Errorf("expected 'exo', got %q", got)
	}
}

func TestResolve_CaseInsensitive(t *testing.T) {
	r := newProfileResolverWith(stubRunner(map[string]string{
		"cojadb": "cojadb",
		"exo":    "Exo Framework",
	}))

	got := r.Resolve("COJADB")
	if got != "cojadb" {
		t.Errorf("expected 'cojadb', got %q", got)
	}

	got = r.Resolve("EXO")
	if got != "exo" {
		t.Errorf("expected 'exo', got %q", got)
	}
}

func TestResolve_ByDisplayName(t *testing.T) {
	r := newProfileResolverWith(stubRunner(map[string]string{
		"exo": "Exo Framework",
	}))

	got := r.Resolve("Exo Framework")
	if got != "exo" {
		t.Errorf("expected 'exo', got %q", got)
	}

	// Case-insensitive display name match.
	got = r.Resolve("exo framework")
	if got != "exo" {
		t.Errorf("expected 'exo', got %q", got)
	}
}

func TestResolve_UnknownPassthrough(t *testing.T) {
	r := newProfileResolverWith(stubRunner(map[string]string{
		"exo": "Exo Framework",
	}))

	got := r.Resolve("unknown-person")
	if got != "unknown-person" {
		t.Errorf("expected 'unknown-person', got %q", got)
	}
}

func TestIsKnownProfile(t *testing.T) {
	r := newProfileResolverWith(stubRunner(map[string]string{
		"cojadb": "cojadb",
		"exo":    "Exo Framework",
	}))

	if !r.IsKnownProfile("cojadb") {
		t.Error("expected cojadb to be known")
	}
	if !r.IsKnownProfile("COJADB") {
		t.Error("expected COJADB (case-insensitive) to be known")
	}
	if !r.IsKnownProfile("Exo Framework") {
		t.Error("expected 'Exo Framework' (display name) to be known")
	}
	if r.IsKnownProfile("nobody") {
		t.Error("expected nobody to be unknown")
	}
}

func TestIsKnownProfile_NilResolver(t *testing.T) {
	var r *ProfileResolver
	if r.IsKnownProfile("test") {
		t.Error("nil resolver should return false")
	}
}

func TestListProfiles(t *testing.T) {
	r := newProfileResolverWith(stubRunner(map[string]string{
		"cojadb": "cojadb",
		"exo":    "Exo Framework",
	}))

	profiles := r.ListProfiles()
	if len(profiles) != 2 {
		t.Errorf("expected 2 profiles, got %d: %v", len(profiles), profiles)
	}

	found := make(map[string]bool)
	for _, p := range profiles {
		found[p] = true
	}
	if !found["cojadb"] || !found["exo"] {
		t.Errorf("missing expected profiles in %v", profiles)
	}
}

func TestListProfiles_Empty(t *testing.T) {
	r := &ProfileResolver{profiles: make(map[string]string)}
	if profiles := r.ListProfiles(); profiles != nil {
		t.Errorf("expected nil, got %v", profiles)
	}
}

func TestParseProfileIDs(t *testing.T) {
	out := "cojadb\nexo\n\n"
	ids := parseProfileIDs(out)
	if len(ids) != 2 || ids[0] != "cojadb" || ids[1] != "exo" {
		t.Errorf("unexpected ids: %v", ids)
	}
}

func TestParseDisplayName(t *testing.T) {
	out := "id: exo\ndisplay_name: Exo Framework\nisolation:\n"
	dn := parseDisplayName(out)
	if dn != "Exo Framework" {
		t.Errorf("expected 'Exo Framework', got %q", dn)
	}
}

func TestParseDisplayName_Missing(t *testing.T) {
	out := "id: exo\nisolation:\n"
	dn := parseDisplayName(out)
	if dn != "" {
		t.Errorf("expected empty, got %q", dn)
	}
}

func TestParseSquadMembers(t *testing.T) {
	out := `id: dev-team
type: stream-aligned
domain: core
members:
  - alice
  - bob
  - charlie
topology:
  type: mesh`
	members := parseSquadMembers(out)
	if len(members) != 3 {
		t.Fatalf("expected 3 members, got %d: %v", len(members), members)
	}
	if members[0] != "alice" || members[1] != "bob" || members[2] != "charlie" {
		t.Errorf("unexpected members: %v", members)
	}
}

func TestParseSquadMembers_Empty(t *testing.T) {
	out := `id: empty-squad
members:
topology:
  type: mesh`
	members := parseSquadMembers(out)
	if len(members) != 0 {
		t.Errorf("expected 0 members, got %d: %v", len(members), members)
	}
}

func TestParseSquadMembers_NoSection(t *testing.T) {
	out := `id: no-members
type: stream-aligned`
	members := parseSquadMembers(out)
	if len(members) != 0 {
		t.Errorf("expected 0 members, got %d: %v", len(members), members)
	}
}

func TestResolveSquadMembers_WithStub(t *testing.T) {
	runner := func(name string, args ...string) ([]byte, error) {
		if name == "aps" && len(args) >= 3 && args[0] == "squad" && args[1] == "show" {
			return []byte("id: team\nmembers:\n  - alice\n  - bob\n"), nil
		}
		return nil, fmt.Errorf("unexpected: %s %v", name, args)
	}
	members, err := resolveSquadMembersWith("team", runner)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(members) != 2 || members[0] != "alice" || members[1] != "bob" {
		t.Errorf("unexpected members: %v", members)
	}
}
