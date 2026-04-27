package extensions

import (
	"context"
	"testing"

	"hop.top/kit/go/ai/ext"
	"hop.top/tlc/internal/auth"
)

func TestGitHubSyncExtension(t *testing.T) {
	store := auth.NewMockStore()
	e := NewGitHubSync(store)
	assertSyncExt(t, e, "github-sync", "github")
}

func TestJiraSyncExtension(t *testing.T) {
	store := auth.NewMockStore()
	e := NewJiraSync(store)
	assertSyncExt(t, e, "jira-sync", "jira")
}

func TestLinearSyncExtension(t *testing.T) {
	store := auth.NewMockStore()
	e := NewLinearSync(store)
	assertSyncExt(t, e, "linear-sync", "linear")
}

func assertSyncExt(t *testing.T, e ext.Extension, name, svc string) {
	t.Helper()

	meta := e.Meta()
	if meta.Name != name {
		t.Fatalf("expected name %q, got %q", name, meta.Name)
	}
	if meta.Version == "" {
		t.Fatal("expected non-empty version")
	}

	caps := e.Capabilities()
	for _, c := range []ext.Capability{ext.CapRegistry, ext.CapHook, ext.CapConfig} {
		if !caps.Has(c) {
			t.Fatalf("expected capability %v", c)
		}
	}

	sp, ok := e.(SyncProvider)
	if !ok {
		t.Fatal("expected SyncProvider interface")
	}
	if sp.ServiceName() != svc {
		t.Fatalf("expected service %q, got %q", svc, sp.ServiceName())
	}
	if sp.Store() == nil {
		t.Fatal("expected non-nil Store")
	}

	if err := e.Init(context.Background()); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	if err := e.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}

func TestSyncExtRegistration(t *testing.T) {
	m := New(nil, nil)
	store := auth.NewMockStore()

	m.Add(NewGitHubSync(store))
	m.Add(NewJiraSync(store))
	m.Add(NewLinearSync(store))

	if got := len(m.Extensions()); got != 3 {
		t.Fatalf("expected 3 extensions, got %d", got)
	}

	if err := m.InitAll(context.Background()); err != nil {
		t.Fatalf("InitAll failed: %v", err)
	}

	errs := m.CloseAll()
	if len(errs) != 0 {
		t.Fatalf("expected no close errors, got %v", errs)
	}
}
