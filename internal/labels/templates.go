package labels

// ProjectType represents a categorized project type.
type ProjectType string

const (
	TypeGoBinary      ProjectType = "go-binary"
	TypeGoSocket      ProjectType = "go-socket"
	TypePythonMVC     ProjectType = "python-mvc"
	TypeReactFrontend ProjectType = "react-frontend"
	TypeMicroservices ProjectType = "microservices"
	TypeGeneric       ProjectType = "generic"
)

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
	default:
		domains = []Label{
			{Name: "domain:core", Color: "0052CC", Description: "Core logic"},
			{Name: "domain:api", Color: "1D76DB", Description: "API layer"},
		}
	}

	return append(common, domains...)
}
