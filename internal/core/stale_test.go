package core_test

import (
	"testing"
	"time"

	"hop.top/tlc/internal/core"
)

func TestIsStale_NotStale(t *testing.T) {
	task := &core.Task{UpdatedAt: time.Now().UTC(), StaleTimeout: ptr(2 * time.Hour)}
	if task.IsStale() {
		t.Fatal("expected not stale")
	}
}

func TestIsStale_Stale(t *testing.T) {
	task := &core.Task{
		UpdatedAt:    time.Now().UTC().Add(-3 * time.Hour),
		StaleTimeout: ptr(2 * time.Hour),
	}
	if !task.IsStale() {
		t.Fatal("expected stale")
	}
}

func TestStaleSince(t *testing.T) {
	task := &core.Task{
		UpdatedAt:    time.Now().UTC().Add(-3 * time.Hour),
		StaleTimeout: ptr(2 * time.Hour),
	}
	since := task.StaleSince()
	if since == nil || *since < time.Hour {
		t.Fatalf("expected ~1h, got %v", since)
	}
}

func TestIsStale_NilTimeout_NotStale(t *testing.T) {
	// nil timeout = no threshold; never stale without project default
	task := &core.Task{UpdatedAt: time.Now().UTC().Add(-999 * time.Hour)}
	if task.IsStale() {
		t.Fatal("nil timeout should not be stale")
	}
}

func ptr[T any](v T) *T { return &v }
