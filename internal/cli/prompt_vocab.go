package cli

// VerbCategory is the semantic class for a verb.
type VerbCategory string

const (
	VerbQuery    VerbCategory = "query"
	VerbCreate   VerbCategory = "create"
	VerbComplete VerbCategory = "complete"
	VerbDestroy  VerbCategory = "destroy"
)

// NounDomain is the CLI domain for a noun.
type NounDomain string

const (
	DomainTask    NounDomain = "task"
	DomainTrack   NounDomain = "track"
	DomainProject NounDomain = "project"
)

// ModifierFlag is the CLI flag/modifier for a modifier word.
type ModifierFlag string

// verbAliases maps raw verb strings to VerbCategory.
var verbAliases = map[string]VerbCategory{
	"list":  VerbQuery,
	"show":  VerbQuery,
	"count": VerbQuery,
	"find":  VerbQuery,

	"create": VerbCreate,
	"new":    VerbCreate,
	"add":    VerbCreate,

	"complete": VerbComplete,
	"finish":   VerbComplete,

	"delete": VerbDestroy,
	"remove": VerbDestroy,
	"drop":   VerbDestroy,
}

// nounAliases maps raw noun strings to NounDomain.
var nounAliases = map[string]NounDomain{
	"task":     DomainTask,
	"tasks":    DomainTask,
	"todo":     DomainTask,
	"todos":    DomainTask,
	"track":    DomainTrack,
	"tracks":   DomainTrack,
	"feature":  DomainTrack,
	"features": DomainTrack,
	"epic":     DomainTrack,
	"epics":    DomainTrack,
	"project":  DomainProject,
	"projects": DomainProject,
}

// modifierAliases maps raw modifier strings to ModifierFlag.
var modifierAliases = map[string]ModifierFlag{
	"active":      "status:active",
	"wip":         "status:active",
	"blocked":     "blocked",
	"stuck":       "blocked",
	"stale":       "stale",
	"old":         "stale",
	"my":          "mine",
	"mine":        "mine",
	"done":        "status:done",
	"completed":   "status:done",
	"incomplete":  "status:active",
	"incompleted": "status:active",
	"open":        "status:active",
	"pending":     "status:pending",
	"remaining":   "status:active",
	"unfinished":  "status:active",
	"overdue":     "stale",
}

// LookupVerb returns the VerbCategory for a raw verb string.
func LookupVerb(word string) (VerbCategory, bool) {
	v, ok := verbAliases[word]
	return v, ok
}

// LookupNoun returns the NounDomain for a raw noun string.
func LookupNoun(word string) (NounDomain, bool) {
	n, ok := nounAliases[word]
	return n, ok
}

// LookupModifier returns the ModifierFlag for a raw modifier string.
func LookupModifier(word string) (ModifierFlag, bool) {
	m, ok := modifierAliases[word]
	return m, ok
}
