package events

import "testing"

func TestTopicConstants_NonEmpty(t *testing.T) {
	topics := []string{
		TaskCreated, TaskClaimed, TaskCompleted, TaskStatusChanged,
		TrackCreated, TrackActivated, TrackCompleted,
		FlowStarted, FlowStepCompleted, FlowCompleted, FlowFailed,
	}
	for _, topic := range topics {
		if topic == "" {
			t.Fatal("expected non-empty topic constant")
		}
	}
}

func TestEntityEvent_ZeroValue(t *testing.T) {
	var e EntityEvent
	if e.ID != "" || e.Actor != "" {
		t.Fatal("expected zero-value EntityEvent")
	}
}
