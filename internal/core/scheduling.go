package core

import "time"

// ScheduleDefaults holds resolved scheduling defaults for a priority.
type ScheduleDefaults struct {
	Due   time.Duration
	RRule string
}

// ApplySchedulingDefaults sets DueAt and RRule on a task when not
// explicitly set, using priority-based defaults from config. Called at
// create time. Does not override explicit values.
func ApplySchedulingDefaults(t *Task, defaults *ScheduleDefaults) {
	if defaults == nil {
		return
	}
	if t.DueAt == nil && defaults.Due > 0 {
		d := t.CreatedAt.Add(defaults.Due)
		t.DueAt = &d
	}
	if t.RRule == "" && defaults.RRule != "" {
		t.RRule = defaults.RRule
	}
}

// AgeNudge represents a triggered age-based nudge.
type AgeNudge struct {
	TaskID    string
	Status    TaskStatus
	Age       time.Duration
	Action    string // "remind" or "escalate"
	Threshold time.Duration
}

// CheckAgeNudges evaluates a task against age-based nudge rules.
// Returns nil if no nudge is triggered.
//
// Finished work is exempt, and TERMINALITY is what "finished" means —
// asked of the configured workflow, not matched against the built-in
// DONE/SKIPPED constants. The literal those constants replaced answered
// for one vocabulary only: a config declaring SHIPPED and CANCELED as
// its terminal statuses matched neither, so completed tasks kept
// generating age nudges forever, growing staler with every run. Asking
// the workflow makes the exemption travel with whatever the user named
// their terminal statuses, exactly as it does on the track and task
// update paths.
func CheckAgeNudges(t *Task, rules []AgeNudgeConfig) *AgeNudge {
	if t.DueAt != nil { // explicit due → skip age nudges
		return nil
	}
	if DefaultWorkflow().IsTerminal(t.Status) {
		return nil
	}

	age := time.Since(t.UpdatedAt)
	for _, r := range rules {
		if r.Status != "" && string(t.Status) != r.Status {
			continue
		}
		if age > r.Threshold {
			return &AgeNudge{
				TaskID:    t.ID,
				Status:    t.Status,
				Age:       age,
				Action:    r.Action,
				Threshold: r.Threshold,
			}
		}
	}
	return nil
}

// AgeNudgeConfig mirrors config.AgeNudgeRule but avoids import cycle.
type AgeNudgeConfig struct {
	Status    string
	Threshold time.Duration
	Action    string
}
