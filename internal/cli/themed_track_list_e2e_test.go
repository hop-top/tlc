package cli

// End-to-end tests for themed track list output (story 075).
// Exercises the full CLI pipeline with seeded tracks + tasks and
// verifies four-color rendering: green (active+healthy),
// pink (active+stale/blocked), muted (abandoned), white (pending).

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/storage"
)

// seedTrackScenario creates 6 tracks + linked tasks to produce all
// four colors. Tracks go through TrackService which auto-mints TypeIDs;
// the supplied "ID" string ends up as Track.Slug. Tasks reference the
// track by its TypeID (the foreign key column).
//
//	auth      — active + healthy (recent task, no blockers) → green
//	payments  — active + healthy → green
//	cdn       — active + stale (old task UpdatedAt) → pink
//	search    — active + blocked (task blocked by external ID) → pink
//	onboard   — pending → white
//	legacy    — abandoned → muted
func seedTrackScenario(
	t *testing.T,
	s *storage.SQLiteStorage,
	ctx context.Context,
) {
	t.Helper()
	now := time.Now().UTC()

	authTrack := &core.Track{ID: "auth", Title: "Auth system", Type: "feature",
		Status: core.TrackStatusActive, CreatedAt: now, UpdatedAt: now}
	paymentsTrack := &core.Track{ID: "payments", Title: "Payment processing", Type: "feature",
		Status: core.TrackStatusActive, CreatedAt: now, UpdatedAt: now}
	cdnTrack := &core.Track{ID: "cdn", Title: "CDN migration", Type: "refactor",
		Status: core.TrackStatusActive, CreatedAt: now, UpdatedAt: now}
	searchTrack := &core.Track{ID: "search", Title: "Search rewrite", Type: "feature",
		Status: core.TrackStatusActive, CreatedAt: now, UpdatedAt: now}
	onboardTrack := &core.Track{ID: "onboard", Title: "Onboarding flow", Type: "feature",
		Status: core.TrackStatusPending, CreatedAt: now, UpdatedAt: now}
	legacyTrack := &core.Track{ID: "legacy", Title: "Legacy cleanup", Type: "refactor",
		Status: core.TrackStatusAbandoned, CreatedAt: now, UpdatedAt: now}
	for _, tr := range []*core.Track{
		authTrack, paymentsTrack, cdnTrack, searchTrack,
		onboardTrack, legacyTrack,
	} {
		seedTrack(t, ctx, s, s, tr)
	}

	// auth: recent task → healthy
	if err := s.CreateTask(ctx, &core.Task{
		ID: "T-0001", Title: "Auth task", Status: core.StatusInProgress,
		TrackID: &authTrack.ID, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seed task: %v", err)
	}

	// payments: recent task → healthy
	if err := s.CreateTask(ctx, &core.Task{
		ID: "T-0002", Title: "Payments task", Status: core.StatusInProgress,
		TrackID: &paymentsTrack.ID, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seed task: %v", err)
	}

	// cdn: old task → stale (UpdatedAt well past threshold)
	staleTime := now.Add(-7 * 24 * time.Hour)
	if err := s.CreateTask(ctx, &core.Task{
		ID: "T-0003", Title: "CDN task", Status: core.StatusInProgress,
		TrackID: &cdnTrack.ID, CreatedAt: staleTime, UpdatedAt: staleTime,
	}); err != nil {
		t.Fatalf("seed task: %v", err)
	}

	// search: task blocked by external ID → blocked
	if err := s.CreateTask(ctx, &core.Task{
		ID: "T-0004", Title: "Search task", Status: core.StatusTodo,
		TrackID: &searchTrack.ID, CreatedAt: now, UpdatedAt: now,
		Meta: map[string]interface{}{"blocked_by": "EXTERNAL-999"},
	}); err != nil {
		t.Fatalf("seed task: %v", err)
	}
}

// TestThemedTrackList_FourColors_E2E verifies that `tlc track list`
// renders four distinct row colors based on track status + state.
func TestThemedTrackList_FourColors_E2E(t *testing.T) {
	withTestLock(func() {
		_, cleanup := setupTestDir(t)
		defer cleanup()
		resetTrackListFlags()
		resetTrackFlags()
		resetTaskFlags()

		ctx := context.Background()
		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		// Set a short stale threshold so cdn track triggers stale.
		viper.Set("tracks.stale_threshold", "1h")

		seedTrackScenario(t, s, ctx)

		t.Run("all statuses", func(t *testing.T) {
			resetTrackListFlags()
			resetTrackFlags()

			cmd := newTestCmd()
			cmd.AddCommand(TrackCmd)
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			cmd.SetArgs([]string{
				"track", "list",
				"--status", "active",
				"--status", "pending",
				"--status", "abandoned",
			})

			if err := cmd.Execute(); err != nil {
				t.Fatalf("track list: %v", err)
			}

			out := buf.String()

			// Verify all 6 tracks present.
			for _, id := range []string{
				"auth", "payments", "cdn", "search", "onboard", "legacy",
			} {
				if !strings.Contains(stripAnsi(out), id) {
					t.Errorf("missing track %s in output", id)
				}
			}

			// GREEN: active + healthy
			for _, id := range []string{"auth", "payments"} {
				assertRowColor(t, out, id, ansiGreen,
					[]string{ansiPink, ansiMuted})
			}

			// PINK: active + stale/blocked
			for _, id := range []string{"cdn", "search"} {
				assertRowColor(t, out, id, ansiPink,
					[]string{ansiGreen, ansiMuted})
			}

			// MUTED: abandoned
			assertRowColor(t, out, "legacy", ansiMuted,
				[]string{ansiGreen, ansiPink})

			// WHITE: pending — no green/pink/muted in cells
			line := rowLine(out, "onboard")
			if line == "" {
				t.Error("track onboard not found in output")
			} else {
				cc := cellColors(line)
				for _, c := range cc {
					if c == ansiGreen || c == ansiPink || c == ansiMuted {
						t.Errorf("track onboard (white): unexpected cell color %q", c)
					}
				}
			}
		})

		t.Run("active only filter", func(t *testing.T) {
			resetTrackListFlags()
			resetTrackFlags()

			cmd := newTestCmd()
			cmd.AddCommand(TrackCmd)
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			cmd.SetArgs([]string{"track", "list", "--status", "active"})

			if err := cmd.Execute(); err != nil {
				t.Fatalf("track list --status active: %v", err)
			}

			out := buf.String()

			// Only active tracks visible.
			for _, id := range []string{"auth", "payments", "cdn", "search"} {
				if rowLine(out, id) == "" {
					t.Errorf("missing active track %s", id)
				}
			}
			for _, id := range []string{"onboard", "legacy"} {
				if rowLine(out, id) != "" {
					t.Errorf("non-active track %s should not appear", id)
				}
			}

			// Healthy → green, flagged → pink.
			for _, id := range []string{"auth", "payments"} {
				assertRowColor(t, out, id, ansiGreen,
					[]string{ansiPink, ansiMuted})
			}
			for _, id := range []string{"cdn", "search"} {
				assertRowColor(t, out, id, ansiPink,
					[]string{ansiGreen, ansiMuted})
			}
		})

		t.Run("headers in muted", func(t *testing.T) {
			resetTrackListFlags()
			resetTrackFlags()

			cmd := newTestCmd()
			cmd.AddCommand(TrackCmd)
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			cmd.SetArgs([]string{"track", "list", "--status", "active"})

			if err := cmd.Execute(); err != nil {
				t.Fatalf("track list: %v", err)
			}

			out := buf.String()
			for _, line := range strings.Split(out, "\n") {
				clean := stripAnsi(line)
				if strings.Contains(clean, "ID") && strings.Contains(clean, "Title") {
					if !strings.Contains(line, ansiMuted) {
						t.Errorf("header line missing muted color:\n  %q", line)
					}
					return
				}
			}
			t.Error("header line with ID and Title not found")
		})
	})
}
