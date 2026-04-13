package events

import (
	"context"
	"time"

	"hop.top/kit/bus"
	"hop.top/tlc/internal/core"
)

// AuditSubscriber subscribes to all tlc events on the bus and persists
// them to the task_logs table via core.LogRepository.
type AuditSubscriber struct {
	logRepo core.LogRepository
	unsub   bus.Unsubscribe
}

// NewAuditSubscriber creates and wires an audit subscriber on the bus.
// It subscribes to "tlc.#" (all tlc events) and writes each event to
// the task_logs table using the existing LogRepository.
func NewAuditSubscriber(b bus.Bus, logRepo core.LogRepository) *AuditSubscriber {
	s := &AuditSubscriber{logRepo: logRepo}
	s.unsub = b.Subscribe("tlc.#", s.handle)
	return s
}

func (s *AuditSubscriber) handle(ctx context.Context, e bus.Event) error {
	entry := &core.LogEntry{
		Timestamp: e.Timestamp,
	}

	// Extract fields from EntityEvent payload when available.
	if ee, ok := e.Payload.(*EntityEvent); ok {
		entry.TaskID = ee.ID
		entry.By = ee.Actor
		entry.Action = topicToAction(string(e.Topic))
		entry.Note = buildNote(ee)
		entry.Meta = ee.Meta
	} else if ee, ok := e.Payload.(EntityEvent); ok {
		entry.TaskID = ee.ID
		entry.By = ee.Actor
		entry.Action = topicToAction(string(e.Topic))
		entry.Note = buildNote(&ee)
		entry.Meta = ee.Meta
	} else {
		// Fallback for non-EntityEvent payloads.
		entry.Action = string(e.Topic)
		entry.By = e.Source
		entry.Timestamp = e.Timestamp
	}

	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now().UTC()
	}

	return s.logRepo.AddLog(ctx, entry)
}

// Close unsubscribes from the bus.
func (s *AuditSubscriber) Close() {
	if s.unsub != nil {
		s.unsub()
	}
}

// topicToAction maps a bus topic to the legacy action string used in
// the task_logs table.
func topicToAction(topic string) string {
	switch topic {
	case TaskCreated:
		return core.ActionCreated
	case TaskClaimed:
		return core.ActionClaimed
	case TaskCompleted:
		return core.ActionDone
	case TaskStatusChanged:
		return core.ActionUpdated
	case TrackCreated:
		return "TRACK_CREATED"
	case TrackActivated:
		return "TRACK_ACTIVATED"
	case TrackCompleted:
		return "TRACK_COMPLETED"
	case FlowStarted:
		return core.ActionFlowStart
	case FlowStepCompleted:
		return core.ActionStepEnd
	case FlowCompleted:
		return core.ActionFlowEnd
	case FlowFailed:
		return core.ActionFailure
	default:
		return topic
	}
}

// buildNote creates a human-readable note from an EntityEvent.
func buildNote(ee *EntityEvent) string {
	if ee.OldState != "" && ee.NewState != "" {
		return ee.OldState + " -> " + ee.NewState
	}
	if ee.NewState != "" {
		return ee.NewState
	}
	return ""
}
