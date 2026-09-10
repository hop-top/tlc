package inbox

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"hop.top/tlc/internal/core"
)

// ParseResult holds parsed task creation data from inbox files.
type ParseResult struct {
	Title       string   `json:"title"       yaml:"title"`
	Description string   `json:"description" yaml:"description"`
	Status      string   `json:"status"      yaml:"status"`
	AssignedTo  string   `json:"assigned_to" yaml:"assigned_to"`
	Tags        []string `json:"tags"        yaml:"tags"`
	Effort      string   `json:"effort"      yaml:"effort"`
	Priority    string   `json:"priority"    yaml:"priority"`
	TrackID     string   `json:"track_id"    yaml:"track_id"`
}

// TransitionIntent holds parsed transition data from inbox files.
type TransitionIntent struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Note   string `json:"note"`
	By     string `json:"by"`
}

// ParseCreateJSON unmarshals JSON into a ParseResult, validates title,
// and applies defaults.
func ParseCreateJSON(data []byte) (*ParseResult, error) {
	var r ParseResult
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf(
			"inbox create JSON: invalid payload; check JSON syntax",
		)
	}
	if err := validateCreate(&r); err != nil {
		return nil, err
	}
	applyCreateDefaults(&r)
	return &r, nil
}

// ParseCreateMarkdown parses YAML frontmatter + body as description.
// Frontmatter must be delimited by "---" lines.
func ParseCreateMarkdown(data []byte) (*ParseResult, error) {
	fm, body, err := splitFrontmatter(data)
	if err != nil {
		return nil, err
	}

	var r ParseResult
	if err := yaml.Unmarshal(fm, &r); err != nil {
		return nil, fmt.Errorf(
			"inbox create markdown: invalid frontmatter YAML; %v",
			err,
		)
	}

	desc := strings.TrimSpace(string(body))
	if desc != "" {
		r.Description = desc
	}

	if err := validateCreate(&r); err != nil {
		return nil, err
	}
	applyCreateDefaults(&r)
	return &r, nil
}

// ParseTransitionJSON unmarshals JSON into a TransitionIntent and
// validates required fields (id, status).
func ParseTransitionJSON(data []byte) (*TransitionIntent, error) {
	var t TransitionIntent
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf(
			"inbox transition JSON: invalid payload; check JSON syntax",
		)
	}
	if err := validateTransition(&t); err != nil {
		return nil, err
	}
	if t.By == "" {
		t.By = "inbox"
	}
	return &t, nil
}

// validateCreate checks title is non-empty and enums are valid.
func validateCreate(r *ParseResult) error {
	if strings.TrimSpace(r.Title) == "" {
		return fmt.Errorf(
			"inbox create: title is required; " +
				"add a non-empty title field",
		)
	}
	// Each gate below validates through core and names its allowed set
	// from the same effective vocabulary, rather than keeping the inbox's
	// own copy. The status map that used to live here predated any
	// trustworthy shared helper; keeping it meant a project declaring
	// IN_REVIEW had the CLI accept the status and the inbox reject it.
	// The retyped priority list is the same defect caught in the act — it
	// read "P0, P1, P2, P3, P4", and P4 has never been a tlc priority.
	if !core.ValidTaskStatus(core.TaskStatus(r.Status)) {
		return fmt.Errorf(
			"inbox create: invalid status %q; allowed: %s",
			r.Status,
			core.TaskStatusVocabularyList(),
		)
	}
	if !core.ValidPriority(core.Priority(r.Priority)) {
		return fmt.Errorf(
			"inbox create: invalid priority %q; allowed: %s",
			r.Priority,
			core.PriorityVocabularyList(),
		)
	}
	if !core.ValidEffort(core.Effort(r.Effort)) {
		return fmt.Errorf(
			"inbox create: invalid effort %q; allowed: %s",
			r.Effort,
			core.EffortVocabularyList(),
		)
	}
	return nil
}

// applyCreateDefaults sets Status to "TODO" when not specified.
func applyCreateDefaults(r *ParseResult) {
	if r.Status == "" {
		r.Status = "TODO"
	}
}

// validateTransition checks that id and status are non-empty.
func validateTransition(t *TransitionIntent) error {
	if strings.TrimSpace(t.ID) == "" {
		return fmt.Errorf(
			"inbox transition: id is required; add a non-empty id field",
		)
	}
	if strings.TrimSpace(t.Status) == "" {
		return fmt.Errorf(
			"inbox transition: status is required; " +
				"add a non-empty status field",
		)
	}
	// The second status gate, and it has to read the same vocabulary as
	// the create gate above. Fixing only one leaves a config-declared
	// status that can be created but never transitioned to.
	if !core.ValidTaskStatus(core.TaskStatus(t.Status)) {
		return fmt.Errorf(
			"inbox transition: invalid status %q; allowed: %s",
			t.Status,
			core.TaskStatusVocabularyList(),
		)
	}
	return nil
}

// splitFrontmatter splits markdown content into YAML frontmatter and
// body. Frontmatter must start and end with "---" lines.
func splitFrontmatter(data []byte) ([]byte, []byte, error) {
	s := string(data)
	if !strings.HasPrefix(s, "---") {
		return nil, nil, fmt.Errorf(
			"inbox create markdown: missing frontmatter; " +
				"file must start with '---'",
		)
	}

	// Find end delimiter after the opening "---\n".
	rest := s[3:]
	if len(rest) > 0 && rest[0] == '\n' {
		rest = rest[1:]
	} else if len(rest) > 1 && rest[0] == '\r' && rest[1] == '\n' {
		rest = rest[2:]
	}

	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return nil, nil, fmt.Errorf(
			"inbox create markdown: unclosed frontmatter; " +
				"add closing '---' delimiter",
		)
	}

	fm := []byte(rest[:idx])
	after := rest[idx+4:] // skip "\n---"
	// Skip optional newline after closing delimiter.
	after = strings.TrimPrefix(after, "\n")
	after = strings.TrimPrefix(after, "\r\n")

	return fm, bytes.TrimSpace([]byte(after)), nil
}
