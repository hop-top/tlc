package core

import (
	"time"

	"hop.top/tlc/internal/config"
)

type TaskStatus string

const (
	StatusTodo       TaskStatus = "TODO"
	StatusInProgress TaskStatus = "IN_PROGRESS"
	StatusDone       TaskStatus = "DONE"
	StatusSkipped    TaskStatus = "SKIPPED"
)

// taskStatuses is the BUILT-IN set of task statuses in lifecycle order,
// used when the user's config declares no `task.statuses` of its own.
//
// It is no longer the whole story: `task.statuses` is a documented,
// validated config surface, and a user who declares IN_REVIEW there means
// it for the CLI too, not only for the workflow engine. Consumers that
// render or accept a status vocabulary — validation messages, the flag
// enums, fuzzy normalisation, shell completion — read
// ConfiguredTaskStatusStrings, which falls back to this slice. This slice
// remains the fallback and the compile-time home of the Status* constants
// the code refers to by name.
var taskStatuses = []TaskStatus{
	StatusTodo,
	StatusInProgress,
	StatusDone,
	StatusSkipped,
}

// TaskStatuses returns the closed set of task statuses in lifecycle order.
// The returned slice is a copy; mutating it does not affect the canon.
func TaskStatuses() []TaskStatus {
	return append([]TaskStatus(nil), taskStatuses...)
}

// TaskStatusStrings returns TaskStatuses as plain strings, for consumers
// that render or register the set (error messages, flag enums, completion).
func TaskStatusStrings() []string {
	return enumStrings(taskStatuses)
}

// ConfiguredInitialTaskStatus returns the status a new task lands in when
// the caller nominates none: `task.default_status` when it names a
// declared status, else the status carrying role "initial".
//
// Resolved lazily on every call, like ConfiguredTaskStatusStrings and for
// the same reason — but here the caching matters more than convention.
// DefaultWorkflowE memoises its answer in a sync.Once, so calling it from
// any code path that runs BEFORE argv is parsed (help rendering, flag
// usage) would pin the process to whatever config existed at that moment
// and silently discard later `-c key=value` overrides. Reading the
// provider directly keeps those paths override-safe.
//
// Returns "" when no status can be resolved; callers decide whether that
// is an error or simply a help string they leave generic.
func ConfiguredInitialTaskStatus() string {
	cfg := resolveTaskConfig()
	if cfg == nil {
		return ""
	}
	statuses := cfg.Statuses
	if len(statuses) == 0 {
		statuses = config.GetDefaultStatuses()
	}
	if cfg.DefaultStatus != "" {
		for _, s := range statuses {
			if s.Name == cfg.DefaultStatus {
				return s.Name
			}
		}
	}
	for _, s := range statuses {
		if s.Role == config.RoleInitial {
			return s.Name
		}
	}
	return ""
}

// ConfiguredTaskStatusStrings returns the effective task-status vocabulary:
// the names declared in the user's `task.statuses`, in declared order, or
// the built-in set when config declares none.
//
// Resolved lazily on every call rather than cached in a package-level var,
// because config is read long after package init: a var initialised at
// init time would pin the built-ins forever. It reads through the same
// taskConfigProvider hook DefaultWorkflow uses, so the vocabulary the CLI
// accepts and the vocabulary the workflow enforces cannot disagree —
// internal/core stays free of any dependency on viper or internal/cli.
func ConfiguredTaskStatusStrings() []string {
	cfg := resolveTaskConfig()
	if cfg == nil || len(cfg.Statuses) == 0 {
		return TaskStatusStrings()
	}
	out := make([]string, 0, len(cfg.Statuses))
	for _, s := range cfg.Statuses {
		if s.Name != "" {
			out = append(out, s.Name)
		}
	}
	if len(out) == 0 {
		return TaskStatusStrings()
	}
	return out
}

// ConfiguredTaskStatusDefinitions returns the effective task statuses as
// full definitions — name, label, description, colour, role, terminality —
// rather than bare names.
//
// ConfiguredTaskStatusStrings answers "which names are legal"; this
// answers "what does each one MEAN", which is what a consumer needs when
// it must decide per status rather than merely validate one. The label
// templates are the first such consumer: they pick which statuses deserve
// a `status:*` label from role and is_terminal, and take the swatch from
// the configured colour, so neither the selection nor the palette can
// drift from config the way a retyped list does.
//
// Resolved lazily through the provider, NOT through DefaultWorkflow*, for
// the reason spelled out on ConfiguredPriorityStrings: the memoising
// singleton would freeze config before `-c key=value` overrides merge if
// any pre-argv path (help, usage, completion) ever reached it.
func ConfiguredTaskStatusDefinitions() []config.StatusDefinition {
	cfg := resolveTaskConfig()
	if cfg == nil || len(cfg.Statuses) == 0 {
		return config.GetDefaultStatuses()
	}
	out := make([]config.StatusDefinition, 0, len(cfg.Statuses))
	for _, s := range cfg.Statuses {
		if s.Name != "" {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return config.GetDefaultStatuses()
	}
	return out
}

// ConfiguredPriorityDefinitions returns the effective priority vocabulary
// as full definitions, in declared order — which IS rank order, most
// urgent first (see config.PriorityDefinition).
//
// The definition-level counterpart to ConfiguredPriorityStrings, and the
// same contract: callers must not sort the result, because sorting it
// would destroy the only expression of rank the schema has.
func ConfiguredPriorityDefinitions() []config.PriorityDefinition {
	cfg := resolveTaskConfig()
	if cfg == nil || len(cfg.Priorities) == 0 {
		return config.GetDefaultPriorities()
	}
	out := make([]config.PriorityDefinition, 0, len(cfg.Priorities))
	for _, p := range cfg.Priorities {
		if p.Name != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return config.GetDefaultPriorities()
	}
	return out
}

// ValidTaskStatus reports whether s is a recognized task status (or empty).
//
// "Recognized" means the EFFECTIVE vocabulary, not the built-in set: a
// user who declares IN_REVIEW in `task.statuses` means it here too. It
// reads ConfiguredTaskStatusStrings for that, so this helper and the flag
// enums, fuzzy normalisation and completion cannot disagree about which
// names are legal. Validating against the built-in taskStatuses slice
// instead would silently reject every configured status.
//
// Resolved through the provider rather than the memoising DefaultWorkflow*
// singleton, for the reason spelled out on ConfiguredPriorityStrings: a
// pre-argv caller (help rendering, flag usage) would otherwise freeze
// config before `-c key=value` overrides have merged.
//
// Empty is valid, matching the pre-existing contract and ValidPriority:
// callers use "" to mean "no status nominated", and the empty case is
// checked before the vocabulary rather than folded into it.
func ValidTaskStatus(s TaskStatus) bool {
	if s == "" {
		return true
	}
	for _, v := range ConfiguredTaskStatusStrings() {
		if string(s) == v {
			return true
		}
	}
	return false
}

// Effort represents task size estimate: XS, S, M, L, XL.
type Effort string

const (
	EffortXS Effort = "XS"
	EffortS  Effort = "S"
	EffortM  Effort = "M"
	EffortL  Effort = "L"
	EffortXL Effort = "XL"
)

// efforts is the closed set of effort values in ascending size order.
// Same contract as taskStatuses: one declaration, every consumer reads it.
var efforts = []Effort{EffortXS, EffortS, EffortM, EffortL, EffortXL}

// Efforts returns the closed set of effort values in ascending size order.
// The returned slice is a copy.
func Efforts() []Effort {
	return append([]Effort(nil), efforts...)
}

// EffortStrings returns Efforts as plain strings.
func EffortStrings() []string {
	return enumStrings(efforts)
}

// ValidEffort returns true if e is a recognised effort value (or empty).
func ValidEffort(e Effort) bool {
	if e == "" {
		return true
	}
	for _, v := range efforts {
		if e == v {
			return true
		}
	}
	return false
}

// Priority represents task urgency level: P0 (critical) through P3 (low).
type Priority string

const (
	PriorityP0 Priority = "P0"
	PriorityP1 Priority = "P1"
	PriorityP2 Priority = "P2"
	PriorityP3 Priority = "P3"
)

// priorities is the BUILT-IN set of priorities in descending urgency
// order, used when the user's config declares no `task.priorities`.
//
// Same contract, and the same caveat, as taskStatuses: it is the
// fallback and the compile-time home of the Priority* constants, not the
// whole story. Consumers that render or accept a priority vocabulary —
// validation messages, the flag enums, fuzzy normalisation, shell
// completion, priority-ordered sorting — read
// ConfiguredPriorityStrings, which falls back to this slice.
var priorities = []Priority{PriorityP0, PriorityP1, PriorityP2, PriorityP3}

// Priorities returns the closed set of priorities in descending urgency
// order. The returned slice is a copy.
func Priorities() []Priority {
	return append([]Priority(nil), priorities...)
}

// PriorityStrings returns Priorities as plain strings.
func PriorityStrings() []string {
	return enumStrings(priorities)
}

// ConfiguredPriorityStrings returns the effective priority vocabulary:
// the names declared in the user's `task.priorities`, in declared order
// (most urgent first), or the built-in set when config declares none.
//
// Declaration order is rank order — see config.PriorityDefinition — so
// the returned slice is also the sort key for priority-ordered listing,
// and callers must not sort it.
//
// Resolved lazily on every call rather than cached in a package-level
// var, for the same two reasons ConfiguredTaskStatusStrings is: a var
// initialised at package-init time predates any config file and could
// only ever hold the built-ins, and reading the provider directly rather
// than through the memoising DefaultWorkflow* singleton keeps pre-argv
// callers (help rendering, flag usage) from freezing config before
// `-c key=value` overrides have merged.
func ConfiguredPriorityStrings() []string {
	cfg := resolveTaskConfig()
	if cfg == nil || len(cfg.Priorities) == 0 {
		return PriorityStrings()
	}
	out := make([]string, 0, len(cfg.Priorities))
	for _, p := range cfg.Priorities {
		if p.Name != "" {
			out = append(out, p.Name)
		}
	}
	if len(out) == 0 {
		return PriorityStrings()
	}
	return out
}

// PriorityRank returns the ordinal of p within the effective priority
// vocabulary — 0 for the most urgent — and whether p is in it.
//
// This is what makes "sort by priority" mean urgency rather than
// alphabet. With the built-in P0..P3 the two coincide by accident:
// lexicographic order over "P0".."P3" happens to be rank order, which is
// why nothing needed this before. A vocabulary of
// URGENT/NORMAL/LATER sorts to LATER, NORMAL, URGENT lexicographically —
// exactly backwards.
//
// The empty priority is not in any vocabulary and gets ok=false.
// Callers order it last: "no priority set" is not the same fact as "the
// least urgent priority", and a task the user never triaged must not
// outrank one they deliberately marked lowest.
func PriorityRank(p Priority) (int, bool) {
	if p == "" {
		return 0, false
	}
	for i, name := range ConfiguredPriorityStrings() {
		if string(p) == name {
			return i, true
		}
	}
	return 0, false
}

// ValidPriority returns true if p is a recognised priority value (or empty).
//
// Empty is valid: priority is optional, unlike status. Every caller
// depends on that — it is how a task created without -p passes
// validation — so the empty case is checked before the vocabulary, not
// folded into it.
func ValidPriority(p Priority) bool {
	if p == "" {
		return true
	}
	for _, v := range ConfiguredPriorityStrings() {
		if string(p) == v {
			return true
		}
	}
	return false
}

// enumStrings renders a canonical enum slice as plain strings, preserving
// declaration order. One helper so every enum's string form is derived the
// same way rather than re-typed beside its constants.
func enumStrings[T ~string](values []T) []string {
	out := make([]string, len(values))
	for i, v := range values {
		out[i] = string(v)
	}
	return out
}

type Task struct {
	ID            string                 `json:"id" yaml:"id" table:"ID"`
	Seq           int64                  `json:"seq" yaml:"seq"`
	Title         string                 `json:"title" yaml:"title" table:"Title"`
	Description   string                 `json:"description,omitempty" yaml:"description,omitempty"`
	Status        TaskStatus             `json:"status" yaml:"status" table:"Status"`
	AssignedTo    *string                `json:"assigned_to" yaml:"assigned_to" table:"Assigned"`
	Tags          []string               `json:"tags,omitempty" yaml:"tags,omitempty"`
	Reference     string                 `json:"reference" yaml:"reference"`
	Effort        Effort                 `json:"effort,omitempty" yaml:"effort,omitempty" table:"Effort"`
	Priority      Priority               `json:"priority,omitempty" yaml:"priority,omitempty" table:"Priority"`
	CreatedAt     time.Time              `json:"created_at" yaml:"created_at"`
	UpdatedAt     time.Time              `json:"updated_at" yaml:"updated_at"`
	OriginSystem  *string                `json:"origin_system,omitempty" yaml:"origin_system,omitempty"`
	LastSyncAt    *time.Time             `json:"last_sync_at,omitempty" yaml:"last_sync_at,omitempty"`
	Archived      bool                   `json:"archived" yaml:"archived"`
	ProjectID     *string                `json:"project_id,omitempty" yaml:"project_id,omitempty"`
	Meta          map[string]interface{} `json:"meta,omitempty" yaml:"meta,omitempty"`
	StaleTimeout  *time.Duration         `json:"stale_timeout,omitempty" yaml:"stale_timeout,omitempty"`
	BlockedReason *string                `json:"blocked_reason,omitempty" yaml:"blocked_reason,omitempty"`
	StaleFiredAt  *time.Time             `json:"stale_fired_at,omitempty" yaml:"stale_fired_at,omitempty"`
	TrackID       *string                `json:"track_id,omitempty" yaml:"track_id,omitempty" table:"Track"`
	DueAt         *time.Time             `json:"due_at,omitempty" yaml:"due_at,omitempty"`
	RemindAt      *time.Time             `json:"remind_at,omitempty" yaml:"remind_at,omitempty"`
	RRule         string                 `json:"rrule,omitempty" yaml:"rrule,omitempty"`
	NoAutoRemind  bool                   `json:"no_auto_remind,omitempty" yaml:"no_auto_remind,omitempty"`
}

type RegisteredProject struct {
	ProjectID    string    `json:"project_id"`
	DBPath       string    `json:"db_path"`
	SpaceURI     string    `json:"space_uri,omitempty"`
	Label        string    `json:"label,omitempty"`
	RegisteredAt time.Time `json:"registered_at"`
	LastSeenAt   time.Time `json:"last_seen_at"`
	Status       string    `json:"status"`
}

type LogEntry struct {
	ID        int64                  `json:"id,omitempty" yaml:"id,omitempty"`
	TaskID    string                 `json:"task_id" yaml:"task_id"`
	Timestamp time.Time              `json:"timestamp" yaml:"timestamp"`
	By        string                 `json:"by" yaml:"by"`
	Action    string                 `json:"action" yaml:"action"`
	Note      string                 `json:"note" yaml:"note"`
	Meta      map[string]interface{} `json:"meta,omitempty" yaml:"meta,omitempty"`
}
