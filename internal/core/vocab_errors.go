package core

import "strings"

// The allowed-set rendering for the three configurable vocabularies lives
// here, next to ConfiguredTaskStatusStrings/ConfiguredPriorityStrings/
// ConfiguredEffortStrings, rather than beside any one consumer.
//
// internal/cli/fieldnorm.go already collapsed the CLI's rejection messages
// onto one formatter precisely so the prose and the flag enum could not
// drift. internal/inbox is a second, independent gate on the same
// vocabularies and cannot import internal/cli (the dependency runs
// cli → inbox → core), so a formatter reachable from both has to sit at
// or below internal/core. Retyping the set in the inbox instead is what
// produced the defect these helpers close: a literal "P0, P1, P2, P3, P4"
// that named a priority tlc has never had.
//
// Every helper reads the Configured* accessors, never the memoising
// DefaultWorkflow* singleton: a caller running before argv is parsed
// would otherwise pin config in a sync.Once and silently discard later
// `-c key=value` overrides for the rest of the process.

// VocabularyList renders a vocabulary the way every rejection message and
// the flag-enum help suffix spell it: comma-separated, declaration order.
func VocabularyList(values []string) string {
	return strings.Join(values, ", ")
}

// TaskStatusVocabularyList renders the effective task-status vocabulary
// for an error message or help string.
func TaskStatusVocabularyList() string {
	return VocabularyList(ConfiguredTaskStatusStrings())
}

// PriorityVocabularyList renders the effective priority vocabulary for an
// error message or help string.
func PriorityVocabularyList() string {
	return VocabularyList(ConfiguredPriorityStrings())
}

// EffortVocabularyList renders the effective effort vocabulary for an
// error message or help string.
func EffortVocabularyList() string {
	return VocabularyList(ConfiguredEffortStrings())
}
