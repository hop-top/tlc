package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/viper"
	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/core"
)

// resolveConfigDir returns the .tlc/ (or .hop/tlc/) directory that
// contains config.yaml for the current project. It walks from cwd
// upward looking for the local config dir. Returns "" if not found.
func resolveConfigDir() string {
	// First: if viper loaded a config file, derive from it.
	if used := viper.ConfigFileUsed(); used != "" {
		dir := filepath.Dir(used)
		base := filepath.Base(dir)
		mode := config.DetectMode()
		configDirName := config.LocalConfigDir(mode)
		// If the config file is inside a config dir (e.g. .tlc/config.yaml),
		// the parent dir IS the config dir.
		if base == filepath.Base(configDirName) {
			return dir
		}
		// Flat config (e.g. .tlc.yaml at project root): look for the
		// dir-style config dir alongside it.
		candidate := filepath.Join(dir, configDirName)
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
		// Flat config exists but no dir — create the dir-style config dir.
		if err := os.MkdirAll(candidate, 0o755); err == nil {
			return candidate
		}
	}
	return ""
}

// trackScaffoldDirName returns the directory name used for the on-disk
// track scaffold. The slug is the human-friendly identifier; fall back
// to the durable typeid only when no slug exists (legacy data).
func trackScaffoldDirName(track *core.Track) string {
	if track.Slug != "" {
		return track.Slug
	}
	return track.ID
}

// scaffoldTrackDir creates tracks/<slug>/ with metadata.json and plan.md,
// updates tracks/tracks.md registry, then prints next-step instructions.
// configDir is the .tlc/ (or .hop/tlc/) directory containing config.yaml.
// Scaffold errors are warnings — track creation already succeeded.
func scaffoldTrackDir(w io.Writer, track *core.Track, configDir string) {
	scaffoldTrackDirWithPlan(w, track, configDir, "")
}

// scaffoldTrackDirWithPlan is scaffoldTrackDir with plan.md supplied by
// the caller — a recipe's rendered track.plan. Empty writes the template
// and the manual next steps; a supplied plan has its tasks materialized
// already, so the next step is to run the track.
func scaffoldTrackDirWithPlan(w io.Writer, track *core.Track, configDir, plan string) {
	trackDir := filepath.Join(configDir, tracksDir(), trackScaffoldDirName(track))
	if err := os.MkdirAll(trackDir, 0o755); err != nil {
		_, _ = fmt.Fprintf(w, "  Warning: could not create %s: %v\n", trackDir, err)
		return
	}

	writeMetadata(w, track, trackDir)
	writePlanMD(w, track, configDir, trackDir, plan)
	updateTracksRegistry(w, track, configDir)
	if plan != "" {
		printRecipeNextSteps(w, track, trackDir)
		return
	}
	printNextSteps(w, track, configDir, trackDir)
}

func writeMetadata(w io.Writer, track *core.Track, trackDir string) {
	type meta struct {
		ID         string    `json:"id"`
		Slug       string    `json:"slug,omitempty"`
		Title      string    `json:"title"`
		Type       string    `json:"type"`
		Status     string    `json:"status"`
		AssignedTo *string   `json:"assigned_to,omitempty"`
		CreatedAt  time.Time `json:"created_at"`
	}
	m := meta{
		ID:         track.ID,
		Slug:       track.Slug,
		Title:      track.Title,
		Type:       track.Type,
		Status:     string(track.Status),
		AssignedTo: track.AssignedTo,
		CreatedAt:  track.CreatedAt,
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		_, _ = fmt.Fprintf(w, "  Warning: could not marshal metadata: %v\n", err)
		return
	}
	path := filepath.Join(trackDir, "metadata.json")
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		_, _ = fmt.Fprintf(w, "  Warning: could not write %s: %v\n", path, err)
	}
}

// writePlanMD writes plan.md: the given content when set, else the
// scaffold template. An existing file is left alone (idempotent re-run).
func writePlanMD(w io.Writer, track *core.Track, configDir, trackDir, content string) {
	path := filepath.Join(trackDir, "plan.md")
	if _, err := os.Stat(path); err == nil {
		return
	}
	if content == "" {
		content = planTemplate(track, configDir)
	}
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil { //nolint:gosec // G306: scaffold files are project-shared, same mode as metadata.json
		_, _ = fmt.Fprintf(w, "  Warning: could not write %s: %v\n", path, err)
	}
}

// planTemplate is the plan.md a hand-made track starts from.
func planTemplate(track *core.Track, configDir string) string {
	assignedTo := "-"
	if track.AssignedTo != nil {
		assignedTo = "@" + *track.AssignedTo
	}
	// Relative path from project root to the config dir for the hint.
	// In hop mode this is ".hop/tlc"; in standalone mode ".tlc".
	relBase := configDirRelToCwd(configDir)
	dirName := trackScaffoldDirName(track)
	return fmt.Sprintf(
		`---
title: %q
tracks:
  - %s
tasks: []
---

# %s

<!-- spec.md describes WHAT to build; plan.md describes HOW. -->

## Objective

TODO: describe the goal of this track.

## Scope

TODO: list what is in scope and out of scope.

## Tasks

<!-- Add tasks to the 'tasks:' frontmatter above, then run:  -->
<!--   tlc track update %s --add-plan %s/tracks/%s/plan.md   -->
<!-- to bulk-create linked tasks.                             -->

## Notes

- Type: %s
- Assigned: %s
`,
		track.Title,
		dirName,
		track.Title,
		dirName, relBase, dirName,
		track.Type,
		assignedTo,
	)
}

// printRecipeNextSteps follows a recipe-built track: its tasks exist, so
// the plan needs no frontmatter ingest.
func printRecipeNextSteps(w io.Writer, track *core.Track, trackDir string) {
	dirName := trackScaffoldDirName(track)
	_, _ = fmt.Fprintf(w, "\nScaffolded:\n")
	_, _ = fmt.Fprintf(w, "  %s/\n", trackDir)
	_, _ = fmt.Fprintf(w, "    metadata.json  — track identity\n")
	_, _ = fmt.Fprintf(w, "    plan.md        — the recipe's plan\n")
	_, _ = fmt.Fprintf(w, "\nNext steps:\n")
	_, _ = fmt.Fprintf(w, "  tlc track show %s      — the materialized tasks\n", dirName)
	_, _ = fmt.Fprintf(w, "  tlc track execute %s   — run them\n", dirName)
}

// updateTracksRegistry appends an entry to tracks/tracks.md.
func updateTracksRegistry(w io.Writer, track *core.Track, configDir string) {
	registryPath := filepath.Join(configDir, tracksDir(), "tracks.md")

	// Create registry with header if missing.
	if _, err := os.Stat(registryPath); os.IsNotExist(err) {
		header := "# Tracks Registry\n\n" +
			"| ID | Title | Type | Status |\n" +
			"|----|-------|------|--------|\n"
		if err := os.WriteFile(registryPath, []byte(header), 0o644); err != nil {
			_, _ = fmt.Fprintf(w, "  Warning: could not create tracks.md: %v\n", err)
			return
		}
	}

	f, err := os.OpenFile(registryPath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		_, _ = fmt.Fprintf(w, "  Warning: could not open tracks.md: %v\n", err)
		return
	}
	defer f.Close()

	dirName := trackScaffoldDirName(track)
	line := fmt.Sprintf("| [%s](%s/) | %s | %s | %s |\n",
		dirName, dirName, track.Title, track.Type, track.Status)
	if _, err := f.WriteString(line); err != nil {
		_, _ = fmt.Fprintf(w, "  Warning: could not update tracks.md: %v\n", err)
	}
}

// configDirRelToCwd returns configDir relative to cwd when possible,
// preserving multi-segment prefixes like ".hop/tlc". Falls back to the
// basename of configDir if cwd is unknown or configDir is unrelated.
func configDirRelToCwd(configDir string) string {
	cwd, err := os.Getwd()
	if err != nil {
		return filepath.Base(configDir)
	}
	rel, err := filepath.Rel(cwd, configDir)
	if err != nil || strings.HasPrefix(rel, "..") {
		return filepath.Base(configDir)
	}
	return rel
}

func printNextSteps(w io.Writer, track *core.Track, configDir, trackDir string) {
	relBase := configDirRelToCwd(configDir)
	_, _ = fmt.Fprintf(w, "\nScaffolded:\n")
	_, _ = fmt.Fprintf(w, "  %s/\n", trackDir)
	_, _ = fmt.Fprintf(w, "    metadata.json  — track identity\n")
	_, _ = fmt.Fprintf(w, "    plan.md        — spec + task frontmatter\n")
	_, _ = fmt.Fprintf(w, "  %s/tracks/tracks.md — registry updated\n", relBase)
	_, _ = fmt.Fprintf(w, "\nNext steps:\n")
	dirName := trackScaffoldDirName(track)
	_, _ = fmt.Fprintf(w, "  [required] Edit %s/tracks/%s/plan.md — fill objective, scope, tasks: frontmatter\n", relBase, dirName)
	_, _ = fmt.Fprintf(w, "  [required] tlc track update %s --add-plan %s/tracks/%s/plan.md\n", dirName, relBase, dirName)
	_, _ = fmt.Fprintf(w, "             (links plan + bulk-creates tasks from frontmatter)\n")
	_, _ = fmt.Fprintf(w, "  [optional] Add %s/tracks/%s/spec.md — detailed spec / ADR\n", relBase, dirName)
	_, _ = fmt.Fprintf(w, "  [optional] tlc task create \"...\" --track %s  — link tasks manually\n", dirName)
}
