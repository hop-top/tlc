package core

import "time"

// ScheduleDefaults holds resolved scheduling defaults for a priority.
type ScheduleDefaults struct {
	Due         time.Duration
	RemindEvery time.Duration
}

// ApplySchedulingDefaults sets DueAt and RemindEvery on a task when
// not explicitly set, using priority-based defaults from config.
// Called at create time. Does not override explicit values.
func ApplySchedulingDefaults(t *Task, defaults *ScheduleDefaults) {
	if defaults == nil {
		return
	}
	if t.DueAt == nil && defaults.Due > 0 {
		d := t.CreatedAt.Add(defaults.Due)
		t.DueAt = &d
	}
	if t.RemindEvery == nil && defaults.RemindEvery > 0 {
		t.RemindEvery = &defaults.RemindEvery
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
func CheckAgeNudges(t *Task, rules []AgeNudgeConfig) *AgeNudge {
	if t.DueAt != nil { // explicit due → skip age nudges
		return nil
	}
	if t.Status == StatusDone || t.Status == StatusSkipped {
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
