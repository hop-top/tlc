package core

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestParseCronExpr_ValidFiveField(t *testing.T) {
	cases := []string{
		"0 9 * * 4",
		"*/5 * * * *",
		"0 0 1 1 0",
		"15 14 1 * *",
	}
	for _, c := range cases {
		if _, err := ParseCronExpr(c); err != nil {
			t.Errorf("expected %q to parse, got %v", c, err)
		}
	}
}

func TestParseCronExpr_RejectsSixField(t *testing.T) {
	expr := "0 0 9 * * 4"
	_, err := ParseCronExpr(expr)
	if err == nil {
		t.Fatal("expected error on 6-field expression")
	}
	if !strings.Contains(err.Error(), "6-field cron not supported") {
		t.Errorf("error %q missing descriptive message", err)
	}
}

func TestParseCronExpr_InvalidFails(t *testing.T) {
	cases := []string{
		"@invalid",
		"99 99 * * *",
		"",
		"a b c d e",
	}
	for _, c := range cases {
		if _, err := ParseCronExpr(c); err == nil {
			t.Errorf("expected error on %q", c)
		}
	}
}

func TestValidateCronTrigger_PolicyValidation(t *testing.T) {
	good := CronTrigger{Expr: "0 9 * * 4", Missed: MissedSkip, Concurrency: ConcurrencyQueue, QueueLimit: 5}
	if err := ValidateCronTrigger(good); err != nil {
		t.Fatalf("expected ok, got %v", err)
	}
	bad := CronTrigger{Expr: "0 9 * * 4", Missed: "wat"}
	if err := ValidateCronTrigger(bad); err == nil {
		t.Fatal("expected error on bad missed policy")
	}
	bad2 := CronTrigger{Expr: "0 9 * * 4", Concurrency: "nope"}
	if err := ValidateCronTrigger(bad2); err == nil {
		t.Fatal("expected error on bad concurrency policy")
	}
	bad3 := CronTrigger{Expr: "0 9 * * 4", QueueLimit: -1}
	if err := ValidateCronTrigger(bad3); err == nil {
		t.Fatal("expected error on negative queue limit")
	}
}

func TestMissedTicks_Catchup(t *testing.T) {
	sched, err := ParseCronExpr("*/15 * * * *")
	if err != nil {
		t.Fatal(err)
	}
	from := time.Date(2026, 4, 26, 9, 0, 0, 0, time.UTC)
	to := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
	ticks := MissedTicks(sched, from, to)
	// 9:00 inclusive of Next() — robfig's Next returns the NEXT after `from`,
	// so first tick is 9:15. Through 10:00: 9:15, 9:30, 9:45, 10:00.
	if len(ticks) != 4 {
		t.Fatalf("expected 4 ticks, got %d (%v)", len(ticks), ticks)
	}
}

func TestMissedTicks_EmptyWhenNoneFire(t *testing.T) {
	sched, err := ParseCronExpr("0 0 * * *")
	if err != nil {
		t.Fatal(err)
	}
	from := time.Date(2026, 4, 26, 9, 0, 0, 0, time.UTC)
	to := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
	if got := MissedTicks(sched, from, to); len(got) != 0 {
		t.Fatalf("expected 0 ticks, got %d", len(got))
	}
}

// fakeStateStore is an in-memory FlowStateStore for tests.
type fakeStateStore struct {
	mu  sync.Mutex
	all map[string]FlowState
}

func newFakeStateStore() *fakeStateStore { return &fakeStateStore{all: map[string]FlowState{}} }

func (f *fakeStateStore) GetFlowState(_ context.Context, flowID string) (*FlowState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	st, ok := f.all[flowID]
	if !ok {
		return nil, nil
	}
	return &st, nil
}

func (f *fakeStateStore) SetFlowState(_ context.Context, st FlowState) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.all[st.FlowID] = st
	return nil
}

func TestCronScheduler_DisableHaltsRuns(t *testing.T) {
	state := newFakeStateStore()
	var fired atomic.Int64
	dispatch := func(_ context.Context, _ string, _ TriggerSource, _ time.Time) (string, error) {
		fired.Add(1)
		return "run:x", nil
	}
	s := NewCronScheduler(time.UTC, dispatch, state)
	defer func() { <-s.Stop().Done() }()

	// Use an expression that fires immediately.
	if err := s.Register("flow:x", []CronTrigger{{Expr: "* * * * *"}}, ""); err != nil {
		t.Fatal(err)
	}

	// Disable before any tick can fire.
	if err := s.SetEnabled(context.Background(), "flow:x", false); err != nil {
		t.Fatal(err)
	}
	enabled, err := s.IsEnabled(context.Background(), "flow:x")
	if err != nil {
		t.Fatal(err)
	}
	if enabled {
		t.Fatal("expected disabled")
	}

	// Synthesize a tick by calling onTick directly.
	s.onTick("flow:x", CronTrigger{Expr: "* * * * *"}, time.Now())
	time.Sleep(20 * time.Millisecond)
	if got := fired.Load(); got != 0 {
		t.Fatalf("expected 0 dispatches when disabled, got %d", got)
	}

	// Re-enable.
	if err := s.SetEnabled(context.Background(), "flow:x", true); err != nil {
		t.Fatal(err)
	}
	s.onTick("flow:x", CronTrigger{Expr: "* * * * *"}, time.Now())
	time.Sleep(50 * time.Millisecond)
	if got := fired.Load(); got != 1 {
		t.Fatalf("expected 1 dispatch after enable, got %d", got)
	}
}

func TestCronScheduler_ConcurrencySkip(t *testing.T) {
	var fired atomic.Int64
	block := make(chan struct{})
	dispatch := func(_ context.Context, _ string, _ TriggerSource, _ time.Time) (string, error) {
		fired.Add(1)
		<-block
		return "run:x", nil
	}
	s := NewCronScheduler(time.UTC, dispatch, nil)
	defer func() {
		close(block)
		<-s.Stop().Done()
	}()
	t1 := CronTrigger{Expr: "* * * * *", Concurrency: ConcurrencySkip}
	if err := s.Register("flow:x", []CronTrigger{t1}, ""); err != nil {
		t.Fatal(err)
	}
	s.onTick("flow:x", t1, time.Now())
	s.onTick("flow:x", t1, time.Now())
	s.onTick("flow:x", t1, time.Now())
	time.Sleep(30 * time.Millisecond)
	// First tick is in-flight (blocked on channel), other two are skipped.
	if got := fired.Load(); got != 1 {
		t.Fatalf("expected 1 dispatch (skip policy), got %d", got)
	}
}

func TestCronScheduler_ConcurrencyQueue(t *testing.T) {
	var fired atomic.Int64
	gate := make(chan struct{}, 3)
	dispatch := func(_ context.Context, _ string, _ TriggerSource, _ time.Time) (string, error) {
		fired.Add(1)
		<-gate
		return "run:x", nil
	}
	s := NewCronScheduler(time.UTC, dispatch, nil)
	defer func() { <-s.Stop().Done() }()
	t1 := CronTrigger{Expr: "* * * * *", Concurrency: ConcurrencyQueue, QueueLimit: 5}
	if err := s.Register("flow:x", []CronTrigger{t1}, ""); err != nil {
		t.Fatal(err)
	}
	// 3 ticks: 1 fires, 2 queued.
	s.onTick("flow:x", t1, time.Now())
	s.onTick("flow:x", t1, time.Now())
	s.onTick("flow:x", t1, time.Now())
	time.Sleep(20 * time.Millisecond)
	if got := fired.Load(); got != 1 {
		t.Fatalf("expected 1 dispatch in-flight, got %d", got)
	}
	// Drain queue.
	gate <- struct{}{}
	gate <- struct{}{}
	gate <- struct{}{}
	time.Sleep(50 * time.Millisecond)
	if got := fired.Load(); got != 3 {
		t.Fatalf("expected 3 dispatches drained, got %d", got)
	}
}

func TestCronScheduler_ParallelAlwaysFires(t *testing.T) {
	var fired atomic.Int64
	block := make(chan struct{})
	dispatch := func(_ context.Context, _ string, _ TriggerSource, _ time.Time) (string, error) {
		fired.Add(1)
		<-block
		return "run:x", nil
	}
	s := NewCronScheduler(time.UTC, dispatch, nil)
	defer func() {
		close(block)
		<-s.Stop().Done()
	}()
	t1 := CronTrigger{Expr: "* * * * *", Concurrency: ConcurrencyParallel}
	if err := s.Register("flow:x", []CronTrigger{t1}, ""); err != nil {
		t.Fatal(err)
	}
	s.onTick("flow:x", t1, time.Now())
	s.onTick("flow:x", t1, time.Now())
	s.onTick("flow:x", t1, time.Now())
	time.Sleep(30 * time.Millisecond)
	if got := fired.Load(); got != 3 {
		t.Fatalf("expected 3 parallel dispatches, got %d", got)
	}
}

func TestCronScheduler_TimezoneRespected(t *testing.T) {
	s := NewCronScheduler(time.UTC, nil, nil)
	defer func() { <-s.Stop().Done() }()
	if err := s.Register("flow:tz", []CronTrigger{{Expr: "0 9 * * *"}}, "America/New_York"); err != nil {
		t.Fatal(err)
	}
	// No assertion on tick time (would couple to wall clock); ensure
	// invalid tz is rejected with a clear error.
	err := s.Register("flow:bad", []CronTrigger{{Expr: "0 9 * * *"}}, "Bogus/Land")
	if err == nil {
		t.Fatal("expected error on bogus timezone")
	}
}

func TestCronScheduler_ParseFlow_YAML_CronTrigger(t *testing.T) {
	yaml := `
flow_id: "flow:cron:1"
name: "cron flow"
version: "1.0"
entry_step: "s1"
triggers:
  cron:
    - expr: "0 9 * * 4"
      missed: skip
      concurrency: queue
      queue_limit: 3
timezone: "America/New_York"
steps:
  s1:
    step_id: "s1"
    type: "task"
    title: "do thing"
`
	f, err := ParseFlow(strings.NewReader(yaml), "flow.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if f.Triggers == nil || len(f.Triggers.Cron) != 1 {
		t.Fatalf("expected 1 cron trigger, got %+v", f.Triggers)
	}
	ct := f.Triggers.Cron[0]
	if ct.Expr != "0 9 * * 4" {
		t.Errorf("expr = %q", ct.Expr)
	}
	if ct.Missed != MissedSkip {
		t.Errorf("missed = %q", ct.Missed)
	}
	if ct.Concurrency != ConcurrencyQueue {
		t.Errorf("concurrency = %q", ct.Concurrency)
	}
	if ct.QueueLimit != 3 {
		t.Errorf("queue_limit = %d", ct.QueueLimit)
	}
	if f.Timezone != "America/New_York" {
		t.Errorf("timezone = %q", f.Timezone)
	}
}

func TestCronScheduler_ParseFlow_RejectsBadCron(t *testing.T) {
	yaml := `
flow_id: "flow:bad"
name: "bad"
version: "1.0"
entry_step: "s1"
triggers:
  cron:
    - expr: "@invalid"
steps:
  s1: { step_id: "s1", type: "task", title: "x" }
`
	_, err := ParseFlow(strings.NewReader(yaml), "f.yaml")
	if err == nil {
		t.Fatal("expected error on invalid cron")
	}
}

func TestCronScheduler_ParseFlow_RejectsBadTimezone(t *testing.T) {
	yaml := `
flow_id: "flow:bad"
name: "bad"
version: "1.0"
entry_step: "s1"
timezone: "Bogus/Land"
steps:
  s1: { step_id: "s1", type: "task", title: "x" }
`
	_, err := ParseFlow(strings.NewReader(yaml), "f.yaml")
	if err == nil {
		t.Fatal("expected error on bad tz")
	}
}
