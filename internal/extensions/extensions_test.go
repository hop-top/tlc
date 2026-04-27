package extensions

import (
	"context"
	"testing"

	"hop.top/kit/go/ai/ext"
)

type stubExt struct {
	name    string
	caps    ext.Capability
	inited  bool
	closed  bool
	initErr error
}

func (s *stubExt) Meta() ext.Metadata {
	return ext.Metadata{Name: s.name, Version: "0.1.0", Description: "stub"}
}
func (s *stubExt) Capabilities() ext.Capability { return s.caps }
func (s *stubExt) Init(_ context.Context) error  { s.inited = true; return s.initErr }
func (s *stubExt) Close() error                  { s.closed = true; return nil }

func TestNew(t *testing.T) {
	m := New(nil, nil)
	if m == nil {
		t.Fatal("expected non-nil Manager")
	}
	if got := len(m.Extensions()); got != 0 {
		t.Fatalf("expected 0 extensions, got %d", got)
	}
}

func TestAddAndInitAll(t *testing.T) {
	m := New(nil, nil)
	s := &stubExt{name: "test-ext", caps: ext.CapRegistry | ext.CapConfig}
	m.Add(s)

	if got := len(m.Extensions()); got != 1 {
		t.Fatalf("expected 1 extension, got %d", got)
	}

	if err := m.InitAll(context.Background()); err != nil {
		t.Fatalf("InitAll failed: %v", err)
	}
	if !s.inited {
		t.Fatal("expected extension to be initialised")
	}
}

func TestCloseAll(t *testing.T) {
	m := New(nil, nil)
	s := &stubExt{name: "close-ext", caps: ext.CapRegistry}
	m.Add(s)

	errs := m.CloseAll()
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
	if !s.closed {
		t.Fatal("expected extension to be closed")
	}
}
