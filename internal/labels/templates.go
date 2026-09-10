package labels

// ProjectType represents a categorized project type.
type ProjectType string

// Every constant here must have a case in GetTemplates that produces a
// domain set of its own. A type that falls through to `default` is worse
// than a type that does not exist: `label init --type <it>` prints the
// generic labels and reports the type back in its own header, so the
// user has no way to tell the choice was ignored. `go-socket` and
// `microservices` were declared here and reachable from neither
// GetTemplates, DetectProjectType nor `label templates`; they are gone
// rather than given invented vocabularies.
const (
	TypeGoBinary      ProjectType = "go-binary"
	TypePythonMVC     ProjectType = "python-mvc"
	TypeReactFrontend ProjectType = "react-frontend"
	TypeGeneric       ProjectType = "generic"
)

// AllProjectTypes is every type GetTemplates gives a domain set of its
// own, in the order the surfaces present them.
//
// It exists so `label templates` and the `--type` help text enumerate
// one list rather than two hand-maintained copies. The phantom types
// this replaces were exactly that failure: the constants, the switch,
// the help text and the templates listing each carried a different idea
// of which types existed.
func AllProjectTypes() []ProjectType {
	return []ProjectType{
		TypeGoBinary,
		TypeReactFrontend,
		TypePythonMVC,
		TypeGeneric,
	}
}

// Label represents a GitHub label.
type Label struct {
	Name        string
	Color       string
	Description string
}

// typeLabels is the `type:*` axis: the repo's own Conventional Commits
// vocabulary, one label per commit type, plus `type:breaking`.
//
// Every name carries the `dimension:value` shape on purpose. The sync
// plugins classify a remote label by its colon — github-sync's
// mapLabelsToTask drops any label without one, and the TLS parser's
// isMetaToken treats an unprefixed word as title text — so a bare `feat`
// is not a weaker label, it is a label that does not survive a round
// trip. `feat` and `fix` keep the colours they were seeded with before
// they gained the prefix, so an existing repo sees a rename rather than
// a palette churn.
//
// `type:breaking` has no Conventional Commits *type* of its own; it
// represents the `!` marker and the BREAKING CHANGE: trailer, which
// modify any type. It is on the axis because it is the one commit fact
// the vocabulary could not express at all.
var typeLabels = []Label{
	{Name: "type:feat", Color: "0052CC", Description: "New feature"},
	{Name: "type:fix", Color: "D73A4A", Description: "Bug fix"},
	{Name: "type:refactor", Color: "5319E7", Description: "Behaviour-preserving restructure"},
	{Name: "type:docs", Color: "0075CA", Description: "Documentation only"},
	{Name: "type:test", Color: "0E8A16", Description: "Tests only"},
	{Name: "type:chore", Color: "CFD3D7", Description: "Maintenance, no src or test change"},
	{Name: "type:perf", Color: "FF8C00", Description: "Performance improvement"},
	{Name: "type:build", Color: "8D6E63", Description: "Build system or dependencies"},
	{Name: "type:ci", Color: "1D76DB", Description: "CI configuration"},
	{Name: "type:style", Color: "D4C5F9", Description: "Formatting, no behaviour change"},
	{Name: "type:breaking", Color: "B60205", Description: "Breaking change (! or BREAKING CHANGE:)"},
}

// GetTemplates returns suggested labels for a project type.
//
// The shared axes are two different kinds of thing, and the split is the
// point of this file. `type:*` mirrors Conventional Commits — a spec, not
// a tlc config surface — so it is a literal above, readable against the
// spec it tracks. Priority, effort and status mirror vocabularies the
// user CONFIGURES, so they are generated from the effective config
// (see axes.go) and cannot drift from it, which is exactly what they had
// done: the old literals named P0-P3's aliases and a fixed
// TODO/IN_PROGRESS/DONE/SKIPPED regardless of what the user declared.
func GetTemplates(projectType ProjectType) []Label {
	generated := generatedAxes()
	common := make([]Label, 0, len(typeLabels)+len(generated))
	common = append(common, typeLabels...)
	common = append(common, generated...)

	var domains []Label
	switch projectType {
	case TypeGoBinary:
		domains = []Label{
			{Name: "domain:cli", Color: "BFD4F2", Description: "CLI"},
			{Name: "domain:core", Color: "0052CC", Description: "Core logic"},
			{Name: "domain:config", Color: "0E8A16", Description: "Config"},
			{Name: "domain:io", Color: "1D76DB", Description: "I/O"},
		}
	case TypeReactFrontend:
		domains = []Label{
			{Name: "domain:frontend", Color: "E99695", Description: "Frontend UI"},
			{Name: "domain:components", Color: "1D76DB", Description: "Components"},
			{Name: "domain:hooks", Color: "0052CC", Description: "Hooks"},
		}
	case TypePythonMVC:
		// The three MVC tiers plus migrations. Migrations earn a label
		// the other tiers do not: a schema change is the one change in
		// this project shape that is ordered, irreversible and reviewed
		// on different grounds than the code around it.
		//
		// `domain:controllers` is deliberately absent even though the
		// name says MVC. Django — the shape `manage.py` detects — calls
		// that tier views, and its `views.py` is where request handling
		// lives, so a `domain:views`/`domain:controllers` pair would
		// give a triager two labels for one file.
		domains = []Label{
			{Name: "domain:models", Color: "0052CC", Description: "Models and ORM"},
			{Name: "domain:views", Color: "E99695", Description: "Views and templates"},
			{Name: "domain:api", Color: "1D76DB", Description: "API layer"},
			{Name: "domain:migrations", Color: "D93F0B", Description: "Schema migrations"},
		}
	case TypeGeneric:
		domains = []Label{
			{Name: "domain:core", Color: "0052CC", Description: "Core logic"},
			{Name: "domain:api", Color: "1D76DB", Description: "API layer"},
		}
	default:
		// An unknown --type value. It reaches here as a ProjectType
		// because the flag is a free string, so the generic set is the
		// only honest answer.
		domains = []Label{
			{Name: "domain:core", Color: "0052CC", Description: "Core logic"},
			{Name: "domain:api", Color: "1D76DB", Description: "API layer"},
		}
	}

	return append(common, domains...)
}
