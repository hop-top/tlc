package core

import (
	"fmt"
	"testing"
)

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
