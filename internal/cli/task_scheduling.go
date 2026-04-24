package cli

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/viper"
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
		t, err := parseFlexTime(due)
		if err != nil {
			return fmt.Errorf("invalid --due %q: %w", due, err)
		}
		s.dueAt = &t
	}
	if remindAt != "" {
		t, err := parseFlexTime(remindAt)
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

// weekdays maps lowercase weekday names to time.Weekday.
var weekdays = map[string]time.Weekday{
	"sunday":    time.Sunday,
	"monday":    time.Monday,
	"tuesday":   time.Tuesday,
	"wednesday": time.Wednesday,
	"thursday":  time.Thursday,
	"friday":    time.Friday,
	"saturday":  time.Saturday,
}

// parseFlexTime parses forward-looking date expressions.
// Supports: "tomorrow", "in 3 days", "+3d", weekday names,
// ISO 8601. Will be replaced by kit's ParseUntilAt once released.
func parseFlexTime(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, fmt.Errorf("empty date input")
	}
	now := time.Now().UTC()

	if s == "tomorrow" {
		return now.AddDate(0, 0, 1), nil
	}

	if strings.HasPrefix(s, "in ") {
		return parseForward(s, now)
	}

	if strings.HasPrefix(s, "+") && len(s) >= 3 {
		return parseShortForward(s, now)
	}

	if target, ok := weekdays[strings.ToLower(s)]; ok {
		days := int(target - now.Weekday())
		if days <= 0 {
			days += 7
		}
		return now.AddDate(0, 0, days), nil
	}

	return parseISOTime(s)
}

func parseForward(s string, now time.Time) (time.Time, error) {
	s = strings.TrimPrefix(s, "in ")
	parts := strings.SplitN(s, " ", 2)
	if len(parts) != 2 {
		return time.Time{},
			fmt.Errorf("invalid format %q", "in "+s)
	}
	n, err := strconv.Atoi(parts[0])
	if err != nil || n <= 0 {
		return time.Time{},
			fmt.Errorf("invalid count %q", parts[0])
	}
	unit := strings.TrimSuffix(parts[1], "s")
	switch unit {
	case "second":
		return now.Add(time.Duration(n) * time.Second), nil
	case "minute":
		return now.Add(time.Duration(n) * time.Minute), nil
	case "hour":
		return now.Add(time.Duration(n) * time.Hour), nil
	case "day":
		return now.AddDate(0, 0, n), nil
	case "week":
		return now.AddDate(0, 0, n*7), nil
	case "month":
		return now.AddDate(0, n, 0), nil
	case "year":
		return now.AddDate(n, 0, 0), nil
	default:
		return time.Time{},
			fmt.Errorf("unknown unit %q", parts[1])
	}
}

func parseShortForward(s string, now time.Time) (time.Time, error) {
	s = strings.TrimPrefix(s, "+")
	if len(s) < 2 {
		return time.Time{}, fmt.Errorf("too short")
	}
	suffix := s[len(s)-1]
	n, err := strconv.Atoi(s[:len(s)-1])
	if err != nil || n <= 0 {
		return time.Time{}, fmt.Errorf("invalid count")
	}
	switch suffix {
	case 's':
		return now.Add(time.Duration(n) * time.Second), nil
	case 'm':
		return now.Add(time.Duration(n) * time.Minute), nil
	case 'h':
		return now.Add(time.Duration(n) * time.Hour), nil
	case 'd':
		return now.AddDate(0, 0, n), nil
	case 'w':
		return now.AddDate(0, 0, n*7), nil
	case 'M':
		return now.AddDate(0, n, 0), nil
	case 'y':
		return now.AddDate(n, 0, 0), nil
	default:
		return time.Time{}, fmt.Errorf("unknown suffix %c", suffix)
	}
}

var isoTimeLayouts = []string{
	time.RFC3339,
	"2006-01-02T15:04:05Z07:00",
	"2006-01-02T15:04:05",
	"2006-01-02",
}

func parseISOTime(s string) (time.Time, error) {
	for _, layout := range isoTimeLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognized date format %q", s)
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
