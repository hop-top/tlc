package cli

// VerbClass is the semantic class for a verb.
type VerbClass string

const (
	VerbQuery    VerbClass = "query"
	VerbCreate   VerbClass = "create"
	VerbComplete VerbClass = "complete"
	VerbDestroy  VerbClass = "destroy"
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

// verbVocab maps raw verb strings to VerbClass.
var verbVocab = map[string]VerbClass{
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

// nounVocab maps raw noun strings to NounDomain.
var nounVocab = map[string]NounDomain{
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

// modifierVocab maps raw modifier strings to ModifierFlag.
var modifierVocab = map[string]ModifierFlag{
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

// LookupVerb returns the VerbClass for a raw verb string.
func LookupVerb(word string) (VerbClass, bool) {
	v, ok := verbVocab[word]
	return v, ok
}

// LookupNoun returns the NounDomain for a raw noun string.
func LookupNoun(word string) (NounDomain, bool) {
	n, ok := nounVocab[word]
	return n, ok
}

// LookupModifier returns the ModifierFlag for a raw modifier string.
func LookupModifier(word string) (ModifierFlag, bool) {
	m, ok := modifierVocab[word]
	return m, ok
}
