package events

import (
	"testing"

	"hop.top/kit/bus"
)

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

func TestTypedTopicConstants_NonEmpty(t *testing.T) {
	topics := []bus.Topic{
		TopicTaskCreated, TopicTaskClaimed, TopicTaskCompleted, TopicTaskReopened,
		TopicTrackCreated, TopicTrackActivated, TopicTrackCompleted,
		TopicApsProfileAll,
	}
	for _, topic := range topics {
		if topic == "" {
			t.Fatal("expected non-empty typed topic constant")
		}
	}
}

func TestTypedTopics_MatchLegacy(t *testing.T) {
	if string(TopicTaskCreated) != TaskCreated {
		t.Fatal("TopicTaskCreated != TaskCreated")
	}
	if string(TopicTaskClaimed) != TaskClaimed {
		t.Fatal("TopicTaskClaimed != TaskClaimed")
	}
	if string(TopicTaskCompleted) != TaskCompleted {
		t.Fatal("TopicTaskCompleted != TaskCompleted")
	}
}

func TestEntityEvent_ZeroValue(t *testing.T) {
	var e EntityEvent
	if e.ID != "" || e.Actor != "" {
		t.Fatal("expected zero-value EntityEvent")
	}
}

func TestPayloadStructs_ZeroValue(t *testing.T) {
	var tc TaskCreatedPayload
	if tc.TaskID != "" {
		t.Fatal("expected zero-value TaskCreatedPayload")
	}
	var tr TaskReopenedPayload
	if tr.TaskID != "" {
		t.Fatal("expected zero-value TaskReopenedPayload")
	}
}
