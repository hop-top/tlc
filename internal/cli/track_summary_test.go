package cli

import (
	"bytes"
	"testing"
	"time"

	"github.com/spf13/viper"
	"hop.top/tlc/internal/core"
)

func TestTrackSummary_Empty(t *testing.T) {
	withTestLock(func() {
		_, cleanup := setupTestDir(t)
		defer cleanup()

		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"track", "summary"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("track summary failed: %v", err)
		}

		output := buf.String()
		if !contains(output, "Active: 0") {
			t.Errorf("expected Active: 0 in output, got: %s", output)
		}
		if !contains(output, "ok") {
			t.Errorf("expected health ok in output, got: %s", output)
		}
	})
}

func TestTrackSummary_WithActiveTracks(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		now := time.Now().UTC()
		tracks := []*core.Track{
			{
				ID: "track-one", Title: "Track One",
				Type: "feature", Status: core.TrackStatusActive,
				CreatedAt: now, UpdatedAt: now,
			},
			{
				ID: "track-two", Title: "Track Two",
				Type: "bug", Status: core.TrackStatusActive,
				CreatedAt: now, UpdatedAt: now,
			},
			{
				ID: "track-three", Title: "Track Three",
				Type: "feature", Status: core.TrackStatusCompleted,
				CreatedAt: now, UpdatedAt: now,
			},
		}
		for _, tr := range tracks {
			if err := s.CreateTrack(ctx, tr); err != nil {
				t.Fatalf("CreateTrack %s: %v", tr.ID, err)
			}
		}

		// Create tasks for track-one (1/2 done = 50%).
		for _, task := range []*core.Task{
			{ID: "T-0001", Title: "A", Status: "DONE", TrackID: strPtr("track-one")},
			{ID: "T-0002", Title: "B", Status: "TODO", TrackID: strPtr("track-one")},
		} {
			if err := s.CreateTask(ctx, task); err != nil {
				t.Fatalf("CreateTask: %v", err)
			}
		}

		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"track", "summary"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("track summary failed: %v", err)
		}

		output := buf.String()
		if !contains(output, "Active: 2") {
			t.Errorf("expected Active: 2, got: %s", output)
		}
		if !contains(output, "Completed: 1") {
			t.Errorf("expected Completed: 1, got: %s", output)
		}
		if !contains(output, "track-one") {
			t.Errorf("expected track-one in table, got: %s", output)
		}
		if !contains(output, "track-two") {
			t.Errorf("expected track-two in table, got: %s", output)
		}
	})
}

func TestTrackSummary_Overcommitted(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		viper.Set("tracks.health.max_active", 2)

		now := time.Now().UTC()
		for _, tr := range []*core.Track{
			{
				ID: "t1", Title: "T1", Type: "feature",
				Status: core.TrackStatusActive,
				CreatedAt: now, UpdatedAt: now,
			},
			{
				ID: "t2", Title: "T2", Type: "feature",
				Status: core.TrackStatusActive,
				CreatedAt: now, UpdatedAt: now,
			},
		} {
			if err := s.CreateTrack(ctx, tr); err != nil {
				t.Fatalf("CreateTrack: %v", err)
			}
		}

		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"track", "summary"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("track summary failed: %v", err)
		}

		output := buf.String()
		if !contains(output, "overcommitted") {
			t.Errorf("expected overcommitted warning, got: %s", output)
		}
	})
}

func TestTrackSummary_ProgressPercent(t *testing.T) {
	tests := []struct {
		name      string
		completed int
		total     int
		want      int
	}{
		{"zero tasks", 0, 0, 0},
		{"half done", 1, 2, 50},
		{"all done", 4, 4, 100},
		{"one of three", 1, 3, 33},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := core.TrackProgress{
				CompletedTasks: tt.completed,
				TotalTasks:     tt.total,
			}
			got := summaryProgressPct(p)
			if got != tt.want {
				t.Errorf("summaryProgressPct(%d/%d) = %d, want %d",
					tt.completed, tt.total, got, tt.want)
			}
		})
	}
}

func strPtr(s string) *string { return &s }
