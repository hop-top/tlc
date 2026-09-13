package cli

import (
	"reflect"
	"testing"
)

// --- RouteNounToDomain ---

func TestRouteNounToDomain_ExactMatch(t *testing.T) {
	tests := []struct {
		noun       string
		wantDomain NounDomain
		wantConf   float64
	}{
		{"tasks", DomainTask, 1.0},
		{"task", DomainTask, 1.0},
		{"track", DomainTrack, 1.0},
		{"tracks", DomainTrack, 1.0},
		{"project", DomainProject, 1.0},
		{"projects", DomainProject, 1.0},
	}
	for _, tc := range tests {
		tokens := PromptTokens{Noun: tc.noun}
		got, conf := RouteNounToDomain(tokens)
		if got != tc.wantDomain {
			t.Errorf("RouteNounToDomain(%q): domain = %q, want %q", tc.noun, got, tc.wantDomain)
		}
		if conf != tc.wantConf {
			t.Errorf("RouteNounToDomain(%q): conf = %v, want %v", tc.noun, conf, tc.wantConf)
		}
	}
}

func TestRouteNounToDomain_FuzzyMatch(t *testing.T) {
	// "tack" is 1 edit from "task" — should fuzzy-match
	tokens := PromptTokens{Noun: "tack"}
	got, conf := RouteNounToDomain(tokens)
	if got != DomainTask {
		t.Errorf("RouteNounToDomain(tack): domain = %q, want %q", got, DomainTask)
	}
	if conf != 0.9 {
		t.Errorf("RouteNounToDomain(tack): conf = %v, want 0.9", conf)
	}
}

func TestRouteNounToDomain_NoMatch(t *testing.T) {
	tokens := PromptTokens{Noun: "zzz"}
	got, conf := RouteNounToDomain(tokens)
	if got != "" {
		t.Errorf("RouteNounToDomain(zzz): expected empty domain, got %q", got)
	}
	if conf != 0 {
		t.Errorf("RouteNounToDomain(zzz): expected conf=0, got %v", conf)
	}
}

func TestRouteNounToDomain_EmptyNoun(t *testing.T) {
	tokens := PromptTokens{}
	got, conf := RouteNounToDomain(tokens)
	if got != "" || conf != 0 {
		t.Errorf("RouteNounToDomain(empty): want (\"\",0), got (%q,%v)", got, conf)
	}
}

// --- Task domain ---

func TestBuildCommand_TaskList(t *testing.T) {
	tokens := PromptTokens{Verb: "list", Noun: "tasks"}
	cmds := BuildCommand(tokens, DomainTask, 1.0)
	if len(cmds) != 1 {
		t.Fatalf("want 1 cmd, got %d", len(cmds))
	}
	want := ResolvedCommand{Cmd: "task", Args: []string{"list"}, Confidence: 1.0}
	if !reflect.DeepEqual(cmds[0], want) {
		t.Errorf("got %+v, want %+v", cmds[0], want)
	}
}

func TestBuildCommand_TaskCreate(t *testing.T) {
	tokens := PromptTokens{Verb: "create", Noun: "task"}
	cmds := BuildCommand(tokens, DomainTask, 1.0)
	if len(cmds) != 1 {
		t.Fatalf("want 1 cmd, got %d", len(cmds))
	}
	if cmds[0].Args[0] != "create" {
		t.Errorf("expected subcommand 'create', got %q", cmds[0].Args[0])
	}
}

func TestBuildCommand_TaskComplete(t *testing.T) {
	tokens := PromptTokens{Verb: "complete", Noun: "task"}
	cmds := BuildCommand(tokens, DomainTask, 1.0)
	if len(cmds) != 1 {
		t.Fatalf("want 1 cmd, got %d", len(cmds))
	}
	if cmds[0].Args[0] != "complete" {
		t.Errorf("expected 'complete', got %q", cmds[0].Args[0])
	}
}

func TestBuildCommand_TaskDelete(t *testing.T) {
	tokens := PromptTokens{Verb: "delete", Noun: "task"}
	cmds := BuildCommand(tokens, DomainTask, 1.0)
	if len(cmds) != 1 {
		t.Fatalf("want 1 cmd, got %d", len(cmds))
	}
	if cmds[0].Args[0] != "delete" {
		t.Errorf("expected 'delete', got %q", cmds[0].Args[0])
	}
}

// --- T-0302: Aggregate query patterns ---

func TestBuildCommand_CountTasks(t *testing.T) {
	tokens := PromptTokens{Verb: "count", Noun: "tasks"}
	cmds := BuildCommand(tokens, DomainTask, 1.0)
	if len(cmds) != 1 {
		t.Fatalf("want 1 cmd, got %d", len(cmds))
	}
	args := cmds[0].Args
	if !containsArg(args, "--counters") {
		t.Errorf("expected --counters in args, got %v", args)
	}
}

func TestBuildCommand_TaskSummary(t *testing.T) {
	tokens := PromptTokens{Verb: "summary", Noun: "tasks"}
	cmds := BuildCommand(tokens, DomainTask, 1.0)
	if len(cmds) != 1 {
		t.Fatalf("want 1 cmd, got %d", len(cmds))
	}
	if !containsArg(cmds[0].Args, "--summary") {
		t.Errorf("expected --summary in args, got %v", cmds[0].Args)
	}
}

func TestClassifyPromptCrossDomain_HowManyTasksBlocked(t *testing.T) {
	cmds := ClassifyPromptCrossDomain("how many tasks are blocked")
	if len(cmds) == 0 {
		t.Fatal("expected at least one command")
	}
	args := cmds[0].Args
	if !containsArg(args, "--counters") {
		t.Errorf("expected --counters in args, got %v", args)
	}
	if !containsArg(args, "--blocked") {
		t.Errorf("expected --blocked in args, got %v", args)
	}
}

// --- T-0303: Track domain ---

func TestBuildCommand_TrackList(t *testing.T) {
	tokens := PromptTokens{Verb: "list", Noun: "tracks"}
	cmds := BuildCommand(tokens, DomainTrack, 1.0)
	if len(cmds) != 1 {
		t.Fatalf("want 1 cmd, got %d", len(cmds))
	}
	want := ResolvedCommand{Cmd: "track", Args: []string{"list"}, Confidence: 1.0}
	if !reflect.DeepEqual(cmds[0], want) {
		t.Errorf("got %+v, want %+v", cmds[0], want)
	}
}

func TestClassifyPromptCrossDomain_ActiveTracks(t *testing.T) {
	cmds := ClassifyPromptCrossDomain("active tracks")
	if len(cmds) == 0 {
		t.Fatal("expected result")
	}
	args := cmds[0].Args
	if cmds[0].Cmd != "track" {
		t.Errorf("expected cmd=track, got %q", cmds[0].Cmd)
	}
	if !containsSequence(args, "--status", "active") {
		t.Errorf("expected --status active in args, got %v", args)
	}
}

func TestClassifyPromptCrossDomain_BlockedTracks(t *testing.T) {
	cmds := ClassifyPromptCrossDomain("blocked tracks")
	if len(cmds) == 0 {
		t.Fatal("expected result")
	}
	// Track domain: --blocked not supported; maps to --state blocked.
	if !containsSequence(cmds[0].Args, "--state", "blocked") {
		t.Errorf("expected --state blocked, got %v", cmds[0].Args)
	}
}

func TestClassifyPromptCrossDomain_StaleTracks(t *testing.T) {
	cmds := ClassifyPromptCrossDomain("stale tracks")
	if len(cmds) == 0 {
		t.Fatal("expected result")
	}
	// Track domain: --stale not supported; maps to --state stale.
	if !containsSequence(cmds[0].Args, "--state", "stale") {
		t.Errorf("expected --state stale, got %v", cmds[0].Args)
	}
}

func TestClassifyPromptCrossDomain_CountActiveTracks(t *testing.T) {
	cmds := ClassifyPromptCrossDomain("count active tracks")
	if len(cmds) == 0 {
		t.Fatal("expected result")
	}
	args := cmds[0].Args
	if !containsSequence(args, "--status", "active") {
		t.Errorf("expected --status active, got %v", args)
	}
	// Track domain: --counters is task-only; must NOT be present.
	if containsArg(args, "--counters") {
		t.Errorf("--counters must not appear for track domain, got %v", args)
	}
}

func TestClassifyPromptCrossDomain_ListTracks(t *testing.T) {
	cmds := ClassifyPromptCrossDomain("list tracks")
	if len(cmds) == 0 {
		t.Fatal("expected result")
	}
	want := ResolvedCommand{Cmd: "track", Args: []string{"list"}, Confidence: 1.0}
	if !reflect.DeepEqual(cmds[0], want) {
		t.Errorf("got %+v, want %+v", cmds[0], want)
	}
}

// --- T-0305: Project domain ---

func TestClassifyPromptCrossDomain_ListProjects(t *testing.T) {
	cmds := ClassifyPromptCrossDomain("list projects")
	if len(cmds) == 0 {
		t.Fatal("expected result")
	}
	if cmds[0].Cmd != "project" || cmds[0].Args[0] != "list" {
		t.Errorf("expected project list, got %+v", cmds[0])
	}
}

func TestClassifyPromptCrossDomain_SwitchProject(t *testing.T) {
	// "project switch" does not exist; classifier must return nil and let LLM handle it.
	cmds := ClassifyPromptCrossDomain("switch to auth project")
	if cmds != nil {
		t.Errorf("expected nil for 'switch to auth project' (no project switch subcommand), got %+v", cmds)
	}
}

// --- Modifier flag mapping ---

func TestBuildCommand_ModifierStatusActive(t *testing.T) {
	tokens := PromptTokens{Verb: "list", Noun: "tasks", Modifiers: []string{"status:active"}}
	cmds := BuildCommand(tokens, DomainTask, 1.0)
	if len(cmds) == 0 {
		t.Fatal("expected result")
	}
	if !containsSequence(cmds[0].Args, "--status", "active") {
		t.Errorf("expected --status active in args, got %v", cmds[0].Args)
	}
}

func TestBuildCommand_ModifierStatusDone(t *testing.T) {
	tokens := PromptTokens{Verb: "list", Noun: "tasks", Modifiers: []string{"status:done"}}
	cmds := BuildCommand(tokens, DomainTask, 1.0)
	if len(cmds) == 0 {
		t.Fatal("expected result")
	}
	if !containsSequence(cmds[0].Args, "--status", "DONE") {
		t.Errorf("expected --status DONE in args, got %v", cmds[0].Args)
	}
}

func TestBuildCommand_ModifierBlocked(t *testing.T) {
	tokens := PromptTokens{Verb: "list", Noun: "tasks", Modifiers: []string{"blocked"}}
	cmds := BuildCommand(tokens, DomainTask, 1.0)
	if len(cmds) == 0 {
		t.Fatal("expected result")
	}
	if !containsArg(cmds[0].Args, "--blocked") {
		t.Errorf("expected --blocked, got %v", cmds[0].Args)
	}
}

func TestBuildCommand_ModifierStale(t *testing.T) {
	tokens := PromptTokens{Verb: "list", Noun: "tasks", Modifiers: []string{"stale"}}
	cmds := BuildCommand(tokens, DomainTask, 1.0)
	if len(cmds) == 0 {
		t.Fatal("expected result")
	}
	if !containsArg(cmds[0].Args, "--stale") {
		t.Errorf("expected --stale, got %v", cmds[0].Args)
	}
}

func TestBuildCommand_ModifierMine(t *testing.T) {
	tokens := PromptTokens{Verb: "list", Noun: "tasks", Modifiers: []string{"mine"}}
	cmds := BuildCommand(tokens, DomainTask, 1.0)
	if len(cmds) == 0 {
		t.Fatal("expected result")
	}
	if !containsArg(cmds[0].Args, "--mine") {
		t.Errorf("expected --mine, got %v", cmds[0].Args)
	}
}

// --- Confidence calculation ---

func TestBuildCommand_ConfidenceMultiplied(t *testing.T) {
	tokens := PromptTokens{Verb: "list", Noun: "tasks"}
	// domainConf = 0.9 (fuzzy noun), verbConf derived from exact verb
	cmds := BuildCommand(tokens, DomainTask, 0.9)
	if len(cmds) == 0 {
		t.Fatal("expected result")
	}
	// verb "list" is exact → verbConf=1.0; 1.0 * 0.9 = 0.9
	if cmds[0].Confidence != 0.9 {
		t.Errorf("expected confidence=0.9, got %v", cmds[0].Confidence)
	}
}

// --- Run verb guard ---

func TestClassifyPromptCrossDomain_RunTasksReturnsNil(t *testing.T) {
	// "run" is not a classifier verb; it must not resolve to any domain.
	cmds := ClassifyPromptCrossDomain("run tasks")
	if cmds != nil {
		t.Errorf("expected nil for 'run tasks', got %+v", cmds)
	}
}

func TestBuildCommand_UnknownVerbReturnsNil(t *testing.T) {
	tokens := PromptTokens{Verb: "zzz", Noun: "tasks"}
	cmds := BuildCommand(tokens, DomainTask, 1.0)
	if cmds != nil {
		t.Errorf("expected nil for unknown verb, got %+v", cmds)
	}
}

func TestBuildCommand_TrackStatusDoneModifier(t *testing.T) {
	tokens := PromptTokens{Verb: "list", Noun: "tracks", Modifiers: []string{"status:done"}}
	cmds := BuildCommand(tokens, DomainTrack, 1.0)
	if len(cmds) == 0 {
		t.Fatal("expected result")
	}
	if cmds[0].Cmd != "track" {
		t.Errorf("expected cmd=track, got %q", cmds[0].Cmd)
	}
	// Track uses "completed" not "DONE" for status:done.
	if !containsSequence(cmds[0].Args, "--status", "completed") {
		t.Errorf("expected --status completed in args, got %v", cmds[0].Args)
	}
}

// --- New modifier aliases + implicit verb ---

func TestClassifyPromptCrossDomain_IncompleteTasks(t *testing.T) {
	cmds := ClassifyPromptCrossDomain("incomplete tasks")
	if len(cmds) == 0 {
		t.Fatal("expected result for 'incomplete tasks'")
	}
	if cmds[0].Cmd != "task" {
		t.Errorf("expected cmd=task, got %q", cmds[0].Cmd)
	}
	if cmds[0].Args[0] != "list" {
		t.Errorf("expected subcommand=list, got %q", cmds[0].Args[0])
	}
}

func TestClassifyPromptCrossDomain_IncompletedTasks(t *testing.T) {
	cmds := ClassifyPromptCrossDomain("incompleted tasks")
	if len(cmds) == 0 {
		t.Fatal("expected result for 'incompleted tasks'")
	}
	if cmds[0].Cmd != "task" {
		t.Errorf("expected cmd=task, got %q", cmds[0].Cmd)
	}
}

func TestClassifyPromptCrossDomain_OpenTasks(t *testing.T) {
	cmds := ClassifyPromptCrossDomain("open tasks")
	if len(cmds) == 0 {
		t.Fatal("expected result for 'open tasks'")
	}
	if cmds[0].Cmd != "task" {
		t.Errorf("expected cmd=task, got %q", cmds[0].Cmd)
	}
}

func TestClassifyPromptCrossDomain_PendingTasks(t *testing.T) {
	cmds := ClassifyPromptCrossDomain("pending tasks")
	if len(cmds) == 0 {
		t.Fatal("expected result for 'pending tasks'")
	}
	if cmds[0].Cmd != "task" {
		t.Errorf("expected cmd=task, got %q", cmds[0].Cmd)
	}
}

func TestClassifyPromptCrossDomain_RemainingTasks(t *testing.T) {
	cmds := ClassifyPromptCrossDomain("remaining tasks")
	if len(cmds) == 0 {
		t.Fatal("expected result for 'remaining tasks'")
	}
	if cmds[0].Cmd != "task" {
		t.Errorf("expected cmd=task, got %q", cmds[0].Cmd)
	}
}

func TestClassifyPromptCrossDomain_UnfinishedTasks(t *testing.T) {
	cmds := ClassifyPromptCrossDomain("unfinished tasks")
	if len(cmds) == 0 {
		t.Fatal("expected result for 'unfinished tasks'")
	}
	if cmds[0].Cmd != "task" {
		t.Errorf("expected cmd=task, got %q", cmds[0].Cmd)
	}
}

func TestClassifyPromptCrossDomain_OverdueTasks(t *testing.T) {
	cmds := ClassifyPromptCrossDomain("overdue tasks")
	if len(cmds) == 0 {
		t.Fatal("expected result for 'overdue tasks'")
	}
	if cmds[0].Cmd != "task" {
		t.Errorf("expected cmd=task, got %q", cmds[0].Cmd)
	}
	if !containsArg(cmds[0].Args, "--stale") {
		t.Errorf("expected --stale in args, got %v", cmds[0].Args)
	}
}

func TestClassifyPromptCrossDomain_AverageAgeOfIncompletedTasks(t *testing.T) {
	cmds := ClassifyPromptCrossDomain("average age of incompleted tasks")
	if len(cmds) == 0 {
		t.Fatal("expected result for 'average age of incompleted tasks'")
	}
	if cmds[0].Cmd != "task" {
		t.Errorf("expected cmd=task, got %q", cmds[0].Cmd)
	}
	if cmds[0].Args[0] != "list" {
		t.Errorf("expected subcommand=list, got %q", cmds[0].Args[0])
	}
}

func TestClassifyPromptCrossDomain_NounOnlyDefaultsList(t *testing.T) {
	cmds := ClassifyPromptCrossDomain("tasks")
	if len(cmds) == 0 {
		t.Fatal("expected result for bare 'tasks'")
	}
	if cmds[0].Cmd != "task" {
		t.Errorf("expected cmd=task, got %q", cmds[0].Cmd)
	}
	if cmds[0].Args[0] != "list" {
		t.Errorf("expected subcommand=list, got %q", cmds[0].Args[0])
	}
}

// --- ClassifyPromptCrossDomain nil returns ---

func TestClassifyPromptCrossDomain_Empty(t *testing.T) {
	if ClassifyPromptCrossDomain("") != nil {
		t.Error("expected nil for empty prompt")
	}
}

func TestClassifyPromptCrossDomain_NoMatch(t *testing.T) {
	if ClassifyPromptCrossDomain("zzz qqq xxx") != nil {
		t.Error("expected nil for unrecognized prompt")
	}
}

// --- helpers ---

func containsArg(args []string, flag string) bool {
	for _, a := range args {
		if a == flag {
			return true
		}
	}
	return false
}

func containsSequence(args []string, a, b string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == a && args[i+1] == b {
			return true
		}
	}
	return false
}
