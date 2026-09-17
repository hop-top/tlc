package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
	"hop.top/tlc/internal/core"
)

// TestTaskRemind_ReportsUnsatisfiableRRule pins that a task whose
// RRULE can never fire is reported, not silently listed without a
// reminder: the row says the next fire is unknown and stderr carries
// the reason.
func TestTaskRemind_ReportsUnsatisfiableRRule(t *testing.T) {
	viper.Set("storage.db_path", resetTestDB(t))

	s, err := getStorageRaw()
	if err != nil {
		t.Fatalf("storage: %v", err)
	}
	created := time.Now().UTC().Add(-2 * 24 * time.Hour)
	seeds := []struct{ id, rule string }{
		{"T-0001", "FREQ=WEEKLY"},
		{"T-0002", "FREQ=YEARLY;BYMONTH=2;BYMONTHDAY=30"},
	}
	for _, seed := range seeds {
		if err := s.CreateTask(context.Background(), &core.Task{
			ID: seed.id, Title: seed.id, Status: core.StatusTodo,
			CreatedAt: created, RRule: seed.rule,
		}); err != nil {
			t.Fatalf("seed %s: %v", seed.id, err)
		}
	}
	_ = s.Close()

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(errOut)
	cmd.SetArgs([]string{"task", "remind"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task remind failed: %v", err)
	}

	got := out.String()
	weekly := lineContaining(got, "T-0001")
	if !strings.Contains(weekly, "next in") {
		t.Errorf("weekly rule should render a next reminder, got %q\n%s", weekly, got)
	}
	broken := lineContaining(got, "T-0002")
	if !strings.Contains(broken, "next unknown") {
		t.Errorf("unsatisfiable rule should be flagged on its row, got %q\n%s", broken, got)
	}
	if !strings.Contains(errOut.String(), "warning: T-0002") ||
		!strings.Contains(errOut.String(), "iteration cap") {
		t.Errorf("expected a stderr warning naming the task and the cap, got %q", errOut.String())
	}
}

func lineContaining(s, needle string) string {
	for _, line := range strings.Split(s, "\n") {
		if strings.Contains(line, needle) {
			return line
		}
	}
	return ""
}
