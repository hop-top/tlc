package cli

// `task remind` under a RENAMED status vocabulary.
//
// The reminder buckets exempt finished work, and that exemption was
// spelled as the built-in DONE/SKIPPED constants — which a config
// declaring SHIPPED/CANCELED matches on neither. Every completed task
// with a past due date then reported as OVERDUE forever, and because
// `--check` exits non-zero whenever the overdue bucket is non-empty, no
// amount of finishing work could make it return clean again.

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/core"
)

// terminalVocabConfig names both terminal statuses SHIPPED and
// CANCELED, sharing no name with the built-in set.
func terminalVocabConfig() *config.TaskConfig {
	return &config.TaskConfig{
		Statuses: []config.StatusDefinition{
			{Name: "BACKLOG", Label: "Backlog", Role: config.RoleInitial, TLSMarker: " "},
			{Name: "DOING", Label: "Doing", Role: config.RoleActive, TLSMarker: ">"},
			{Name: "SHIPPED", Label: "Shipped", IsTerminal: true, Role: config.RoleCompleted, TLSMarker: "x"},
			{Name: "CANCELED", Label: "Canceled", IsTerminal: true, Role: config.RoleSkipped, TLSMarker: "-"},
		},
		StateMachine: &config.WorkflowDefinition{
			Rules: map[string][]string{
				"BACKLOG": {"DOING", "CANCELED"},
				"DOING":   {"SHIPPED", "CANCELED", "BACKLOG"},
			},
		},
	}
}

func TestTaskRemind_SkipsConfiguredTerminalStatuses(t *testing.T) {
	viper.Set("storage.db_path", resetTestDB(t))
	withTaskConfig(t, terminalVocabConfig())

	past := time.Now().UTC().Add(-48 * time.Hour)
	s, err := getStorageRaw()
	if err != nil {
		t.Fatalf("storage: %v", err)
	}
	seeds := []struct {
		id     string
		status core.TaskStatus
	}{
		{"T-0001", "BACKLOG"},  // open and past due: overdue
		{"T-0002", "SHIPPED"},  // finished: must not be overdue
		{"T-0003", "CANCELED"}, // finished: must not be overdue
	}
	for _, seed := range seeds {
		due := past
		if err := s.CreateTask(context.Background(), &core.Task{
			ID: seed.id, Title: seed.id, Status: seed.status, DueAt: &due,
		}); err != nil {
			t.Fatalf("seed %s: %v", seed.id, err)
		}
	}
	_ = s.Close()

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "remind"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task remind failed: %v", err)
	}

	out := buf.String()
	for _, finished := range []string{"T-0002", "T-0003"} {
		if strings.Contains(out, finished) {
			t.Errorf("%s is terminal but appears in remind output:\n%s", finished, out)
		}
	}
	if !strings.Contains(out, "T-0001") {
		t.Errorf("T-0001 is open and past due: expected it in remind output:\n%s", out)
	}
}
