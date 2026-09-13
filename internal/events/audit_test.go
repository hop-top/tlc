package events

import (
	"context"
	"fmt"
	"testing"
	"time"

	"hop.top/kit/go/runtime/bus"
	"hop.top/tlc/internal/core"
)

type mockLogRepo struct {
	entries []*core.LogEntry
}

func (m *mockLogRepo) AddLog(_ context.Context, entry *core.LogEntry) error {
	m.entries = append(m.entries, entry)
	return nil
}

func (m *mockLogRepo) GetLogs(_ context.Context, _ string, _ string) ([]*core.LogEntry, error) {
	return m.entries, nil
}

func (m *mockLogRepo) ListLogs(_ context.Context, _ core.LogQuery) ([]*core.LogEntry, error) {
	return m.entries, nil
}

func (m *mockLogRepo) UpdateLogNote(_ context.Context, logID int64, note string, meta map[string]any) error {
	for _, e := range m.entries {
		if e.ID == logID {
			e.Note = note
			e.Meta = meta
			return nil
		}
	}
	return fmt.Errorf("log entry %d not found", logID)
}

func TestAuditSubscriber_PersistsEvent(t *testing.T) {
	b := bus.New()
	defer b.Close(context.Background())

	repo := &mockLogRepo{}
	sub := NewAuditSubscriber(b, repo)
	defer sub.Close()

	err := b.Publish(context.Background(), bus.NewEvent(
		bus.Topic(TaskCreated),
		"test",
		&EntityEvent{
			ID:        "T-0042",
			NewState:  "TODO",
			Actor:     "agent",
			Timestamp: time.Now().UTC(),
		},
	))
	if err != nil {
		t.Fatalf("publish failed: %v", err)
	}

	if len(repo.entries) != 1 {
		t.Fatalf("expected 1 audit entry, got %d", len(repo.entries))
	}

	entry := repo.entries[0]
	if entry.TaskID != "T-0042" {
		t.Fatalf("expected task ID T-0042, got %s", entry.TaskID)
	}
	if entry.Action != core.ActionCreated {
		t.Fatalf("expected action CREATED, got %s", entry.Action)
	}
	if entry.By != "agent" {
		t.Fatalf("expected by=agent, got %s", entry.By)
	}
}

func TestAuditSubscriber_ValuePayload(t *testing.T) {
	b := bus.New()
	defer b.Close(context.Background())

	repo := &mockLogRepo{}
	sub := NewAuditSubscriber(b, repo)
	defer sub.Close()

	// Test with value (non-pointer) EntityEvent payload.
	err := b.Publish(context.Background(), bus.NewEvent(
		bus.Topic(TaskClaimed),
		"test",
		EntityEvent{
			ID:       "T-0099",
			OldState: "TODO",
			NewState: "IN_PROGRESS",
			Actor:    "dev",
		},
	))
	if err != nil {
		t.Fatalf("publish failed: %v", err)
	}

	if len(repo.entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(repo.entries))
	}
	if repo.entries[0].Note != "TODO -> IN_PROGRESS" {
		t.Fatalf("unexpected note: %s", repo.entries[0].Note)
	}
}

func TestTopicToAction_Coverage(t *testing.T) {
	tests := []struct {
		topic string
		want  string
	}{
		{TaskCreated, core.ActionCreated},
		{TaskClaimed, core.ActionClaimed},
		{TaskCompleted, core.ActionDone},
		{TaskStatusChanged, core.ActionUpdated},
		{TrackCreated, "TRACK_CREATED"},
		{TrackActivated, "TRACK_ACTIVATED"},
		{TrackCompleted, "TRACK_COMPLETED"},
		{"unknown.topic", "unknown.topic"},
	}
	for _, tt := range tests {
		got := topicToAction(tt.topic)
		if got != tt.want {
			t.Errorf("topicToAction(%q) = %q, want %q", tt.topic, got, tt.want)
		}
	}
}
