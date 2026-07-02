package cli

import (
	"fmt"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Prompt evaluation suite
//
// Measures resolution quality across a corpus of natural-language prompts.
// Each entry specifies the prompt, the expected command/args, and which
// resolution stage should handle it (stage1=regex, stage2=cross-domain,
// stage3=LLM).
//
// Run:  go test -run TestPromptEval -v ./internal/cli/
// ---------------------------------------------------------------------------

// evalResult records one evaluated prompt.
type evalResult struct {
	Name    string
	Prompt  string
	Stage   string // which stage resolved it: "stage1", "stage2", "none"
	Pass    bool
	Details string
}

// evalCase defines one test case for the evaluation suite.
type evalCase struct {
	name     string
	prompt   string
	wantCmd  string
	wantArgs []string // subset match: each element must appear in resolved args
	wantStge string   // expected stage: "stage1", "stage2", "either"
	minConf  float64  // minimum acceptable confidence
}

// promptEvalCases is the master evaluation corpus.
var promptEvalCases = []evalCase{
	// ---------------------------------------------------------------
	// Tier 1: New modifier aliases (noun+modifier, no explicit verb)
	// ---------------------------------------------------------------
	{
		name:     "incomplete tasks",
		prompt:   "incomplete tasks",
		wantCmd:  "task",
		wantArgs: []string{"list"},
		wantStge: "stage2",
		minConf:  0.8,
	},
	{
		name:     "incompleted tasks",
		prompt:   "incompleted tasks",
		wantCmd:  "task",
		wantArgs: []string{"list"},
		wantStge: "stage2",
		minConf:  0.8,
	},
	{
		name:     "open tasks",
		prompt:   "open tasks",
		wantCmd:  "task",
		wantArgs: []string{"list"},
		wantStge: "stage2",
		minConf:  0.8,
	},
	{
		name:     "pending tasks",
		prompt:   "pending tasks",
		wantCmd:  "task",
		wantArgs: []string{"list"},
		wantStge: "stage2",
		minConf:  0.8,
	},
	{
		name:     "remaining tasks",
		prompt:   "remaining tasks",
		wantCmd:  "task",
		wantArgs: []string{"list"},
		wantStge: "stage2",
		minConf:  0.8,
	},
	{
		name:     "unfinished tasks",
		prompt:   "unfinished tasks",
		wantCmd:  "task",
		wantArgs: []string{"list"},
		wantStge: "stage2",
		minConf:  0.8,
	},
	{
		name:     "overdue tasks",
		prompt:   "overdue tasks",
		wantCmd:  "task",
		wantArgs: []string{"list", "--stale"},
		wantStge: "stage2",
		minConf:  0.8,
	},

	// ---------------------------------------------------------------
	// Tier 2: Analytical / implicit queries
	// ---------------------------------------------------------------
	{
		name:     "average age of incompleted tasks",
		prompt:   "average age of incompleted tasks",
		wantCmd:  "task",
		wantArgs: []string{"list"},
		wantStge: "stage2",
		minConf:  0.7,
	},
	{
		name:     "noun-only bare tasks",
		prompt:   "tasks",
		wantCmd:  "task",
		wantArgs: []string{"list"},
		wantStge: "stage2",
		minConf:  0.8,
	},

	// ---------------------------------------------------------------
	// Tier 3: Ambiguous single-word (LLM fallback acceptable)
	// ---------------------------------------------------------------
	{
		name:     "gaps (bare word)",
		prompt:   "gaps",
		wantCmd:  "task",
		wantArgs: []string{"list"},
		wantStge: "either",
		minConf:  0.5,
	},
	{
		name:     "what's left",
		prompt:   "what's left",
		wantCmd:  "task",
		wantArgs: []string{"list"},
		wantStge: "either",
		minConf:  0.5,
	},
	{
		name:     "progress",
		prompt:   "progress",
		wantCmd:  "track",
		wantArgs: []string{"summary"},
		wantStge: "either",
		minConf:  0.5,
	},
	{
		name:     "status",
		prompt:   "status",
		wantCmd:  "track",
		wantArgs: []string{"summary"},
		wantStge: "either",
		minConf:  0.5,
	},

	// ---------------------------------------------------------------
	// Tier 4: Regression baselines (must keep passing)
	// ---------------------------------------------------------------
	{
		name:     "list tasks (baseline)",
		prompt:   "list tasks",
		wantCmd:  "task",
		wantArgs: []string{"list"},
		wantStge: "stage1",
		minConf:  1.0,
	},
	{
		name:     "active tracks (baseline)",
		prompt:   "active tracks",
		wantCmd:  "track",
		wantArgs: []string{"list", "--status"},
		wantStge: "stage2",
		minConf:  0.8,
	},
	{
		name:     "blocked tasks (baseline)",
		prompt:   "blocked tasks",
		wantCmd:  "task",
		wantArgs: []string{"list", "--blocked"},
		wantStge: "stage1",
		minConf:  1.0,
	},
	{
		name:     "my tasks (baseline)",
		prompt:   "my tasks",
		wantCmd:  "task",
		wantArgs: []string{"list", "--mine"},
		wantStge: "stage1",
		minConf:  1.0,
	},
	{
		name:     "stale tasks (baseline)",
		prompt:   "stale tasks",
		wantCmd:  "task",
		wantArgs: []string{"list", "--stale"},
		wantStge: "stage2",
		minConf:  0.8,
	},
	{
		name:     "done tasks (baseline)",
		prompt:   "done tasks",
		wantCmd:  "task",
		wantArgs: []string{"list", "--status"},
		wantStge: "stage2",
		minConf:  0.8,
	},
	{
		name:     "how many tasks (baseline)",
		prompt:   "how many tasks",
		wantCmd:  "task",
		wantArgs: []string{"list", "--counters"},
		wantStge: "stage2",
		minConf:  0.8,
	},
}

// TestPromptEval runs every eval case through stages 1+2 and reports a
// quality scorecard. Individual failures are t.Error (not Fatal) so the
// full corpus is always evaluated.
func TestPromptEval(t *testing.T) {
	var results []evalResult
	pass, fail := 0, 0

	for _, tc := range promptEvalCases {
		t.Run(tc.name, func(t *testing.T) {
			r := evalOne(tc)
			results = append(results, r)
			if r.Pass {
				pass++
			} else {
				fail++
				t.Errorf("FAIL: %s", r.Details)
			}
		})
	}

	// Print scorecard summary.
	total := pass + fail
	pct := 0.0
	if total > 0 {
		pct = float64(pass) / float64(total) * 100
	}
	t.Logf("\n=== Prompt Eval Scorecard ===")
	t.Logf("Pass: %d / %d (%.1f%%)", pass, total, pct)
	t.Logf("Fail: %d / %d", fail, total)

	// Quality gate: require at least 60% pass rate.
	// Raise this as we improve.
	minPassRate := 0.60
	if total > 0 && float64(pass)/float64(total) < minPassRate {
		t.Errorf("quality gate: pass rate %.1f%% < minimum %.0f%%",
			pct, minPassRate*100)
	}
}

// evalOne evaluates a single case against stages 1 and 2.
func evalOne(tc evalCase) evalResult {
	r := evalResult{
		Name:   tc.name,
		Prompt: tc.prompt,
	}

	// Try Stage 1 (regex classifier).
	cmds := ClassifyPrompt(tc.prompt)
	if cmds != nil && len(cmds) > 0 {
		r.Stage = "stage1"
		return evalMatch(r, tc, cmds[0])
	}

	// Try Stage 2 (cross-domain NLP).
	cmds = ClassifyPromptCrossDomain(tc.prompt)
	if cmds != nil && len(cmds) > 0 {
		r.Stage = "stage2"
		return evalMatch(r, tc, cmds[0])
	}

	// Neither stage resolved.
	r.Stage = "none"
	r.Pass = false
	r.Details = fmt.Sprintf("prompt %q → no resolution (wanted %s %v)",
		tc.prompt, tc.wantCmd, tc.wantArgs)
	return r
}

// evalMatch checks whether a resolved command matches the expected output.
func evalMatch(r evalResult, tc evalCase, cmd ResolvedCommand) evalResult {
	var issues []string

	// Check stage expectation.
	if tc.wantStge != "either" && r.Stage != tc.wantStge {
		issues = append(issues, fmt.Sprintf(
			"resolved by %s, wanted %s", r.Stage, tc.wantStge,
		))
	}

	// Check command.
	if cmd.Cmd != tc.wantCmd {
		issues = append(issues, fmt.Sprintf(
			"cmd=%q, want %q", cmd.Cmd, tc.wantCmd,
		))
	}

	// Check args (subset match).
	for _, wantArg := range tc.wantArgs {
		found := false
		for _, gotArg := range cmd.Args {
			if gotArg == wantArg {
				found = true
				break
			}
		}
		if !found {
			issues = append(issues, fmt.Sprintf(
				"missing arg %q in %v", wantArg, cmd.Args,
			))
		}
	}

	// Check confidence.
	if cmd.Confidence < tc.minConf {
		issues = append(issues, fmt.Sprintf(
			"confidence=%.2f < min %.2f", cmd.Confidence, tc.minConf,
		))
	}

	if len(issues) > 0 {
		r.Pass = false
		r.Details = fmt.Sprintf("prompt %q → %s %v (conf=%.2f): %s",
			tc.prompt, cmd.Cmd, cmd.Args, cmd.Confidence,
			strings.Join(issues, "; "))
	} else {
		r.Pass = true
		r.Details = fmt.Sprintf("prompt %q → %s %v (conf=%.2f) via %s",
			tc.prompt, cmd.Cmd, cmd.Args, cmd.Confidence, r.Stage)
	}
	return r
}
