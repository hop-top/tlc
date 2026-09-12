package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"hop.top/tlc/internal/core"
)

// The on-disk track scaffold is keyed by SLUG, never by the durable
// TypeID and never by a per-project display alias. The slug is what makes
// tracks/<name>/ readable in a diff or a file listing, so these tests pin
// that choice: a future change that swaps the directory name for an alias
// has to delete an assertion to do it.

// TestTrackScaffold_DirNameIsSlugNotTypeID pins that the scaffold path
// derives from the slug, and that the durable TypeID never leaks into it.
func TestTrackScaffold_DirNameIsSlugNotTypeID(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()

		cwd, _ := os.Getwd()
		tlcDir := filepath.Join(cwd, ".tlc")

		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{
			"track", "create", "Browser Rendering",
			"--type", "feature",
		})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("track create failed: %v", err)
		}

		// Directory is named for the slug.
		trackDir := filepath.Join(tlcDir, "tracks", "browser-rendering")
		if _, err := os.Stat(trackDir); err != nil {
			t.Fatalf("scaffold dir not at slug path %s: %v", trackDir, err)
		}

		// The durable TypeID is a real, distinct identifier...
		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		track, err := s.GetTrackBySlug(ctx, "", "browser-rendering")
		if err != nil {
			t.Fatalf("GetTrackBySlug: %v", err)
		}
		if !strings.HasPrefix(track.ID, "track_") {
			t.Fatalf("expected durable TypeID, got %q", track.ID)
		}
		if track.ID == track.Slug {
			t.Fatal("TypeID and slug must be distinct identifiers")
		}

		// ...and it must NOT be used as a directory name.
		if _, err := os.Stat(filepath.Join(tlcDir, "tracks", track.ID)); err == nil {
			t.Errorf("scaffold dir wrongly named for TypeID %s", track.ID)
		}

		// Nor may the TypeID leak into plan.md or the registry, both of
		// which reference the track by the name a human reads.
		planData, err := os.ReadFile(filepath.Join(trackDir, "plan.md"))
		if err != nil {
			t.Fatalf("plan.md not created: %v", err)
		}
		if contains(string(planData), track.ID) {
			t.Errorf("plan.md leaked durable TypeID %s", track.ID)
		}
		if !contains(string(planData), "browser-rendering") {
			t.Error("plan.md missing slug reference")
		}

		registryData, err := os.ReadFile(filepath.Join(tlcDir, "tracks", "tracks.md"))
		if err != nil {
			t.Fatalf("tracks.md not created: %v", err)
		}
		if contains(string(registryData), track.ID) {
			t.Errorf("tracks.md leaked durable TypeID %s", track.ID)
		}
		if !contains(string(registryData), "browser-rendering") {
			t.Error("tracks.md missing slug entry")
		}
	})
}

// TestTrackScaffold_ClampedSlugYieldsSaneDirName pins that a title long
// enough to trip the write-path limit still produces a directory name
// that is short, whole-word, and free of a trailing hyphen.
func TestTrackScaffold_ClampedSlugYieldsSaneDirName(t *testing.T) {
	withTestLock(func() {
		_, cleanup := setupTestDir(t)
		defer cleanup()

		cwd, _ := os.Getwd()
		tlcDir := filepath.Join(cwd, ".tlc")

		viper.Set("tracks.slug_max_len", 24)
		defer viper.Set("tracks.slug_max_len", 0)

		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{
			"track", "create",
			"Config driven label templates and workflow wiring",
			"--type", "feature",
		})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("track create failed: %v", err)
		}

		entries, err := os.ReadDir(filepath.Join(tlcDir, "tracks"))
		if err != nil {
			t.Fatalf("read tracks dir: %v", err)
		}
		var dirName string
		for _, e := range entries {
			if e.IsDir() {
				dirName = e.Name()
			}
		}
		if dirName == "" {
			t.Fatal("no scaffold directory created")
		}

		if len(dirName) > 24 {
			t.Errorf("scaffold dir %q exceeds clamp (%d chars)", dirName, len(dirName))
		}
		if strings.HasSuffix(dirName, "-") || strings.HasPrefix(dirName, "-") {
			t.Errorf("scaffold dir %q has a dangling hyphen", dirName)
		}
		// Clamping cuts at a word boundary, so every segment is a whole
		// word from the title rather than a truncated fragment.
		title := "config driven label templates and workflow wiring"
		for _, seg := range strings.Split(dirName, "-") {
			if seg == "" {
				t.Errorf("scaffold dir %q has an empty segment", dirName)
				continue
			}
			if !strings.Contains(title, seg) {
				t.Errorf("scaffold dir %q segment %q is not a whole title word", dirName, seg)
			}
		}
	})
}

// TestTrackScaffold_LegacyLongSlugStillResolves pins the grandfather
// guarantee: a slug minted before the write-path limit existed keeps
// resolving on the read path, and nothing renames its directory.
func TestTrackScaffold_LegacyLongSlugStillResolves(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()

		cwd, _ := os.Getwd()
		tracksRoot := filepath.Join(cwd, ".tlc", "tracks")

		// A 48-char slug: legal under the read ceiling, far over the
		// 24-char write limit now applied to new slugs.
		const legacySlug = "config-driven-label-templates-and-workflow-wiring"
		if len(legacySlug) <= 24 {
			t.Fatalf("fixture slug %q is not longer than the write limit", legacySlug)
		}
		if err := core.ValidateTrackSlug(legacySlug); err != nil {
			t.Fatalf("read-path validator rejected legacy slug: %v", err)
		}
		if err := core.ValidateNewTrackSlug(legacySlug, 24); err == nil {
			t.Fatal("write-path validator must reject a slug over the limit")
		}

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		// Seed the track the way an older version would have, bypassing
		// the service so the write-path limit does not apply.
		track := &core.Track{
			ID:     core.NewTrackID(),
			Slug:   legacySlug,
			Title:  "Config driven label templates and workflow wiring",
			Type:   core.TrackTypeFeature,
			Status: core.TrackStatusPending,
		}
		if err := s.CreateTrack(ctx, track); err != nil {
			t.Fatalf("CreateTrack: %v", err)
		}

		legacyDir := filepath.Join(tracksRoot, legacySlug)
		if err := os.MkdirAll(legacyDir, 0o755); err != nil {
			t.Fatalf("seed legacy dir: %v", err)
		}

		// Read path resolves the long slug.
		got, err := s.GetTrackBySlug(ctx, "", legacySlug)
		if err != nil {
			t.Fatalf("legacy long slug must still resolve: %v", err)
		}
		if got.Slug != legacySlug {
			t.Errorf("slug = %q, want %q", got.Slug, legacySlug)
		}

		// A successful write leaves the slug, and the directory, alone.
		got.Title = "Legacy Retitled"
		if err := s.UpdateTrack(ctx, got); err != nil {
			t.Fatalf("UpdateTrack on legacy track: %v", err)
		}
		after, err := s.GetTrackBySlug(ctx, "", legacySlug)
		if err != nil {
			t.Fatalf("legacy slug stopped resolving after update: %v", err)
		}
		if after.Slug != legacySlug {
			t.Errorf("update re-slugged track: %q, want %q", after.Slug, legacySlug)
		}
		if _, err := os.Stat(legacyDir); err != nil {
			t.Errorf("legacy track dir was renamed or removed: %v", err)
		}
	})
}
