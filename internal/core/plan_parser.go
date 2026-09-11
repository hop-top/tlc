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
	"hop.top/tlc/internal/uriutil"
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
// explicit task ID ("T-NNNN"), a cross-track reference
// ("<track-id>#N", 1-based task number), or a cross-project
// reference ("org/project/T-NNNN", canonical; "org/project#T-NNNN"
// and "tlc://org/project/T-NNNN" also accepted).
type BlockedByRef struct {
	// Index is >= 0 when this entry is an intra-track index.
	Index int
	// TaskID is set when the entry is a concrete "T-NNNN" string.
	TaskID string
	// CrossTrack is set when the entry is "<track-id>#N".
	CrossTrack *CrossTrackRef
	// CrossProject is set when the entry references a task in
	// another project (e.g. "hop-top/c12n/T-0018", canonical;
	// "hop-top/c12n#T-0018" and "tlc://hop-top/c12n/T-0018" legacy).
	CrossProject *CrossProjectRef
}

// CrossProjectRef identifies a task in another project.
type CrossProjectRef struct {
	ProjectID string // e.g. "hop-top/c12n"
	TaskID    string // e.g. "T-0018"
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
	case r.CrossProject != nil:
		return fmt.Sprintf("%s#%s", r.CrossProject.ProjectID, r.CrossProject.TaskID)
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
	return r.CrossTrack == nil && r.CrossProject == nil && r.TaskID == ""
}

var (
	// taskIDPattern matches "T-NNNN" task IDs (1+ digits).
	taskIDPattern = regexp.MustCompile(`^T-\d+$`)
	// crossTrackPattern matches "<track-id>#<N>" refs. Track IDs
	// are lowercase alphanumerics with hyphens.
	crossTrackPattern = regexp.MustCompile(
		`^([a-z0-9][a-z0-9-]*)#(\d+)$`,
	)
	// crossProjectPattern matches the legacy "<org/project>#<T-NNNN>"
	// spelling. Deprecated in favor of crossProjectSlashPattern but
	// still accepted so existing plans keep working.
	crossProjectPattern = regexp.MustCompile(
		`^([a-zA-Z0-9_-]+/[a-zA-Z0-9_-]+)#(T-\d+)$`,
	)
	// crossProjectSlashPattern matches the canonical
	// "<org>/<project>/<T-NNNN>" ref. The slash form is what the
	// resolver stores and what `--blocked-by` accepts, so plan
	// frontmatter uses the same spelling. Project IDs may carry more
	// than two segments; the task ID is the final segment.
	crossProjectSlashPattern = regexp.MustCompile(
		`^([a-zA-Z0-9_-]+(?:/[a-zA-Z0-9_-]+)+)/(T-\d+)$`,
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
		// Canonical cross-project ref: "org/project/T-NNNN".
		if m := crossProjectSlashPattern.FindStringSubmatch(s); m != nil {
			return BlockedByRef{
				CrossProject: &CrossProjectRef{
					ProjectID: m[1],
					TaskID:    m[2],
				},
			}, nil
		}
		// Legacy cross-project shorthand: "org/project#T-NNNN".
		if m := crossProjectPattern.FindStringSubmatch(s); m != nil {
			return BlockedByRef{
				CrossProject: &CrossProjectRef{
					ProjectID: m[1],
					TaskID:    m[2],
				},
			}, nil
		}
		// Full URI: "tlc://org/project/T-NNNN".
		if strings.HasPrefix(s, "tlc://") {
			ref, uriErr := parseTLCURIRef(s)
			if uriErr != nil {
				return BlockedByRef{}, uriErr
			}
			return ref, nil
		}
		// Allow bare integers as strings too (yaml quirks).
		if n, err := strconv.Atoi(s); err == nil && n >= 0 {
			return BlockedByRef{Index: n}, nil
		}
		return BlockedByRef{}, fmt.Errorf(
			"blocked-by entry %q is not a valid form; expected an int "+
				"index, \"T-NNNN\", \"<track-id>#<N>\", "+
				"\"<org>/<project>/<T-NNNN>\", or "+
				"\"tlc://<org>/<project>/<T-NNNN>\"",
			s,
		)
	default:
		return BlockedByRef{}, fmt.Errorf(
			"blocked-by entry has unsupported type %T; "+
				"expected int or string", raw,
		)
	}
}

// splitTLCURI decomposes a "tlc://..." URI into (namespace, id) without
// going through scheme.Parse, which rejects empty-namespace URIs. The
// historical contract accepts "tlc:///T-NNNN" as a local task ref.
//
// Returns ok=false only when the input does not start with "tlc://".
// Empty namespace and empty id are both permitted at this layer; callers
// validate task ID shape after splitting.
func splitTLCURI(s string) (namespace, id string, ok bool) {
	const prefix = "tlc://"
	if !strings.HasPrefix(s, prefix) {
		return "", "", false
	}
	rest := strings.TrimPrefix(s, prefix)
	// Trim query/fragment; plan refs do not use them.
	if i := strings.IndexAny(rest, "?#"); i >= 0 {
		rest = rest[:i]
	}
	if rest == "" {
		return "", "", true
	}
	// First segment is namespace; remainder (joined) is id.
	if i := strings.Index(rest, "/"); i >= 0 {
		return rest[:i], rest[i+1:], true
	}
	// Single segment with no trailing slash → treat as namespace, id empty.
	return rest, "", true
}

// parseTLCURIRef parses a "tlc://..." URI into a BlockedByRef.
//
// Accepted forms:
//   - tlc://org/project/T-NNNN → CrossProject{org/project, T-NNNN}
//   - tlc:///T-NNNN            → TaskID (local bare ref)
func parseTLCURIRef(s string) (BlockedByRef, error) {
	// scheme.Parse requires a non-empty namespace, so parse the URI
	// components manually here to preserve the historical contract
	// that "tlc:///T-NNNN" (empty namespace, local task ref) is valid
	// and that "tlc://incomplete" surfaces the friendlier "missing or
	// invalid task ID" message rather than a low-level parser error.
	namespace, id, ok := splitTLCURI(s)
	if !ok {
		return BlockedByRef{}, fmt.Errorf(
			"blocked-by entry %q: invalid tlc URI; "+
				"expected tlc://<org>/<project>/<T-NNNN> or tlc:///T-NNNN",
			s,
		)
	}

	projectID, taskID := uriutil.SplitProjectTask(namespace, id)

	if taskID == "" || !taskIDPattern.MatchString(taskID) {
		return BlockedByRef{}, fmt.Errorf(
			"blocked-by entry %q: missing or invalid task ID; "+
				"expected tlc://<org>/<project>/<T-NNNN> or tlc:///T-NNNN",
			s,
		)
	}

	if projectID == "" {
		// Local task ref: tlc:///T-NNNN
		return BlockedByRef{TaskID: taskID}, nil
	}

	return BlockedByRef{
		CrossProject: &CrossProjectRef{
			ProjectID: projectID,
			TaskID:    taskID,
		},
	}, nil
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
