package events_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"hop.top/kit/go/runtime/bus"
	"hop.top/kit/go/runtime/domain"
	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/events"
)

// TestStateMachine_FiresPrefixedTopics asserts each tlc state-machine
// constructor publishes entity-specific transition topics (tlc.task.status.*,
// tlc.track.status.*, tlc.flow.status.*) instead of the kit-generic
// kit.runtime.state.* defaults. Sibling tools subscribe to these topics
// to react to lifecycle changes.
//
// Pre-transition publish errors veto the transition (the kit returns
// "pre-transition veto: <err>"). Post-transition is fire-and-forget so
// subscribers can audit/side-effect without blocking the state change.
func TestStateMachine_FiresPrefixedTopics(t *testing.T) {
	cases := []struct {
		name     string
		makeSM   func(pub *recordingPublisher) interface {
			Transition(ctx context.Context, from, to domain.State, force bool) error
		}
		from, to domain.State
		wantPre  string
		wantPost string
	}{
		{
			name: "task",
			makeSM: func(pub *recordingPublisher) interface {
				Transition(ctx context.Context, from, to domain.State, force bool) error
			} {
				return core.NewTaskStateMachine(&config.TaskConfig{}, pub)
			},
			from:     domain.State("TODO"),
			to:       domain.State("IN_PROGRESS"),
			wantPre:  "tlc.task.status.pre_transitioned",
			wantPost: "tlc.task.status.post_transitioned",
		},
		{
			name: "track",
			makeSM: func(pub *recordingPublisher) interface {
				Transition(ctx context.Context, from, to domain.State, force bool) error
			} {
				return core.NewTrackStateMachine(pub)
			},
			from:     domain.State(core.TrackStatusPending),
			to:       domain.State(core.TrackStatusActive),
			wantPre:  "tlc.track.status.pre_transitioned",
			wantPost: "tlc.track.status.post_transitioned",
		},
		{
			name: "flow",
			makeSM: func(pub *recordingPublisher) interface {
				Transition(ctx context.Context, from, to domain.State, force bool) error
			} {
				return core.NewFlowStateMachine(pub)
			},
			from:     domain.State(core.FlowStatusQueued),
			to:       domain.State(core.FlowStatusRunning),
			wantPre:  "tlc.flow.status.pre_transitioned",
			wantPost: "tlc.flow.status.post_transitioned",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pub := &recordingPublisher{}
			sm := tc.makeSM(pub)
			if err := sm.Transition(context.Background(), tc.from, tc.to, false); err != nil {
				t.Fatalf("Transition: %v", err)
			}
			pre, post := pub.waitForPair(t, time.Second)
			if pre != tc.wantPre {
				t.Errorf("pre topic = %q; want %q", pre, tc.wantPre)
			}
			if post != tc.wantPost {
				t.Errorf("post topic = %q; want %q", post, tc.wantPost)
			}
		})
	}
}

// TestStateMachine_BusE2E_TaskTransition verifies a tlc task state
// machine wired through the real kit bus delivers the post-transition
// event to a subscriber on tlc.task.status.post_transitioned.
func TestStateMachine_BusE2E_TaskTransition(t *testing.T) {
	b := bus.New()
	defer b.Close(context.Background())

	got := make(chan bus.Event, 1)
	b.Subscribe("tlc.task.status.post_transitioned", func(_ context.Context, e bus.Event) error {
		got <- e
		return nil
	})

	pub := events.NewBusPublisher(b)
	sm := core.NewTaskStateMachine(&config.TaskConfig{}, pub)

	if err := sm.Transition(context.Background(), domain.State("TODO"), domain.State("IN_PROGRESS"), false); err != nil {
		t.Fatalf("Transition: %v", err)
	}

	select {
	case ev := <-got:
		if ev.Topic != "tlc.task.status.post_transitioned" {
			t.Errorf("topic = %q; want tlc.task.status.post_transitioned", ev.Topic)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("post-transition event not received within 2s")
	}
}

// recordingPublisher captures published (topic, time) tuples for
// assertion. Implements domain.EventPublisher.
type recordingPublisher struct {
	mu     sync.Mutex
	topics []string
}

func (p *recordingPublisher) Publish(_ context.Context, topic, _ string, _ any) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.topics = append(p.topics, topic)
	return nil
}

// waitForPair blocks until both pre and post topics have been recorded
// or the deadline elapses. Returns them in publication order. Useful
// because post-transition publishes happen in a goroutine.
func (p *recordingPublisher) waitForPair(t *testing.T, d time.Duration) (string, string) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		p.mu.Lock()
		n := len(p.topics)
		p.mu.Unlock()
		if n >= 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.topics) < 2 {
		t.Fatalf("waited %s for pre+post pair; got %d topics: %v", d, len(p.topics), p.topics)
	}
	return p.topics[0], p.topics[1]
}
