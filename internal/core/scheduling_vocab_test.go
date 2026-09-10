package core

// Age nudges under a RENAMED status vocabulary.
//
// The nudge skip is a terminality question, not a name question: a task
// that has reached the end of its lifecycle must stop generating nudges
// whatever its status is called. Naming the built-in DONE/SKIPPED
// constants answered it for one vocabulary only, so a config declaring
// SHIPPED/CANCELED as its terminal statuses kept nudging finished work
// forever — with no way to turn it off short of deleting the rule.

import (
	"testing"
	"time"

	"hop.top/tlc/internal/config"
)

// twoTerminalVocabConfig extends the package's customVocabConfig with a
// second terminal status, CANCELED. Two terminals is the shape that
// matters here: the built-in set has two, so a fix that recognized only
// one would pass against a single-terminal vocabulary and still be
// wrong. No name is shared with the built-in set, so any assertion that
// passes under it passed on meaning rather than on a remembered literal.
func twoTerminalVocabConfig() *config.TaskConfig {
	cfg := customVocabConfig()
	cfg.Statuses = append(cfg.Statuses, config.StatusDefinition{
		Name: "CANCELED", Label: "Canceled",
		IsTerminal: true, Role: config.RoleSkipped, TLSMarker: "-",
	})
	cfg.StateMachine = &config.WorkflowDefinition{
		Rules: map[string][]string{
			"BACKLOG": {"DOING", "CANCELED"},
			"DOING":   {"SHIPPED", "CANCELED", "BACKLOG"},
		},
	}
	return cfg
}

// TestCheckAgeNudges_SkipsConfiguredTerminalStatuses is the regression:
// terminal statuses the config named itself must silence the nudge, and
// the non-terminal ones must still fire so the fix cannot be a blanket
// "never nudge".
func TestCheckAgeNudges_SkipsConfiguredTerminalStatuses(t *testing.T) {
	withTaskConfigProvider(t, twoTerminalVocabConfig)

	stale := time.Now().UTC().Add(-72 * time.Hour)
	// Empty Status matches every status, so the rule itself draws no
	// distinction: whatever filtering happens is terminality's doing.
	rules := []AgeNudgeConfig{{Threshold: 48 * time.Hour, Action: "remind"}}

	cases := []struct {
		status    TaskStatus
		wantNudge bool
	}{
		{"BACKLOG", true},
		{"DOING", true},
		{"SHIPPED", false},
		{"CANCELED", false},
	}

	for _, tc := range cases {
		t.Run(string(tc.status), func(t *testing.T) {
			task := &Task{ID: "T-0001", Status: tc.status, UpdatedAt: stale}
			got := CheckAgeNudges(task, rules)
			if tc.wantNudge && got == nil {
				t.Fatalf("status %s is not terminal: expected a nudge, got none", tc.status)
			}
			if !tc.wantNudge && got != nil {
				t.Fatalf("status %s is terminal: expected no nudge, got %+v", tc.status, got)
			}
		})
	}
}

// TestCheckAgeNudges_BuiltinVocabularyUnchanged keeps the default set on
// exactly the behavior it had, so the config-driven rule is a
// generalization rather than a change of meaning.
func TestCheckAgeNudges_BuiltinVocabularyUnchanged(t *testing.T) {
	withTaskConfigProvider(t, nil)

	stale := time.Now().UTC().Add(-72 * time.Hour)
	rules := []AgeNudgeConfig{{Threshold: 48 * time.Hour, Action: "remind"}}

	for status, wantNudge := range map[TaskStatus]bool{
		StatusTodo:       true,
		StatusInProgress: true,
		StatusDone:       false,
		StatusSkipped:    false,
	} {
		task := &Task{ID: "T-0002", Status: status, UpdatedAt: stale}
		got := CheckAgeNudges(task, rules)
		if wantNudge != (got != nil) {
			t.Errorf("status %s: nudge=%v, want %v", status, got != nil, wantNudge)
		}
	}
}
