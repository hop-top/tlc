package core

import (
	"fmt"
	"strconv"
	"strings"
)

// PhaseProgress holds completion stats for a single phase.
type PhaseProgress struct {
	Phase     int    `json:"phase"`
	Label     string `json:"label,omitempty"` // reserved for future use
	Total     int    `json:"total"`
	Completed int    `json:"completed"`
}

// TrackProgress aggregates phase-level and overall completion for a track.
type TrackProgress struct {
	TotalTasks     int             `json:"total_tasks"`
	CompletedTasks int             `json:"completed_tasks"`
	Phases         []PhaseProgress `json:"phases"`
	CurrentPhase   int             `json:"current_phase"`
	TotalPhases    int             `json:"total_phases"`
	Unphased       []*Task         `json:"unphased,omitempty"`
}

// ComputeTrackProgress derives phase progress from a set of linked tasks.
//
// Tasks are assigned to phases via a "phase:N" tag. Tasks without a phase
// tag go into the Unphased bucket. A task is considered completed if its
// status is DONE or SKIPPED. CurrentPhase is the lowest phase number that
// still has incomplete tasks (0 if all phases are complete or no phases
// exist).
func ComputeTrackProgress(tasks []*Task) TrackProgress {
	wm := DefaultWorkflow()

	// phase number -> index in progress slice
	phaseMap := make(map[int]*PhaseProgress)
	var unphased []*Task
	maxPhase := 0
	totalCompleted := 0

	for _, t := range tasks {
		done := wm.IsTerminal(t.Status)
		if done {
			totalCompleted++
		}

		phase, ok := parsePhaseTag(t.Tags)
		if !ok {
			unphased = append(unphased, t)
			continue
		}

		if phase > maxPhase {
			maxPhase = phase
		}

		pp, exists := phaseMap[phase]
		if !exists {
			pp = &PhaseProgress{Phase: phase}
			phaseMap[phase] = pp
		}
		pp.Total++
		if done {
			pp.Completed++
		}
	}

	// Build ordered phase slice (1..maxPhase), including empty phases.
	phases := make([]PhaseProgress, 0, maxPhase)
	currentPhase := 0
	for i := 1; i <= maxPhase; i++ {
		pp, exists := phaseMap[i]
		if !exists {
			pp = &PhaseProgress{Phase: i}
		}
		phases = append(phases, *pp)
		if currentPhase == 0 && pp.Total > pp.Completed {
			currentPhase = i
		}
	}

	return TrackProgress{
		TotalTasks:     len(tasks),
		CompletedTasks: totalCompleted,
		Phases:         phases,
		CurrentPhase:   currentPhase,
		TotalPhases:    maxPhase,
		Unphased:       unphased,
	}
}

// FormatProgress returns a human-readable progress string.
// Format: "5/8 (P2/3)" — completed/total (current-phase/total-phases).
// If no phases exist, returns "5/8".
func FormatProgress(p TrackProgress) string {
	base := fmt.Sprintf("%d/%d", p.CompletedTasks, p.TotalTasks)
	if p.TotalPhases == 0 {
		return base
	}
	return fmt.Sprintf("%s (P%d/%d)", base, p.CurrentPhase, p.TotalPhases)
}

// parsePhaseTag extracts the phase number from a "phase:N" tag.
// Returns (N, true) if found, (0, false) otherwise.
func parsePhaseTag(tags []string) (int, bool) {
	for _, tag := range tags {
		if !strings.HasPrefix(tag, "phase:") {
			continue
		}
		n, err := strconv.Atoi(strings.TrimPrefix(tag, "phase:"))
		if err != nil || n < 1 {
			continue
		}
		return n, true
	}
	return 0, false
}
