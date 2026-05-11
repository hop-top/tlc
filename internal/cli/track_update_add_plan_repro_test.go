package cli

// Regression tests for T-0857: track update --add-plan must honor
// the CLI-supplied track id when multiple tracks exist, and must
// reject a frontmatter tracks: [...] that disagrees with that id.

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"hop.top/tlc/internal/core"
)

func TestTrackUpdateAddPlan_HonorsTargetTrackID(t *testing.T) {
	cases := []struct {
		name      string
		target    string // slug passed to `track update <slug>`
		fmTracks  string // optional `tracks: [...]` line for the plan frontmatter
		wantErr   bool
		wantMatch string // substring expected in error message
	}{
		{
			name:   "no frontmatter tracks, plan lands on target",
			target: "beta",
		},
		{
			name:     "frontmatter matches target slug, plan lands on target",
			target:   "beta",
			fmTracks: "tracks: [beta]",
		},
		{
			name:      "frontmatter names a different track, errors",
			target:    "alpha",
			fmTracks:  "tracks: [beta]",
			wantErr:   true,
			wantMatch: "does not match target track",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withTestLock(func() {
				ctx, cleanup := setupTestDir(t)
				defer cleanup()

				s, err := getStorageRaw()
				if err != nil {
					t.Fatalf("getStorageRaw: %v", err)
				}
				now := time.Now().UTC()
				alphaID := seedTrack(t, ctx, s, s, &core.Track{
					ID: "alpha", Title: "Alpha",
					Type: "feature", Status: core.TrackStatusActive,
					CreatedAt: now, UpdatedAt: now,
				})
				betaID := seedTrack(t, ctx, s, s, &core.Track{
					ID: "beta", Title: "Beta",
					Type: "feature", Status: core.TrackStatusActive,
					CreatedAt: now, UpdatedAt: now,
				})
				gammaID := seedTrack(t, ctx, s, s, &core.Track{
					ID: "gamma", Title: "Gamma",
					Type: "feature", Status: core.TrackStatusActive,
					CreatedAt: now, UpdatedAt: now,
				})
				s.Close()

				lines := []string{"---", "title: P"}
				if tc.fmTracks != "" {
					lines = append(lines, tc.fmTracks)
				}
				lines = append(lines, "---", "# body", "")
				planBody := strings.Join(lines, "\n")
				planPath := filepath.Join(t.TempDir(), "plan.md")
				if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
					t.Fatalf("write plan: %v", err)
				}

				cmd := newTestCmd()
				cmd.AddCommand(TrackCmd)
				buf := new(bytes.Buffer)
				cmd.SetOut(buf)
				cmd.SetErr(buf)
				cmd.SetArgs([]string{
					"track", "update", tc.target,
					"--add-plan", planPath,
				})
				execErr := cmd.Execute()

				if tc.wantErr {
					if execErr == nil {
						t.Fatalf("expected error, got success\noutput:\n%s", buf.String())
					}
					if tc.wantMatch != "" && !strings.Contains(execErr.Error(), tc.wantMatch) {
						t.Errorf("error %q missing substring %q", execErr.Error(), tc.wantMatch)
					}
					assertNoTrackHasPlan(t, planPath, alphaID, betaID, gammaID)
					return
				}
				if execErr != nil {
					t.Fatalf("unexpected error: %v\noutput:\n%s", execErr, buf.String())
				}

				wantTrackID := map[string]string{
					"alpha": alphaID, "beta": betaID, "gamma": gammaID,
				}[tc.target]
				assertOnlyTrackHasPlan(t, planPath, wantTrackID,
					alphaID, betaID, gammaID,
				)
			})
		})
	}
}

func assertOnlyTrackHasPlan(t *testing.T, planPath, want string, ids ...string) {
	t.Helper()
	s, err := getStorageRaw()
	if err != nil {
		t.Fatalf("getStorageRaw (verify): %v", err)
	}
	defer s.Close()
	ctx := context.Background()
	for _, id := range ids {
		tr, err := s.GetTrack(ctx, id)
		if err != nil {
			t.Fatalf("GetTrack %s: %v", id, err)
		}
		if tr == nil {
			t.Fatalf("track %s not found", id)
		}
		has := trackHasPlan(tr, planPath)
		if id == want && !has {
			t.Errorf("target track %s missing plan %q; meta=%v", id, planPath, tr.Meta)
		}
		if id != want && has {
			t.Errorf("non-target track %s unexpectedly got plan %q; meta=%v", id, planPath, tr.Meta)
		}
	}
}

func assertNoTrackHasPlan(t *testing.T, planPath string, ids ...string) {
	t.Helper()
	s, err := getStorageRaw()
	if err != nil {
		t.Fatalf("getStorageRaw (verify): %v", err)
	}
	defer s.Close()
	ctx := context.Background()
	for _, id := range ids {
		tr, err := s.GetTrack(ctx, id)
		if err != nil {
			t.Fatalf("GetTrack %s: %v", id, err)
		}
		if tr != nil && trackHasPlan(tr, planPath) {
			t.Errorf("track %s should not have plan after rejected update; meta=%v", id, tr.Meta)
		}
	}
}

func trackHasPlan(tr *core.Track, planPath string) bool {
	if tr.Meta == nil {
		return false
	}
	raw, ok := tr.Meta["plans"]
	if !ok {
		return false
	}
	switch v := raw.(type) {
	case []string:
		for _, p := range v {
			if p == planPath {
				return true
			}
		}
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok && s == planPath {
				return true
			}
		}
	}
	return false
}
