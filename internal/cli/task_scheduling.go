package cli

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
	"hop.top/kit/go/core/util"
	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/core"
)

type taskScheduling struct {
	dueAt        *time.Time
	remindAt     *time.Time
	rrule        string
	noAutoRemind bool
}

func (s *taskScheduling) parse(
	due, remindAt, rrule string, noAutoRemind bool,
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
	if rrule != "" {
		if err := core.ValidateRRule(rrule); err != nil {
			return fmt.Errorf("invalid --rrule: %w", err)
		}
		s.rrule = rrule
	}
	s.noAutoRemind = noAutoRemind
	return nil
}

// applySchedulingConfig applies priority-based scheduling defaults
// from config. Does not override explicit values.
//
// The lookup goes through the decoded TaskConfig rather than
// viper.GetStringMap on an interpolated key path. The interpolated form
// composed "task.scheduling.by_priority." with the task's CANONICAL
// priority ("P0"), while viper lower-cases every map key it stores
// ("p0"), so the two never met and the rule silently no-opped for every
// priority not already spelled in lower case. Decoding re-cases those
// keys (see normalizeSchedulingKeys), so an exact match here is a real
// match.
func applySchedulingConfig(task *core.Task) {
	p := string(task.Priority)
	if p == "" {
		return
	}

	var cfg config.TaskConfig
	if err := unmarshalConfigKey("task", &cfg); err != nil {
		return
	}
	rule, ok := cfg.Scheduling.ByPriority[p]
	if !ok {
		return
	}

	defaults := core.ScheduleDefaults{Due: rule.Due}
	if rule.RRule != "" {
		if err := core.ValidateRRule(rule.RRule); err == nil {
			defaults.RRule = rule.RRule
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
