package cli

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
	"hop.top/kit/go/core/util"
	"hop.top/tlc/internal/core"
)

type taskScheduling struct {
	dueAt        *time.Time
	remindAt     *time.Time
	remindEvery  *time.Duration
	noAutoRemind bool
}

func (s *taskScheduling) parse(
	due, remindAt, remindEvery string, noAutoRemind bool,
) error {
	if due != "" {
		t, err := util.ParseUntil(due)
		if err != nil {
			return fmt.Errorf("invalid --due %q: %w", due, err)
		}
		s.dueAt = &t
	}
	if remindAt != "" {
		t, err := util.ParseUntil(remindAt)
		if err != nil {
			return fmt.Errorf(
				"invalid --remind-at %q: %w", remindAt, err,
			)
		}
		s.remindAt = &t
	}
	if remindEvery != "" {
		d, err := time.ParseDuration(remindEvery)
		if err != nil {
			return fmt.Errorf(
				"invalid --remind-every %q: %w", remindEvery, err,
			)
		}
		if d <= 0 {
			return fmt.Errorf(
				"--remind-every must be positive, got %q", remindEvery,
			)
		}
		s.remindEvery = &d
	}
	s.noAutoRemind = noAutoRemind
	return nil
}


// applySchedulingConfig applies priority-based scheduling defaults
// from config. Does not override explicit values.
func applySchedulingConfig(task *core.Task) {
	p := string(task.Priority)
	if p == "" {
		return
	}

	key := fmt.Sprintf("task.scheduling.by_priority.%s", p)
	sub := viper.GetStringMap(key)
	if len(sub) == 0 {
		return
	}

	var defaults core.ScheduleDefaults
	if v, ok := sub["due"]; ok {
		if d, err := time.ParseDuration(fmt.Sprint(v)); err == nil {
			defaults.Due = d
		}
	}
	if v, ok := sub["remind_every"]; ok {
		if d, err := time.ParseDuration(fmt.Sprint(v)); err == nil {
			defaults.RemindEvery = d
		}
	}
	core.ApplySchedulingDefaults(task, &defaults)
}

// CollectAgeNudges scans tasks for age-based nudges per config.
func CollectAgeNudges(tasks []*core.Task) []*core.AgeNudge {
	raw := viper.Get("task.scheduling.age_nudges")
	if raw == nil {
		return nil
	}

	items, ok := raw.([]interface{})
	if !ok {
		// Try typed slice from YAML
		if typed, ok2 := raw.([]map[string]interface{}); ok2 {
			for _, m := range typed {
				items = append(items, m)
			}
		}
	}
	if len(items) == 0 {
		return nil
	}

	var rules []core.AgeNudgeConfig
	for _, item := range items {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		var r core.AgeNudgeConfig
		if v, ok := m["status"]; ok {
			r.Status = fmt.Sprint(v)
		}
		if v, ok := m["threshold"]; ok {
			if d, err := time.ParseDuration(
				fmt.Sprint(v),
			); err == nil {
				r.Threshold = d
			}
		}
		if v, ok := m["action"]; ok {
			r.Action = fmt.Sprint(v)
		}
		rules = append(rules, r)
	}

	var nudges []*core.AgeNudge
	for _, t := range tasks {
		if n := core.CheckAgeNudges(t, rules); n != nil {
			nudges = append(nudges, n)
		}
	}
	return nudges
}
