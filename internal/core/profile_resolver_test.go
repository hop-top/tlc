package core

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// Fixtures under testdata/aps were captured from a real aps run against a
// throwaway data dir holding invented profiles, stdout only (the resolver
// reads through a pipe, so no terminal rendering is involved):
//
//	aps profile list --format json   -> profile_list.json
//	aps profile list                 -> profile_list_table.txt
//
// The profiles are avery ("Avery Quinn", has an email), bolt-bot ("Bolt"),
// Casey_Dev (no display name, so aps echoes the id), noor ("Noor") and
// quirky (a display name that needs quoting in YAML).
var fixtureProfileIDs = []string{"Casey_Dev", "avery", "bolt-bot", "noor", "quirky"}

func readAPSFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "aps", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(b)
}

// recordingAPS answers like the real aps: structured output only when
// asked for with `--format json`, the human table otherwise. Any other
// subcommand is an error, so a resolver that strays from `profile list`
// fails loudly instead of being fed something made up.
type recordingAPS struct {
	jsonOut  string
	tableOut string
	calls    [][]string
}

func (a *recordingAPS) run(name string, args ...string) ([]byte, error) {
	a.calls = append(a.calls, append([]string{name}, args...))
	if name == "aps" && slices.Equal(args, []string{"profile", "list", "--format", "json"}) {
		return []byte(a.jsonOut), nil
	}
	if name == "aps" && slices.Equal(args, []string{"profile", "list"}) {
		return []byte(a.tableOut), nil
	}
	return nil, errors.New("unexpected aps invocation")
}

func realAPS(t *testing.T) *recordingAPS {
	t.Helper()
	return &recordingAPS{
		jsonOut:  readAPSFixture(t, "profile_list.json"),
		tableOut: readAPSFixture(t, "profile_list_table.txt"),
	}
}

func sortedProfiles(r *ProfileResolver) []string {
	got := r.ListProfiles()
	slices.Sort(got)
	return got
}

func TestProfileResolver_ReadsIDsFromStructuredList(t *testing.T) {
	r := newProfileResolverWith(realAPS(t).run)

	if got := sortedProfiles(r); !reflect.DeepEqual(got, fixtureProfileIDs) {
		t.Fatalf("profile ids = %v, want %v", got, fixtureProfileIDs)
	}
	for _, id := range fixtureProfileIDs {
		if got := r.Resolve(id); got != id {
			t.Errorf("Resolve(%q) = %q, want the id unchanged", id, got)
		}
		if !r.IsKnownProfile(id) {
			t.Errorf("IsKnownProfile(%q) = false, want true", id)
		}
	}
}

func TestProfileResolver_IDLookupIsCaseInsensitive(t *testing.T) {
	r := newProfileResolverWith(realAPS(t).run)

	cases := map[string]string{
		"AVERY":     "avery",
		"Bolt-Bot":  "bolt-bot",
		"casey_dev": "Casey_Dev",
		"CASEY_DEV": "Casey_Dev",
	}
	for in, want := range cases {
		if got := r.Resolve(in); got != want {
			t.Errorf("Resolve(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestProfileResolver_DisplayNameResolvesToID(t *testing.T) {
	r := newProfileResolverWith(realAPS(t).run)

	cases := map[string]string{
		"Avery Quinn": "avery", // a display name with a space
		"avery quinn": "avery", // case-insensitive
		"BOLT":        "bolt-bot",
		"Noor":        "noor",
		`Yes: "No"`:   "quirky", // quoting survives, unlike the YAML text of `aps profile show`
	}
	for in, want := range cases {
		if got := r.Resolve(in); got != want {
			t.Errorf("Resolve(%q) = %q, want %q", in, got, want)
		}
		if !r.IsKnownProfile(in) {
			t.Errorf("IsKnownProfile(%q) = false, want true", in)
		}
	}
}

func TestProfileResolver_UnknownPassesThrough(t *testing.T) {
	r := newProfileResolverWith(realAPS(t).run)

	for _, in := range []string{"nobody", "avery q", "avery quinn jr", ""} {
		if got := r.Resolve(in); got != in {
			t.Errorf("Resolve(%q) = %q, want it unchanged", in, got)
		}
		if r.IsKnownProfile(in) && in != "" {
			t.Errorf("IsKnownProfile(%q) = true, want false", in)
		}
	}
}

func TestProfileResolver_DisplayNameEqualToIDIsNotDuplicated(t *testing.T) {
	r := newProfileResolverWith(realAPS(t).run)

	// Casey_Dev has no display name, so aps echoes the id into it.
	if got := len(r.ListProfiles()); got != len(fixtureProfileIDs) {
		t.Errorf("ListProfiles() has %d entries, want %d", got, len(fixtureProfileIDs))
	}
}

// The structured form is requested explicitly; plain `aps profile list`
// is a table meant for people and changes shape with the terminal.
func TestProfileResolver_AsksForStructuredList(t *testing.T) {
	aps := realAPS(t)
	newProfileResolverWith(aps.run)

	want := []string{"aps", "profile", "list", "--format", "json"}
	if len(aps.calls) == 0 || !slices.Equal(aps.calls[0], want) {
		t.Fatalf("first aps call = %v, want %v", aps.calls, want)
	}
}

// The table is what plain `aps profile list` prints. It must never be
// mistaken for ids, whatever path hands it to the resolver.
func TestProfileResolver_TableTextIsNotParsedAsIDs(t *testing.T) {
	table := readAPSFixture(t, "profile_list_table.txt")
	header, _, _ := strings.Cut(table, "\n")

	r := newProfileResolverWith(func(string, ...string) ([]byte, error) {
		return []byte(table), nil
	})

	if got := r.ListProfiles(); got != nil {
		t.Errorf("ListProfiles() = %q, want none", got)
	}
	if r.IsKnownProfile(strings.TrimSpace(header)) {
		t.Errorf("table header %q registered as a profile id", header)
	}
	for _, line := range strings.Split(table, "\n") {
		if strings.TrimSpace(line) != "" && r.IsKnownProfile(line) {
			t.Errorf("table line %q registered as a profile id", line)
		}
	}
}

func TestProfileResolver_EmptyResolverWhenAPSOutputIsUnusable(t *testing.T) {
	cases := map[string]struct {
		out string
		err error
	}{
		"command fails":      {err: errors.New("exit status 1")},
		"no output":          {out: ""},
		"whitespace only":    {out: "\n\n"},
		"error text":         {out: "Error: NOT_FOUND: no such thing\n"},
		"not json":           {out: "<html>nope</html>"},
		"truncated json":     {out: `[{"id": "avery", "display_na`},
		"object not array":   {out: `{"id": "avery", "display_name": "Avery Quinn"}`},
		"wrong field types":  {out: `[{"id": 7, "display_name": ["x"]}]`},
		"array of strings":   {out: `["avery", "noor"]`},
		"json null":          {out: `null`},
		"empty array":        {out: `[]`},
		"rows without an id": {out: `[{"display_name": "Nobody"}, {"id": "", "display_name": "Blank"}]`},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			r := newProfileResolverWith(func(string, ...string) ([]byte, error) {
				return []byte(tc.out), tc.err
			})
			if got := r.ListProfiles(); got != nil {
				t.Errorf("ListProfiles() = %q, want none", got)
			}
			if got := r.Resolve("avery"); got != "avery" {
				t.Errorf("Resolve(%q) = %q, want it unchanged", "avery", got)
			}
			if r.IsKnownProfile("Nobody") || r.IsKnownProfile("Blank") || r.IsKnownProfile("") {
				t.Error("a row without an id was registered")
			}
		})
	}
}

// A row that decodes but lacks an id is skipped without discarding the
// rows around it.
func TestProfileResolver_SkipsRowsWithoutIDKeepsTheRest(t *testing.T) {
	r := newProfileResolverWith(func(string, ...string) ([]byte, error) {
		return []byte(`[{"display_name":"Ghost"},{"id":"noor","display_name":"Noor"}]`), nil
	})
	if got := r.ListProfiles(); !reflect.DeepEqual(got, []string{"noor"}) {
		t.Errorf("ListProfiles() = %q, want [noor]", got)
	}
	if r.IsKnownProfile("Ghost") {
		t.Error("row without an id registered its display name")
	}
}
