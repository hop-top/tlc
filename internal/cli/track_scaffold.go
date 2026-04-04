package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
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

// scaffoldTrackDir creates tracks/<id>/ with metadata.json and plan.md,
// updates tracks/tracks.md registry, then prints next-step instructions.
// configDir is the .tlc/ (or .hop/tlc/) directory containing config.yaml.
// Scaffold errors are warnings — track creation already succeeded.
func scaffoldTrackDir(w io.Writer, track *core.Track, configDir string) {
	trackDir := filepath.Join(configDir, "tracks", track.ID)
	if err := os.MkdirAll(trackDir, 0o755); err != nil {
		_, _ = fmt.Fprintf(w, "  Warning: could not create %s: %v\n", trackDir, err)
		return
	}

	writeMetadata(w, track, trackDir)
	writePlanMD(w, track, configDir, trackDir)
	updateTracksRegistry(w, track, configDir)
	printNextSteps(w, track, configDir, trackDir)
}

func writeMetadata(w io.Writer, track *core.Track, trackDir string) {
	type meta struct {
		ID         string     `json:"id"`
		Title      string     `json:"title"`
		Type       string     `json:"type"`
		Status     string     `json:"status"`
		AssignedTo *string    `json:"assigned_to,omitempty"`
		CreatedAt  time.Time  `json:"created_at"`
	}
	m := meta{
		ID:         track.ID,
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

func writePlanMD(w io.Writer, track *core.Track, configDir, trackDir string) {
	path := filepath.Join(trackDir, "plan.md")
	// Skip if already exists (e.g. idempotent re-run).
	if _, err := os.Stat(path); err == nil {
		return
	}
	assignedTo := "-"
	if track.AssignedTo != nil {
		assignedTo = "@" + *track.AssignedTo
	}
	// Relative path from project root to the plan file for the hint.
	relBase := filepath.Base(configDir)
	content := fmt.Sprintf(`---
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
		track.ID,
		track.Title,
		track.ID, relBase, track.ID,
		track.Type,
		assignedTo,
	)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		_, _ = fmt.Fprintf(w, "  Warning: could not write %s: %v\n", path, err)
	}
}

// updateTracksRegistry appends an entry to tracks/tracks.md.
func updateTracksRegistry(w io.Writer, track *core.Track, configDir string) {
	registryPath := filepath.Join(configDir, "tracks", "tracks.md")

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

	line := fmt.Sprintf("| [%s](%s/) | %s | %s | %s |\n",
		track.ID, track.ID, track.Title, track.Type, track.Status)
	if _, err := f.WriteString(line); err != nil {
		_, _ = fmt.Fprintf(w, "  Warning: could not update tracks.md: %v\n", err)
	}
}

func printNextSteps(w io.Writer, track *core.Track, configDir, trackDir string) {
	relBase := filepath.Base(configDir)
	_, _ = fmt.Fprintf(w, "\nScaffolded:\n")
	_, _ = fmt.Fprintf(w, "  %s/\n", trackDir)
	_, _ = fmt.Fprintf(w, "    metadata.json  — track identity\n")
	_, _ = fmt.Fprintf(w, "    plan.md        — spec + task frontmatter\n")
	_, _ = fmt.Fprintf(w, "  %s/tracks/tracks.md — registry updated\n", relBase)
	_, _ = fmt.Fprintf(w, "\nNext steps:\n")
	_, _ = fmt.Fprintf(w, "  [required] Edit %s/tracks/%s/plan.md — fill objective, scope, tasks: frontmatter\n", relBase, track.ID)
	_, _ = fmt.Fprintf(w, "  [required] tlc track update %s --add-plan %s/tracks/%s/plan.md\n", track.ID, relBase, track.ID)
	_, _ = fmt.Fprintf(w, "             (links plan + bulk-creates tasks from frontmatter)\n")
	_, _ = fmt.Fprintf(w, "  [optional] Add %s/tracks/%s/spec.md — detailed spec / ADR\n", relBase, track.ID)
	_, _ = fmt.Fprintf(w, "  [optional] tlc task create \"...\" --track %s  — link tasks manually\n", track.ID)
}
