package core

import (
	"encoding/json"
	"fmt"
	"regexp"

	"gopkg.in/yaml.v3"
)

// Recipe is a generation-only template: expanded, rendered and
// materialized into a track of tasks, then forgotten. It never executes
// anything itself — `track execute` is the single engine.
type Recipe struct {
	Name        string            `yaml:"recipe" json:"recipe"`
	Version     string            `yaml:"version" json:"version"`
	Description string            `yaml:"description,omitempty" json:"description,omitempty"`
	Requires    RecipeRequires    `yaml:"requires,omitempty" json:"requires,omitempty"`
	Vars        map[string]VarDef `yaml:"vars,omitempty" json:"vars,omitempty"`
	Agent       string            `yaml:"agent,omitempty" json:"agent,omitempty"`
	Track       *RecipeTrack      `yaml:"track,omitempty" json:"track,omitempty"`
	Steps       []RecipeStep      `yaml:"steps" json:"steps"`

	// Path is the file the recipe was parsed from, Source the directory or
	// layer it was located in, Hash the sha256 of the raw bytes. None of
	// them is part of the document.
	Path   string `yaml:"-" json:"-"`
	Source string `yaml:"-" json:"-"`
	Hash   string `yaml:"-" json:"-"`
}

// Key is the pinned reference form, name@version.
func (r *Recipe) Key() string { return r.Name + "@" + r.Version }

// RecipeRequires declares what a recipe must be applied to.
type RecipeRequires struct {
	Subject string `yaml:"subject,omitempty" json:"subject,omitempty"` // task | track | none
}

// Subject kinds a recipe can require.
const (
	SubjectNone  = "none"
	SubjectTask  = "task"
	SubjectTrack = "track"
)

// VarDef declares a recipe variable. Required and Default are mutually
// exclusive: a default makes the var optional.
type VarDef struct {
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
	Required    bool   `yaml:"required,omitempty" json:"required,omitempty"`
	Default     any    `yaml:"default,omitempty" json:"default,omitempty"`
}

// RecipeTrack is what `track create --recipe` builds the track from.
type RecipeTrack struct {
	Title string `yaml:"title,omitempty" json:"title,omitempty"`
	Type  string `yaml:"type,omitempty" json:"type,omitempty"`
	Plan  string `yaml:"plan,omitempty" json:"plan,omitempty"`
}

// RecipeStep is one entry of the ordered steps list. A step is exactly one
// of: a task (default), an include of another recipe, or a repeat block
// over nested steps. Ordinal is the 1-based position after expansion.
type RecipeStep struct {
	ID          string   `yaml:"id" json:"id"`
	Kind        TaskKind `yaml:"kind,omitempty" json:"kind,omitempty"`
	Title       string   `yaml:"title,omitempty" json:"title,omitempty"`
	Description string   `yaml:"description,omitempty" json:"description,omitempty"`
	DependsOn   []string `yaml:"depends_on,omitempty" json:"depends_on,omitempty"`
	When        string   `yaml:"when,omitempty" json:"when,omitempty"`
	Agent       string   `yaml:"agent,omitempty" json:"agent,omitempty"`
	Assignee    string   `yaml:"assignee,omitempty" json:"assignee,omitempty"`
	Due         DueSpec  `yaml:"due,omitempty" json:"due,omitempty"`

	Exec  *ExecSpec  `yaml:"exec,omitempty" json:"exec,omitempty"`
	Human *HumanSpec `yaml:"human,omitempty" json:"human,omitempty"`
	Retry *RetrySpec `yaml:"retry,omitempty" json:"retry,omitempty"`
	Gate  *StepGate  `yaml:"gate,omitempty" json:"gate,omitempty"`

	// Composition: include another recipe, binding its vars through With.
	Include string         `yaml:"include,omitempty" json:"include,omitempty"`
	With    map[string]any `yaml:"with,omitempty" json:"with,omitempty"`

	// Repetition: unroll Steps Repeat times; Until stops early at dispatch
	// time through a derived `when` on every iteration after the first.
	Repeat int          `yaml:"repeat,omitempty" json:"repeat,omitempty"`
	Until  string       `yaml:"until,omitempty" json:"until,omitempty"`
	Steps  []RecipeStep `yaml:"steps,omitempty" json:"steps,omitempty"`

	Ordinal int `yaml:"-" json:"ordinal,omitempty"`
}

// IsInclude reports whether the step composes another recipe.
func (s *RecipeStep) IsInclude() bool { return s.Include != "" }

// IsRepeat reports whether the step is an unrolled block.
func (s *RecipeStep) IsRepeat() bool { return s.Repeat > 0 || len(s.Steps) > 0 }

// IsBlock reports whether the step expands into other steps rather than
// becoming a task itself.
func (s *RecipeStep) IsBlock() bool { return s.IsInclude() || s.IsRepeat() }

// EffectiveKind resolves an unset kind to agent.
func (s *RecipeStep) EffectiveKind() TaskKind {
	if s.Kind == "" {
		return TaskKindAgent
	}
	return s.Kind
}

// DueSpec is a step's due date in either form:
//
//	due: "{{sprint_end}}"                     # Raw: any string the due parser accepts,
//	due: "{{steps.planning.due}} + 5d"        # optionally followed by an offset
//	due: {after: planning, offset: 5d}        # structured: another step's due plus an offset
//
// Raw holds the scalar form and, after rendering, the resolved value of
// either form.
type DueSpec struct {
	Raw    string `yaml:"-" json:"-"`
	After  string `yaml:"after,omitempty" json:"after,omitempty"`
	Offset string `yaml:"offset,omitempty" json:"offset,omitempty"`
}

// IsZero reports whether no due was given; it lets yaml omitempty skip it.
func (d DueSpec) IsZero() bool { return d.Raw == "" && d.After == "" && d.Offset == "" }

// UnmarshalYAML accepts a scalar or an {after, offset} mapping.
func (d *DueSpec) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		d.Raw = value.Value
		return nil
	case yaml.MappingNode:
		type structured struct {
			After  string `yaml:"after"`
			Offset string `yaml:"offset"`
		}
		var s structured
		if err := dueMappingKnownFields(value); err != nil {
			return err
		}
		if err := value.Decode(&s); err != nil {
			return fmt.Errorf("due: %w", err)
		}
		d.After, d.Offset = s.After, s.Offset
		return nil
	default:
		return fmt.Errorf("line %d: due must be a string or {after, offset}", value.Line)
	}
}

// MarshalYAML emits the scalar form when set, else the structured form.
func (d DueSpec) MarshalYAML() (any, error) {
	if d.Raw != "" {
		return d.Raw, nil
	}
	return map[string]string{"after": d.After, "offset": d.Offset}, nil
}

// UnmarshalJSON mirrors UnmarshalYAML for JSON recipes.
func (d *DueSpec) UnmarshalJSON(b []byte) error {
	if len(b) > 0 && b[0] == '"' {
		return json.Unmarshal(b, &d.Raw) //nolint:wrapcheck // caller wraps with the field name
	}
	type structured struct {
		After  string `json:"after"`
		Offset string `json:"offset"`
	}
	var s structured
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("due: %w", err)
	}
	d.After, d.Offset = s.After, s.Offset
	return nil
}

// MarshalJSON mirrors MarshalYAML.
func (d DueSpec) MarshalJSON() ([]byte, error) {
	if d.Raw != "" {
		return json.Marshal(d.Raw) //nolint:wrapcheck // plain string, nothing to add
	}
	return json.Marshal(map[string]string{"after": d.After, "offset": d.Offset}) //nolint:wrapcheck // plain map
}

func dueMappingKnownFields(value *yaml.Node) error {
	for i := 0; i+1 < len(value.Content); i += 2 {
		if k := value.Content[i].Value; k != dueKeyAfter && k != dueKeyOffset {
			return fmt.Errorf("line %d: due: unknown key %q (want after, offset)", value.Content[i].Line, k)
		}
	}
	return nil
}

// Grammar shared by the parser, validator and selector.
var (
	// recipeNameRe bounds recipe names: file-name and reference safe.
	recipeNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	// stepIDRe bounds authored step ids; "/" is reserved for expansion.
	stepIDRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	// varNameRe bounds var names to identifiers so {{name}} is unambiguous.
	varNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

// reservedVarNames are template namespaces a recipe cannot shadow.
var reservedVarNames = map[string]bool{
	nsSubject: true, nsResults: true, nsRun: true, nsSteps: true, nsVars: true,
}

// Template namespaces: {{<namespace>.<field>}}. A bare {{name}} is a var.
const (
	nsVars    = "vars"
	nsSubject = "subject"
	nsRun     = "run"
	nsSteps   = "steps"
	nsResults = "results"
)

// Field names shared by the template scope, the validator and the
// rendered-step view.
const (
	fieldID          = "id"
	fieldTitle       = "title"
	fieldDescription = "description"
	fieldDue         = "due"
	fieldAssignee    = "assignee"
	fieldWhen        = "when"
	fieldAgent       = "agent"
	fieldKind        = "kind"
	fieldIteration   = "iteration"
	fieldTrack       = "track"
	fieldTags        = "tags"
	fieldProject     = "project"
	fieldRecipe      = "recipe"
	fieldVersion     = "version"
)

// Keys of the structured due form.
const (
	dueKeyAfter  = "after"
	dueKeyOffset = "offset"
)

// subjectFields are the values auto-bound from a subject task or track.
var subjectFields = map[string]bool{
	fieldID: true, fieldTitle: true, fieldDescription: true, fieldTrack: true, fieldTags: true, fieldProject: true,
}

// runFields are the values bound from the materialization run itself.
// "iteration" is only bound inside a repeat block.
var runFields = map[string]bool{
	fieldID: true, fieldRecipe: true, fieldVersion: true, fieldIteration: true,
}

// stepFields are the rendered values another step may read through
// steps.<id>.<field>.
var stepFields = map[string]bool{
	fieldID: true, fieldTitle: true, fieldDescription: true, fieldDue: true,
	fieldAssignee: true, fieldWhen: true, fieldAgent: true, fieldKind: true,
}
