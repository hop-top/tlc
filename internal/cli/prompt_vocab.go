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
	DomainFlow    NounDomain = "flow"
	DomainProject NounDomain = "project"
)

// ModifierFlag is the CLI flag/modifier for a modifier word.
type ModifierFlag string

// VerbAliases maps raw verb strings to VerbCategory.
var VerbAliases = map[string]VerbCategory{
	"list":  VerbQuery,
	"show":  VerbQuery,
	"count": VerbQuery,
	"find":  VerbQuery,

	"create": VerbCreate,
	"new":    VerbCreate,
	"add":    VerbCreate,

	"complete": VerbComplete,
	"finish":   VerbComplete,
	"done":     VerbComplete,

	"delete": VerbDestroy,
	"remove": VerbDestroy,
	"drop":   VerbDestroy,
}

// NounAliases maps raw noun strings to NounDomain.
var NounAliases = map[string]NounDomain{
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
	"flow":     DomainFlow,
	"flows":    DomainFlow,
	"pipeline": DomainFlow,
	"project":  DomainProject,
	"projects": DomainProject,
}

// ModifierAliases maps raw modifier strings to ModifierFlag.
var ModifierAliases = map[string]ModifierFlag{
	"active":    "status:active",
	"wip":       "status:active",
	"blocked":   "blocked",
	"stuck":     "blocked",
	"stale":     "stale",
	"old":       "stale",
	"my":        "mine",
	"mine":      "mine",
	"done":      "status:done",
	"completed": "status:done",
}

// LookupVerb returns the VerbCategory for a raw verb string.
func LookupVerb(word string) (VerbCategory, bool) {
	v, ok := VerbAliases[word]
	return v, ok
}

// LookupNoun returns the NounDomain for a raw noun string.
func LookupNoun(word string) (NounDomain, bool) {
	n, ok := NounAliases[word]
	return n, ok
}

// LookupModifier returns the ModifierFlag for a raw modifier string.
func LookupModifier(word string) (ModifierFlag, bool) {
	m, ok := ModifierAliases[word]
	return m, ok
}
