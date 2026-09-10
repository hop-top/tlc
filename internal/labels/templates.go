package labels

import (
	"sort"
	"strings"
)

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
	TypeNodeBackend   ProjectType = "node-backend"
	TypePythonMVC     ProjectType = "python-mvc"
	TypeReactFrontend ProjectType = "react-frontend"
	TypeLibrary       ProjectType = "library"
	TypeMonorepo      ProjectType = "monorepo"
	TypeInfra         ProjectType = "infra"
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
		TypeNodeBackend,
		TypeReactFrontend,
		TypePythonMVC,
		TypeLibrary,
		TypeMonorepo,
		TypeInfra,
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
// trip. `feat` and `fix` keep the colors they were seeded with before
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
	{Name: "type:style", Color: "D4C5F9", Description: "Formatting, no behavior change"},
	{Name: "type:breaking", Color: "B60205", Description: "Breaking change (! or BREAKING CHANGE:)"},
}

// needsLabels is the `needs:*` axis: what the task is waiting on from a
// PERSON, as opposed to what it is waiting on from another task.
//
// It fills a gap `status:*` leaves open by design. `status:blocked` is
// generated from Task.BlockedReason, which tlc populates from task
// dependencies — it means blocked-by-task, and the reason string names
// the tasks. Nothing on that axis can say "this is open because a human
// has not answered yet", and the three values here are the three shapes
// that takes: nobody has looked at it (`triage`), somebody looked and
// cannot reproduce the report (`repro`), or the work is understood and
// waiting on a call somebody has to make (`decision`).
//
// It lives here rather than in axes.go because axes.go generates the
// axes that MIRROR a configured vocabulary — statuses, priorities,
// efforts all exist in config and would drift if retyped. `needs:*`
// mirrors no config surface, so there is nothing to derive it from;
// generating it would mean inventing a vocabulary and then reading it
// back. That makes it the same kind of thing as `type:*`: a literal,
// readable against the concept it tracks.
//
// It is on `common` rather than in a switch case because the question it
// answers is not project-shaped. A Terraform module and a React app both
// have issues nobody has triaged.
//
// The values round-trip: github-sync's mapLabelsToTask sends any
// unrecognized `dimension:value` label to task tags, so a `needs:*`
// label pulled from a forge survives as a tag rather than being dropped.
//
// The swatches moved off the priority axis's. `needs:repro` shared
// D93F0B with priority:high and status:blocked, and `needs:triage`
// shared FBCA04 with priority:medium — and `needs:*` is read on the same
// issue as both, so each pair rendered as one badge. Priority and effort
// could not give way (their colors are pinned; `sync push` re-colors
// live issues), so `needs:*` moved: triage to a darker gold, repro to a
// muted brown that reads as a question rather than as an alarm, which is
// what "cannot reproduce" actually is.
var needsLabels = []Label{
	{Name: "needs:triage", Color: "B08800", Description: "Unreviewed — needs a first pass"},
	{Name: "needs:repro", Color: "8E6A3F", Description: "Cannot reproduce — needs steps or a case"},
	{Name: "needs:decision", Color: "5319E7", Description: "Blocked on a human decision, not on a task"},
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
	out, _ := GetTemplatesWithConflicts(projectType)
	return out
}

// GetTemplatesWithConflicts is GetTemplates plus the label names that
// were generated more than once with disagreeing colors.
//
// The conflict list exists because the alternative to reporting a
// duplicate is failing on one, and failing would leave `label init`
// refusing to seed anything at all (see dedupeByName). Callers that
// render to a user — `label init` — print it; callers that only need
// the set use GetTemplates and ignore it.
func GetTemplatesWithConflicts(projectType ProjectType) ([]Label, []LabelConflict) {
	generated := generatedAxes()
	common := make([]Label, 0, len(typeLabels)+len(generated)+len(needsLabels))
	common = append(common, typeLabels...)
	common = append(common, generated...)
	common = append(common, needsLabels...)

	var domains []Label
	switch projectType {
	case TypeGoBinary:
		// `domain:storage` is added to the original four. A Go CLI that
		// keeps state — tlc itself is one: sqlite plus a file tree — has
		// a persistence layer that is neither `domain:io` nor
		// `domain:core`. `domain:io` is the boundary the process reads
		// and writes ACROSS (stdin, stdout, a file handed to it);
		// storage is the durable state it OWNS, where a change means a
		// migration and a compatibility question. Filing a schema change
		// under `domain:io` puts it next to output-formatting work it
		// has nothing in common with.
		domains = []Label{
			{Name: "domain:cli", Color: "BFD4F2", Description: "CLI"},
			{Name: "domain:core", Color: "0052CC", Description: "Core logic"},
			{Name: "domain:config", Color: "0E8A16", Description: "Config"},
			{Name: "domain:io", Color: "1D76DB", Description: "I/O"},
			{Name: "domain:storage", Color: "006B75", Description: "Persistence and schema"},
		}
	case TypeReactFrontend:
		// `domain:state` replaces `domain:hooks`, and `domain:api` is
		// added.
		//
		// Hooks are one framework's spelling of one concern. A React
		// codebase that moves to signals, or one that never adopted
		// hooks, still has state management to label; `domain:hooks`
		// stops describing where the work is the moment the idiom moves,
		// and unlike `domain:components` — which names a thing every UI
		// framework has — nothing else fits under it in the meantime.
		//
		// `domain:api` was present in generic and absent here, which had
		// the asymmetry backwards: a frontend is defined by calling an
		// API it does not own, and client-side integration work — the
		// fetch layer, response shapes, error and retry handling — is
		// among the most commonly filed work in this project shape.
		domains = []Label{
			{Name: "domain:frontend", Color: "E99695", Description: "Frontend UI"},
			{Name: "domain:components", Color: "1D76DB", Description: "Components"},
			{Name: "domain:state", Color: "0052CC", Description: "Client state management"},
			{Name: "domain:api", Color: "0075CA", Description: "API integration"},
		}
	case TypeNodeBackend:
		// A `package.json` with no app entrypoint. The domains are the
		// four things a service is asked to do that a triager can tell
		// apart from a one-line report: serve a request (`api`), read or
		// write persistent state (`db`), decide who may (`auth`), or do
		// something out of band (`jobs`).
		//
		// `domain:auth` earns its own label rather than living under
		// `api` because it is the one layer where a bug is a security
		// bug, which changes who reviews it and how fast.
		//
		// `domain:jobs` covers queues, workers and schedules together:
		// what they share, and what separates them from `api`, is that
		// nothing is waiting on the other end of the request — so a
		// failure is silent, and that is the fact worth labeling.
		domains = []Label{
			{Name: "domain:api", Color: "1D76DB", Description: "HTTP and API surface"},
			{Name: "domain:db", Color: "0E8A16", Description: "Database and persistence"},
			{Name: "domain:auth", Color: "5319E7", Description: "Authn and authz"},
			{Name: "domain:jobs", Color: "FBCA04", Description: "Background jobs and queues"},
		}
	case TypeLibrary:
		// A package consumed by other code, with no entrypoint of its
		// own. The whole set turns on one distinction the other shapes
		// do not have to make: what is PUBLIC.
		//
		// `domain:api` here means the exported surface — the signatures
		// consumers compile against — which is a different thing from
		// the same label on node-backend, where it means the HTTP
		// surface. `domain:internal` is its complement, and the pair is
		// the whole point: for a library, "is this change visible to
		// consumers?" is the first question asked about any change, and
		// these two labels answer it before anyone opens the diff.
		//
		// `domain:docs` is here and not everywhere because for a library
		// the docs ARE part of the product: a consumer cannot read the
		// source of a dependency the way a maintainer reads their own
		// app, so a doc gap is a usability defect rather than a chore.
		//
		// `domain:compat` is deliberately NOT included, even though
		// breaking changes matter most in this shape. `type:breaking`
		// already exists on the generated axis and says the same thing
		// more precisely — it is derived from the Conventional Commits
		// marker, so it stays in step with the commit that caused it.
		// A `domain:compat` beside it would be a second label for one
		// fact, and the two would be free to disagree: a triager could
		// mark `domain:compat` on a change the commit never flagged as
		// breaking. The axes are orthogonal on purpose — `type:breaking`
		// says WHAT KIND of change, `domain:api` says WHERE — and
		// `domain:compat` would be a domain smuggling in a type.
		domains = []Label{
			{Name: "domain:api", Color: "0052CC", Description: "Public API surface"},
			{Name: "domain:internal", Color: "6A737D", Description: "Internal implementation"},
			{Name: "domain:docs", Color: "0075CA", Description: "Docs and examples"},
		}
	case TypeMonorepo:
		// The one shape where a layer-shaped domain set would be wrong.
		// A monorepo's packages already have their own layers, and they
		// differ per package — labeling a task `domain:api` in a repo
		// holding six services says nothing about where to look.
		//
		// What IS repo-wide is the work that crosses every package, and
		// that is what these three name: the build and task graph
		// (`tooling`), version and publish coordination (`release`), and
		// shared dependency management (`deps`). Each is work that
		// exists BECAUSE the packages share a repo, so each is
		// unambiguous at this level in a way no layer label is.
		//
		// `domain:deps` is separate from `domain:tooling` because a
		// version bump in a shared dependency and a change to the build
		// graph have different blast radii and different reviewers, even
		// though both live in the root config.
		domains = []Label{
			{Name: "domain:tooling", Color: "FBCA04", Description: "Build graph and workspace tooling"},
			{Name: "domain:release", Color: "5319E7", Description: "Versioning and publishing"},
			{Name: "domain:deps", Color: "8D6E63", Description: "Shared dependencies"},
		}
	case TypeInfra:
		// Declarative infrastructure. The split is by what BREAKS when
		// the change is wrong, which in this shape is the only useful
		// question: a bad `terraform` change destroys and recreates
		// state, a bad `k8s` change rolls out to running workloads, a
		// bad `network` change severs access to both, and a bad
		// `secrets` change is a disclosure.
		//
		// `domain:network` is separate from `domain:k8s` because network
		// scope crosses the cluster boundary — VPCs, DNS, load
		// balancers, ingress — and is the classic source of an outage
		// that looks like an application failure.
		//
		// `domain:secrets` is included for the same reason node-backend
		// gets `domain:auth`: it is the label that changes who must look
		// at the change, and a repo carrying secret material wants that
		// visible on the issue rather than discovered in review.
		domains = []Label{
			{Name: "domain:terraform", Color: "5319E7", Description: "Terraform and IaC"},
			{Name: "domain:k8s", Color: "1D76DB", Description: "Kubernetes manifests and charts"},
			{Name: "domain:network", Color: "0E8A16", Description: "Networking, DNS, ingress"},
			{Name: "domain:secrets", Color: "B60205", Description: "Secrets and credentials"},
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
		// Generic names the shape we know nothing else about, so every
		// label here has to be one that holds for ANY repo. `domain:docs`
		// and `domain:ci` qualify and `domain:core`/`domain:api` were
		// already assumed to: without them the only labelable work in an
		// undetected project is code, which leaves the two most common
		// kinds of non-code task — writing something down, and fixing
		// the pipeline — with nowhere to go but a project-specific
		// template the user does not have.
		//
		// `domain:ci` overlaps `type:ci`, and that overlap is fine
		// because the axes answer different questions. `type:ci` is the
		// Conventional Commits type of the change; `domain:ci` is the
		// part of the repo it lands in. A `fix` to a flaky workflow is
		// `type:fix` + `domain:ci`, and neither label alone places it.
		domains = []Label{
			{Name: "domain:core", Color: "0052CC", Description: "Core logic"},
			{Name: "domain:api", Color: "1D76DB", Description: "API layer"},
			{Name: "domain:docs", Color: "0075CA", Description: "Documentation"},
			{Name: "domain:ci", Color: "BFD4F2", Description: "CI and automation"},
		}
	default:
		// An unknown --type value. It reaches here as a ProjectType
		// because the flag is a free string, so the generic set is the
		// only honest answer.
		//
		// It recurses into the generic case rather than repeating the
		// literal. The duplicate that used to sit here is why
		// TestEveryAdvertisedTypeIsDistinct has to compare sets instead
		// of reading the switch: two copies of one vocabulary can drift,
		// and adding to generic while forgetting the copy would give an
		// unknown type a SMALLER set than the generic it is supposed to
		// be identical to.
		return GetTemplatesWithConflicts(TypeGeneric)
	}

	return dedupeByName(append(common, domains...))
}

// DomainTagPrefix is the `domain:` axis prefix, without a wildcard.
//
// One spelling of the prefix, so the seeder that reads a template's
// domain labels and the vocabulary that admits them cannot disagree
// about where the axis starts.
const DomainTagPrefix = "domain:"

// DomainTags returns the `domain:*` label names a project type seeds,
// sorted and de-duplicated.
//
// It exists so `label init` can RECORD what it seeded into
// `task.tags.allowed`. The domain axis is the one axis core cannot
// enumerate for itself — its values are chosen per project TYPE, and
// core cannot see the type — so under a closed policy core has to admit
// the whole namespace by wildcard. Writing the chosen values into the
// config is what lets a closed policy name them literally instead, and
// this is the only place that knows which values were chosen.
//
// Sorted rather than left in template order: the result is written to a
// config file, and a file that reordered itself between runs would show
// a diff on a command that is annotated idempotent.
func DomainTags(projectType ProjectType) []string {
	labels := GetTemplates(projectType)

	seen := make(map[string]struct{}, len(labels))
	out := make([]string, 0, len(labels))
	for _, l := range labels {
		if !strings.HasPrefix(l.Name, DomainTagPrefix) {
			continue
		}
		if _, ok := seen[l.Name]; ok {
			continue
		}
		seen[l.Name] = struct{}{}
		out = append(out, l.Name)
	}
	sort.Strings(out)
	return out
}
