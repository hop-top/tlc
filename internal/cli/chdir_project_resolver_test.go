package cli

import (
	"errors"
	"strings"
	"testing"

	"hop.top/kit/go/console/hay"
	"hop.top/tlc/internal/core"
)

func TestProjectRootFromDBPath(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "standalone .tlc layout",
			in:   "/Users/jadb/.w/ideacrafterslabs/wsm/.tlc/db.sqlite",
			want: "/Users/jadb/.w/ideacrafterslabs/wsm",
		},
		{
			name: "standalone hops/main/.tlc layout",
			in:   "/Users/jadb/.w/ideacrafterslabs/aps/hops/main/.tlc/db.sqlite",
			want: "/Users/jadb/.w/ideacrafterslabs/aps/hops/main",
		},
		{
			name: "hop-mode .hop/tlc layout",
			in:   "/Users/jadb/.w/some-hop-repo/.hop/tlc/db.sqlite",
			want: "/Users/jadb/.w/some-hop-repo",
		},
		{
			name: "empty input",
			in:   "",
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := projectRootFromDBPath(tc.in)
			if got != tc.want {
				t.Errorf("projectRootFromDBPath(%q) = %q; want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestProjectDirName(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "standalone", in: "/Users/jadb/.w/wsm/.tlc/db.sqlite", want: "wsm"},
		{name: "nested", in: "/Users/jadb/.w/aps/hops/main/.tlc/db.sqlite", want: "main"},
		{name: "hop-mode", in: "/Users/jadb/.w/x/.hop/tlc/db.sqlite", want: "x"},
		{name: "empty", in: "", want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := projectDirName(tc.in)
			if got != tc.want {
				t.Errorf("projectDirName(%q) = %q; want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestChdirResolveError_Ambiguous(t *testing.T) {
	amb := &hay.ErrAmbiguous[core.RegisteredProject]{
		Query: "ev",
		Candidates: []hay.Scored[core.RegisteredProject]{
			{Item: core.RegisteredProject{ProjectID: "hop-top/eva", DBPath: "/Users/jadb/.w/eva/hops/main/.tlc/db.sqlite"}, Score: 10},
			{Item: core.RegisteredProject{ProjectID: "hop-top/eva-pkg", DBPath: "/Users/jadb/.w/eva-pkg/.tlc/db.sqlite"}, Score: 10},
		},
	}
	got := chdirResolveError("ev", amb).Error()
	if !strings.Contains(got, "ambiguous") {
		t.Errorf("error must mention ambiguity; got: %s", got)
	}
	if !strings.Contains(got, "hop-top/eva") || !strings.Contains(got, "hop-top/eva-pkg") {
		t.Errorf("error must list both candidates; got: %s", got)
	}
	if !strings.Contains(got, "Disambiguate") {
		t.Errorf("error must include disambiguation hint; got: %s", got)
	}
}

func TestChdirResolveError_NoMatch(t *testing.T) {
	nm := &hay.ErrNoMatch{Query: "zzz"}
	got := chdirResolveError("zzz", nm).Error()
	if !strings.Contains(got, "no path or registered project matches") {
		t.Errorf("error must explain no match; got: %s", got)
	}
	if !strings.Contains(got, "tlc project list") {
		t.Errorf("error must point user at `tlc project list`; got: %s", got)
	}

	nmStale := &hay.ErrNoMatch{Query: "zzz", Stale: 3}
	gotStale := chdirResolveError("zzz", nmStale).Error()
	if !strings.Contains(gotStale, "3 stale") {
		t.Errorf("error must report stale count; got: %s", gotStale)
	}
}

func TestChdirResolveError_OtherErrorPassesThrough(t *testing.T) {
	other := errors.New("disk on fire")
	got := chdirResolveError("foo", other).Error()
	if !strings.Contains(got, "foo") || !strings.Contains(got, "disk on fire") {
		t.Errorf("non-hay errors must wrap target + cause; got: %s", got)
	}
}
