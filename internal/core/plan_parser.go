package core

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// PlanFrontmatter represents the YAML frontmatter of a plan document.
type PlanFrontmatter struct {
	Title  string         `yaml:"title"`
	Tracks []string       `yaml:"tracks,omitempty"`
	Tasks  []PlanTaskSpec `yaml:"tasks,omitempty"`
}

// PlanTaskSpec describes a task to be created from a plan.
type PlanTaskSpec struct {
	Title       string         `yaml:"title"`
	Description string         `yaml:"description,omitempty"`
	Tags        []string       `yaml:"tags,omitempty"`
	Effort      string         `yaml:"effort,omitempty"`
	Priority    string         `yaml:"priority,omitempty"`
	AssignedTo  string         `yaml:"assigned-to,omitempty"`
	BlockedBy   []BlockedByRef `yaml:"blocked-by,omitempty"`
}

// BlockedByRef is one entry in a PlanTaskSpec.BlockedBy list. It
// represents either an intra-track index (int, 0-based), an
// explicit task ID ("T-NNNN"), or a cross-track reference
// ("<track-id>#N", 1-based task number).
type BlockedByRef struct {
	// Index is >= 0 when this entry is an intra-track index.
	Index int
	// TaskID is set when the entry is a concrete "T-NNNN" string.
	TaskID string
	// CrossTrack is set when the entry is "<track-id>#N".
	CrossTrack *CrossTrackRef
}

// CrossTrackRef identifies a task in another track by its 1-based
// position in that track's plan frontmatter.
type CrossTrackRef struct {
	TrackID string
	TaskNum int // 1-based
}

// Raw returns the original string/int form of this reference as it
// appears (or would appear) in plan YAML.
func (r BlockedByRef) Raw() string {
	switch {
	case r.CrossTrack != nil:
		return fmt.Sprintf("%s#%d", r.CrossTrack.TrackID, r.CrossTrack.TaskNum)
	case r.TaskID != "":
		return r.TaskID
	default:
		return strconv.Itoa(r.Index)
	}
}

// IsIndex reports whether this ref is an intra-track index.
func (r BlockedByRef) IsIndex() bool {
	return r.CrossTrack == nil && r.TaskID == ""
}

var (
	// taskIDPattern matches "T-NNNN" task IDs (1+ digits).
	taskIDPattern = regexp.MustCompile(`^T-\d+$`)
	// crossTrackPattern matches "<track-id>#<N>" refs. Track IDs
	// are lowercase alphanumerics with hyphens.
	crossTrackPattern = regexp.MustCompile(
		`^([a-z0-9][a-z0-9-]*)#(\d+)$`,
	)
)

// parseBlockedByRef parses a single raw value (int or string) into
// a BlockedByRef.
func parseBlockedByRef(raw interface{}) (BlockedByRef, error) {
	switch v := raw.(type) {
	case int:
		if v < 0 {
			return BlockedByRef{}, fmt.Errorf(
				"blocked-by int entry %d must be >= 0", v,
			)
		}
		return BlockedByRef{Index: v}, nil
	case int64:
		return parseBlockedByRef(int(v))
	case float64:
		return parseBlockedByRef(int(v))
	case string:
		s := strings.TrimSpace(v)
		if s == "" {
			return BlockedByRef{}, fmt.Errorf(
				"blocked-by entry is empty string; use an int index, " +
					"a task ID like \"T-0001\", or \"<track>#<N>\"",
			)
		}
		if taskIDPattern.MatchString(s) {
			return BlockedByRef{TaskID: s}, nil
		}
		if m := crossTrackPattern.FindStringSubmatch(s); m != nil {
			n, _ := strconv.Atoi(m[2]) //nolint:errcheck // regex guarantees digits
			if n < 1 {
				return BlockedByRef{}, fmt.Errorf(
					"blocked-by cross-track ref %q: task number must be >= 1",
					s,
				)
			}
			return BlockedByRef{
				CrossTrack: &CrossTrackRef{TrackID: m[1], TaskNum: n},
			}, nil
		}
		// Allow bare integers as strings too (yaml quirks).
		if n, err := strconv.Atoi(s); err == nil && n >= 0 {
			return BlockedByRef{Index: n}, nil
		}
		return BlockedByRef{}, fmt.Errorf(
			"blocked-by entry %q is not a valid form; expected an int "+
				"index, \"T-NNNN\", or \"<track-id>#<N>\"",
			s,
		)
	default:
		return BlockedByRef{}, fmt.Errorf(
			"blocked-by entry has unsupported type %T; "+
				"expected int or string", raw,
		)
	}
}

// UnmarshalYAML implements yaml.Unmarshaler so BlockedByRef can be
// decoded from either a scalar int or a scalar string inside a
// mixed sequence.
func (r *BlockedByRef) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.ScalarNode {
		return fmt.Errorf(
			"blocked-by entry at line %d: expected scalar, got kind %d",
			node.Line, node.Kind,
		)
	}

	// Try int first when the tag says so or the value parses.
	if node.Tag == "!!int" || node.Tag == "" {
		if n, err := strconv.Atoi(node.Value); err == nil {
			ref, pErr := parseBlockedByRef(n)
			if pErr != nil {
				return fmt.Errorf(
					"blocked-by entry at line %d: %w", node.Line, pErr,
				)
			}
			*r = ref
			return nil
		}
	}

	ref, err := parseBlockedByRef(node.Value)
	if err != nil {
		return fmt.Errorf(
			"blocked-by entry at line %d: %w", node.Line, err,
		)
	}
	*r = ref
	return nil
}

// UnmarshalJSON implements json.Unmarshaler so BlockedByRef can be
// decoded from JSON extractor output (which is string-or-int).
func (r *BlockedByRef) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return fmt.Errorf("blocked-by entry: empty JSON value")
	}
	if trimmed == "null" {
		return fmt.Errorf(
			"blocked-by entry: null is not allowed; use an int " +
				"index, \"T-NNNN\", or \"<track>#<N>\"",
		)
	}
	// String form.
	if trimmed[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		ref, err := parseBlockedByRef(s)
		if err != nil {
			return err
		}
		*r = ref
		return nil
	}
	// Number form.
	var n int
	if err := json.Unmarshal(data, &n); err != nil {
		return fmt.Errorf(
			"blocked-by entry %s: %w", trimmed, err,
		)
	}
	ref, err := parseBlockedByRef(n)
	if err != nil {
		return err
	}
	*r = ref
	return nil
}

// ParsePlanFrontmatter reads a markdown file and extracts YAML frontmatter.
// Frontmatter must be delimited by "---" lines at the start of the file.
// Returns an error if the file cannot be read or the YAML is malformed.
// Returns nil with no error if the file has no frontmatter.
func ParsePlanFrontmatter(path string) (*PlanFrontmatter, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf(
			"plan file %q: cannot open; verify path exists", path,
		)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)

	// First line must be "---".
	if !scanner.Scan() {
		return nil, nil // empty file
	}
	if strings.TrimSpace(scanner.Text()) != "---" {
		return nil, nil // no frontmatter
	}

	// Collect YAML lines until closing "---".
	var yamlLines []string
	found := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "---" {
			found = true
			break
		}
		yamlLines = append(yamlLines, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf(
			"plan file %q: read error; %v", path, err,
		)
	}
	if !found {
		return nil, fmt.Errorf(
			"plan file %q: unclosed frontmatter; add closing '---' line",
			path,
		)
	}

	raw := strings.Join(yamlLines, "\n")
	var fm PlanFrontmatter
	if err := yaml.Unmarshal([]byte(raw), &fm); err != nil {
		return nil, fmt.Errorf(
			"plan file %q: malformed YAML frontmatter; %v", path, err,
		)
	}

	return &fm, nil
}
