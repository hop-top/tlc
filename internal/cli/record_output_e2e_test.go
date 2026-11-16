package cli

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"hop.top/tlc/internal/core"
)

// resetRecordFlags clears --json/--jq state the global commands keep
// between Execute calls in one process.
func resetRecordFlags(cmds ...*cobra.Command) {
	for _, c := range cmds {
		if f := c.Flags().Lookup(flagRecordJSON); f != nil {
			if sv, ok := f.Value.(pflag.SliceValue); ok {
				_ = sv.Replace(nil)
			}
			f.Changed = false
		}
		if f := c.Flags().Lookup(flagRecordJQ); f != nil {
			_ = f.Value.Set("")
			f.Changed = false
		}
	}
}

func runRecordCmd(t *testing.T, args ...string) string {
	t.Helper()
	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("%v: %v\n%s", args, err, buf)
	}
	return buf.String()
}

// TestTaskJSONDisplayIDRoundTrips: a legacy row stored as "T-0734" with a
// different seq prints T-0734 in the human view; `--json display_id`
// returns the same string, and `task show <display_id>` resolves back.
func TestTaskJSONDisplayIDRoundTrips(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, err := getStorageRaw()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.CreateTask(ctx, &core.Task{
		ID: "T-0734", Title: "legacy", Status: core.StatusTodo, Priority: "P1",
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		resetRecordFlags(TaskListCmd, TaskShowCmd)
		viper.Set("output.format", "table")
	})
	viper.Set("output.format", formatTable)

	var rows []map[string]any
	out := runRecordCmd(t, "task", "list", "--json", "id,display_id,seq,priority,track_id,blocked_by")
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if len(rows) != 1 || rows[0]["display_id"] != "T-0734" || rows[0]["priority"] != "P1" {
		t.Fatalf("rows = %v", rows)
	}
	if v, ok := rows[0]["track_id"]; !ok || v != nil {
		t.Errorf("track_id must be present and null, got %v (present=%v)", v, ok)
	}
	resetRecordFlags(TaskListCmd, TaskShowCmd)
	viper.Set("output.format", formatTable)

	human := runRecordCmd(t, "task", "show", "T-0734")
	if !contains(human, "Task: T-0734") {
		t.Fatalf("human view does not show T-0734:\n%s", human)
	}
	one := runRecordCmd(t, "task", "show", "T-0734", "--json", "display_id,priority")
	var rec map[string]any
	if err := json.Unmarshal([]byte(one), &rec); err != nil {
		t.Fatalf("%v\n%s", err, one)
	}
	if rec["display_id"] != "T-0734" {
		t.Errorf("show display_id = %v", rec["display_id"])
	}
}
