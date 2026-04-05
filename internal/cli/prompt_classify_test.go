package cli

import (
	"testing"
)

func TestClassifyPrompt(t *testing.T) {
	tests := []struct {
		name     string
		prompt   string
		wantNil  bool
		wantCmd  string
		wantArgs []string
		wantConf float64
	}{
		// --- complete ---
		{
			name:     "complete with T-prefix",
			prompt:   "complete T-42",
			wantCmd:  "task",
			wantArgs: []string{"complete", "T-0042"},
			wantConf: 1.0,
		},
		{
			name:     "complete bare number",
			prompt:   "complete 42",
			wantCmd:  "task",
			wantArgs: []string{"complete", "T-0042"},
			wantConf: 1.0,
		},
		{
			name:     "complete already canonical",
			prompt:   "complete T-0042",
			wantCmd:  "task",
			wantArgs: []string{"complete", "T-0042"},
			wantConf: 1.0,
		},
		{
			name:     "finish synonym",
			prompt:   "finish T-42",
			wantCmd:  "task",
			wantArgs: []string{"complete", "T-0042"},
			wantConf: 1.0,
		},
		{
			name:     "mark done",
			prompt:   "mark T-42 done",
			wantCmd:  "task",
			wantArgs: []string{"complete", "T-0042"},
			wantConf: 1.0,
		},
		{
			name:     "mark as done",
			prompt:   "mark T-42 as done",
			wantCmd:  "task",
			wantArgs: []string{"complete", "T-0042"},
			wantConf: 1.0,
		},
		{
			name:     "mark task done",
			prompt:   "mark task 99 done",
			wantCmd:  "task",
			wantArgs: []string{"complete", "T-0099"},
			wantConf: 1.0,
		},

		// --- show ---
		{
			name:     "show task",
			prompt:   "show T-42",
			wantCmd:  "task",
			wantArgs: []string{"show", "T-0042"},
			wantConf: 1.0,
		},
		{
			name:     "view task",
			prompt:   "view T-0042",
			wantCmd:  "task",
			wantArgs: []string{"show", "T-0042"},
			wantConf: 1.0,
		},
		{
			name:     "details of task",
			prompt:   "details of T-7",
			wantCmd:  "task",
			wantArgs: []string{"show", "T-0007"},
			wantConf: 1.0,
		},
		{
			name:     "describe task",
			prompt:   "describe T-42",
			wantCmd:  "task",
			wantArgs: []string{"show", "T-0042"},
			wantConf: 1.0,
		},

		// --- delete ---
		{
			name:     "delete task",
			prompt:   "delete T-42",
			wantCmd:  "task",
			wantArgs: []string{"delete", "T-0042"},
			wantConf: 1.0,
		},
		{
			name:     "remove task",
			prompt:   "remove T-42",
			wantCmd:  "task",
			wantArgs: []string{"delete", "T-0042"},
			wantConf: 1.0,
		},

		// --- claim ---
		{
			name:     "claim task",
			prompt:   "claim T-42",
			wantCmd:  "task",
			wantArgs: []string{"claim", "T-0042"},
			wantConf: 1.0,
		},
		{
			name:     "take task",
			prompt:   "take T-42",
			wantCmd:  "task",
			wantArgs: []string{"claim", "T-0042"},
			wantConf: 1.0,
		},
		{
			name:     "grab task",
			prompt:   "grab 42",
			wantCmd:  "task",
			wantArgs: []string{"claim", "T-0042"},
			wantConf: 1.0,
		},

		// --- unclaim ---
		{
			name:     "unclaim task",
			prompt:   "unclaim T-42",
			wantCmd:  "task",
			wantArgs: []string{"unclaim", "T-0042"},
			wantConf: 1.0,
		},
		{
			name:     "release task",
			prompt:   "release T-42",
			wantCmd:  "task",
			wantArgs: []string{"unclaim", "T-0042"},
			wantConf: 1.0,
		},
		{
			name:     "drop task",
			prompt:   "drop T-42",
			wantCmd:  "task",
			wantArgs: []string{"unclaim", "T-0042"},
			wantConf: 1.0,
		},

		// --- assign ---
		{
			name:     "assign task to user",
			prompt:   "assign T-42 to jadb",
			wantCmd:  "task",
			wantArgs: []string{"assign", "jadb", "T-0042"},
			wantConf: 1.0,
		},
		{
			name:     "assign user to task",
			prompt:   "assign jadb to T-42",
			wantCmd:  "task",
			wantArgs: []string{"assign", "jadb", "T-0042"},
			wantConf: 1.0,
		},

		// --- create ---
		{
			name:     "create task with called",
			prompt:   "create a task called Login flow",
			wantCmd:  "task",
			wantArgs: []string{"create", "Login flow"},
			wantConf: 1.0,
		},
		{
			name:     "create task simple",
			prompt:   "create task Login flow",
			wantCmd:  "task",
			wantArgs: []string{"create", "Login flow"},
			wantConf: 1.0,
		},
		{
			name:     "new task",
			prompt:   "new task Refactor auth module",
			wantCmd:  "task",
			wantArgs: []string{"create", "Refactor auth module"},
			wantConf: 1.0,
		},
		{
			name:     "create task named",
			prompt:   "create a task named Deploy pipeline",
			wantCmd:  "task",
			wantArgs: []string{"create", "Deploy pipeline"},
			wantConf: 1.0,
		},

		// --- list blocked ---
		{
			name:     "what tasks are blocked",
			prompt:   "what tasks are blocked?",
			wantCmd:  "task",
			wantArgs: []string{"list", "--blocked"},
			wantConf: 1.0,
		},
		{
			name:     "blocked tasks",
			prompt:   "blocked tasks",
			wantCmd:  "task",
			wantArgs: []string{"list", "--blocked"},
			wantConf: 1.0,
		},
		{
			name:     "show blocked tasks",
			prompt:   "show blocked tasks",
			wantCmd:  "task",
			wantArgs: []string{"list", "--blocked"},
			wantConf: 1.0,
		},
		{
			name:     "list blocked",
			prompt:   "list blocked",
			wantCmd:  "task",
			wantArgs: []string{"list", "--blocked"},
			wantConf: 1.0,
		},

		// --- list mine ---
		{
			name:     "list my tasks",
			prompt:   "list my tasks",
			wantCmd:  "task",
			wantArgs: []string{"list", "--mine"},
			wantConf: 1.0,
		},
		{
			name:     "my tasks",
			prompt:   "my tasks",
			wantCmd:  "task",
			wantArgs: []string{"list", "--mine"},
			wantConf: 1.0,
		},
		{
			name:     "show my tasks",
			prompt:   "show my tasks",
			wantCmd:  "task",
			wantArgs: []string{"list", "--mine"},
			wantConf: 1.0,
		},

		// --- list all ---
		{
			name:     "list tasks",
			prompt:   "list tasks",
			wantCmd:  "task",
			wantArgs: []string{"list"},
			wantConf: 1.0,
		},
		{
			name:     "show tasks",
			prompt:   "show tasks",
			wantCmd:  "task",
			wantArgs: []string{"list"},
			wantConf: 1.0,
		},
		{
			name:     "all tasks",
			prompt:   "all tasks",
			wantCmd:  "task",
			wantArgs: []string{"list"},
			wantConf: 1.0,
		},

		// --- case insensitivity ---
		{
			name:     "uppercase COMPLETE",
			prompt:   "COMPLETE T-42",
			wantCmd:  "task",
			wantArgs: []string{"complete", "T-0042"},
			wantConf: 1.0,
		},
		{
			name:     "mixed case Show",
			prompt:   "Show T-42",
			wantCmd:  "task",
			wantArgs: []string{"show", "T-0042"},
			wantConf: 1.0,
		},
		{
			name:     "mixed case Claim",
			prompt:   "CLAIM t-42",
			wantCmd:  "task",
			wantArgs: []string{"claim", "T-0042"},
			wantConf: 1.0,
		},

		// --- extra whitespace ---
		{
			name:     "leading/trailing whitespace",
			prompt:   "  complete T-42  ",
			wantCmd:  "task",
			wantArgs: []string{"complete", "T-0042"},
			wantConf: 1.0,
		},
		{
			name:     "extra internal spaces",
			prompt:   "complete   T-42",
			wantCmd:  "task",
			wantArgs: []string{"complete", "T-0042"},
			wantConf: 1.0,
		},

		// --- task ID formats ---
		{
			name:     "five digit task ID",
			prompt:   "show T-10042",
			wantCmd:  "task",
			wantArgs: []string{"show", "T-10042"},
			wantConf: 1.0,
		},
		{
			name:     "single digit bare",
			prompt:   "show 1",
			wantCmd:  "task",
			wantArgs: []string{"show", "T-0001"},
			wantConf: 1.0,
		},

		// --- no match ---
		{
			name:    "unknown command",
			prompt:  "what is the weather?",
			wantNil: true,
		},
		{
			name:    "empty string",
			prompt:  "",
			wantNil: true,
		},
		{
			name:    "whitespace only",
			prompt:  "   ",
			wantNil: true,
		},
		{
			name:    "random sentence",
			prompt:  "the quick brown fox jumps over the lazy dog",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyPrompt(tt.prompt)

			if tt.wantNil {
				if got != nil {
					t.Fatalf("ClassifyPrompt(%q) = %+v; want nil", tt.prompt, got)
				}
				return
			}

			if got == nil {
				t.Fatalf("ClassifyPrompt(%q) = nil; want match", tt.prompt)
			}
			if len(got) == 0 {
				t.Fatalf("ClassifyPrompt(%q) returned empty slice; want match", tt.prompt)
			}

			cmd := got[0]
			if cmd.Cmd != tt.wantCmd {
				t.Errorf("Cmd = %q; want %q", cmd.Cmd, tt.wantCmd)
			}
			if cmd.Confidence != tt.wantConf {
				t.Errorf("Confidence = %f; want %f", cmd.Confidence, tt.wantConf)
			}
			if len(cmd.Args) != len(tt.wantArgs) {
				t.Fatalf("Args = %v (len %d); want %v (len %d)",
					cmd.Args, len(cmd.Args), tt.wantArgs, len(tt.wantArgs))
			}
			for i, a := range cmd.Args {
				if a != tt.wantArgs[i] {
					t.Errorf("Args[%d] = %q; want %q", i, a, tt.wantArgs[i])
				}
			}
		})
	}
}
