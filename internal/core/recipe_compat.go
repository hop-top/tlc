package core

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"hop.top/flo/spec"
)

// Vocabulary compat between tlc recipes and flo/spec flows.
//
//	recipe:        <-> flow:
//	gate:          <-> check:
//	run.recipe     <-> run.flow
//
// Documents are rewritten before spec parses them (shim*), messages after
// (recipeWording), so recipes on disk and the text users see stay in recipe
// terms.

const (
	keyRecipe = "recipe"
	keyFlow   = "flow"
	keyGate   = "gate"
	keyCheck  = "check"
	keySteps  = "steps"
	extJSON   = ".json"
)

var runRecipeRef = regexp.MustCompile(`(\{\{[^{}]*?\brun\.)recipe\b`)

func isJSONName(filename string) bool { return strings.EqualFold(filepath.Ext(filename), extJSON) }

// shimRecipeSource rewrites a recipe document into flow vocabulary.
// Anything it cannot decode is returned unchanged for spec to report.
func shimRecipeSource(data []byte, filename string) []byte {
	data = runRecipeRef.ReplaceAll(data, []byte("${1}flow"))
	if isJSONName(filename) {
		return shimRecipeJSON(data)
	}
	return shimRecipeYAML(data)
}

type keyEdit struct {
	line, col int
	from, to  string
}

func shimRecipeYAML(data []byte) []byte {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return data
	}
	root := &doc
	if root.Kind == yaml.DocumentNode && len(root.Content) > 0 {
		root = root.Content[0]
	}
	if root.Kind != yaml.MappingNode {
		return data
	}
	var edits []keyEdit
	collectRenames(root, keyRecipe, keyFlow, &edits, func(m *yaml.Node) {
		collectRenames(m, keyTrack, tlcInflight+keyTrack, &edits, nil)
		for i := 0; i+1 < len(m.Content); i += 2 {
			if m.Content[i].Value == keySteps && m.Content[i+1].Kind == yaml.SequenceNode {
				collectStepRenames(m.Content[i+1], &edits)
			}
		}
	})
	return applyKeyEdits(data, edits)
}

// collectRenames queues the rename of key from->to in mapping m unless
// the target key is already present, then runs after on m.
func collectRenames(m *yaml.Node, from, to string, edits *[]keyEdit, after func(*yaml.Node)) {
	renamed := false
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == to {
			renamed = true
		}
	}
	for i := 0; i+1 < len(m.Content) && !renamed; i += 2 {
		if k := m.Content[i]; k.Kind == yaml.ScalarNode && k.Value == from {
			*edits = append(*edits, keyEdit{line: k.Line, col: k.Column, from: from, to: to})
		}
	}
	if after != nil {
		after(m)
	}
}

func collectStepRenames(seq *yaml.Node, edits *[]keyEdit) {
	for _, step := range seq.Content {
		if step.Kind != yaml.MappingNode {
			continue
		}
		collectRenames(step, keyGate, keyCheck, edits, func(m *yaml.Node) {
			for _, k := range tlcStepKeys {
				collectRenames(m, k, tlcInflight+k, edits, nil)
			}
			for i := 0; i+1 < len(m.Content); i += 2 {
				if m.Content[i].Value == keySteps && m.Content[i+1].Kind == yaml.SequenceNode {
					collectStepRenames(m.Content[i+1], edits)
				}
			}
		})
	}
}

// applyKeyEdits renames keys in place on their own line, so line numbers
// in later errors stay true to the file. An edit whose key text is not at
// its position (flow-style oddities) is skipped.
func applyKeyEdits(data []byte, edits []keyEdit) []byte {
	if len(edits) == 0 {
		return data
	}
	lines := strings.Split(string(data), "\n")
	sort.Slice(edits, func(i, j int) bool {
		if edits[i].line != edits[j].line {
			return edits[i].line < edits[j].line
		}
		return edits[i].col > edits[j].col
	})
	for _, e := range edits {
		if e.line < 1 || e.line > len(lines) {
			continue
		}
		line := lines[e.line-1]
		off := runeOffset(line, e.col-1)
		if off < 0 {
			continue
		}
		if off < len(line) && (line[off] == '"' || line[off] == '\'') {
			off++
		}
		if !strings.HasPrefix(line[off:], e.from) {
			continue
		}
		lines[e.line-1] = line[:off] + e.to + line[off+len(e.from):]
	}
	return []byte(strings.Join(lines, "\n"))
}

// runeOffset converts a 0-based rune column into a byte offset, or -1.
func runeOffset(s string, col int) int {
	n := 0
	for i := range s {
		if n == col {
			return i
		}
		n++
	}
	if n == col {
		return len(s)
	}
	return -1
}

func shimRecipeJSON(data []byte) []byte {
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(data, &doc); err != nil {
		return data
	}
	renameJSONKey(doc, keyRecipe, keyFlow)
	renameJSONKey(doc, keyTrack, tlcInflight+keyTrack)
	if raw, ok := doc[keySteps]; ok {
		if steps, ok := shimJSONSteps(raw); ok {
			doc[keySteps] = steps
		}
	}
	out, err := json.Marshal(doc)
	if err != nil {
		return data
	}
	return out
}

func shimJSONSteps(raw json.RawMessage) (json.RawMessage, bool) {
	var steps []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &steps); err != nil {
		return nil, false
	}
	for _, s := range steps {
		renameJSONKey(s, keyGate, keyCheck)
		for _, k := range tlcStepKeys {
			renameJSONKey(s, k, tlcInflight+k)
		}
		if nested, ok := s[keySteps]; ok {
			if out, ok := shimJSONSteps(nested); ok {
				s[keySteps] = out
			}
		}
	}
	out, err := json.Marshal(steps)
	if err != nil {
		return nil, false
	}
	return out, true
}

func renameJSONKey(m map[string]json.RawMessage, from, to string) {
	v, ok := m[from]
	if !ok {
		return
	}
	if _, clash := m[to]; clash {
		return
	}
	delete(m, from)
	m[to] = v
}

// unshimRecipeNode renames flow vocabulary back to recipe vocabulary in an
// encoded recipe document, the inverse of shimRecipeYAML.
func unshimRecipeNode(root *yaml.Node) {
	if root.Kind == yaml.DocumentNode && len(root.Content) > 0 {
		root = root.Content[0]
	}
	if root.Kind != yaml.MappingNode {
		return
	}
	unshimTLCNode(root)
	for i := 0; i+1 < len(root.Content); i += 2 {
		switch root.Content[i].Value {
		case keyFlow:
			root.Content[i].Value = keyRecipe
		case keySteps:
			unshimSteps(root.Content[i+1])
		}
	}
}

func unshimSteps(seq *yaml.Node) {
	if seq.Kind != yaml.SequenceNode {
		return
	}
	for _, step := range seq.Content {
		unshimTLCStep(step)
		for i := 0; i+1 < len(step.Content); i += 2 {
			switch step.Content[i].Value {
			case keyCheck:
				step.Content[i].Value = keyGate
			case keySteps:
				unshimSteps(step.Content[i+1])
			}
		}
	}
}

func rawHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func parseRecipe(r io.Reader, filename string) (*Recipe, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read recipe %s: %w", filename, err)
	}
	if err := rejectReservedExt(data, filename); err != nil {
		return nil, err
	}
	rec, err := spec.ParseFlow(bytes.NewReader(shimRecipeSource(data, filename)), filename)
	if err != nil {
		return nil, recipeWordingAfter(err, filename+": ")
	}
	if err := finishTLC(rec); err != nil {
		return nil, fmt.Errorf("%s: %w", filename, err)
	}
	// Drift detection compares the hash of the file as authored, not of
	// the rewritten copy.
	rec.Hash = rawHash(data)
	return rec, nil
}

// --- messages ----------------------------------------------------------

type wordedError struct {
	msg string
	err error
}

func (e *wordedError) Error() string { return e.msg }
func (e *wordedError) Unwrap() error { return e.err }

// recipeWording rewrites spec's flow/check wording to recipe/gate. The
// original error stays reachable through errors.Is/As.
func recipeWording(err error) error {
	return recipeWordingAfter(err, "")
}

// recipeWordingAfter leaves a leading skip prefix (a file name) untouched.
func recipeWordingAfter(err error, skip string) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	head := ""
	if skip != "" && strings.HasPrefix(msg, skip) {
		head, msg = skip, msg[len(skip):]
	}
	out := head + recipeWords(msg)
	if out == err.Error() {
		return err
	}
	return &wordedError{msg: out, err: err}
}

// quotedKeyWords maps the few quoted spans that name the document's own
// keys; every other quoted span (ids, values) is left alone.
var quotedKeyWords = strings.NewReplacer(
	"run.flow", "run.recipe",
	`required field "flow"`, `required field "recipe"`,
	`with "flow:"`, `with "recipe:"`,
)

var (
	quotedSpan   = regexp.MustCompile(`"(?:[^"\\]|\\.)*"`)
	flowCheckWrd = regexp.MustCompile(`\b(flow|check)\b`)
	fileExtAfter = regexp.MustCompile(`^\.(ya?ml|json)\b`)
)

// recipeWords maps whole words flow->recipe and check->gate outside quoted
// spans, step ids ("step check:") and path-like or identifier-like uses.
func recipeWords(msg string) string {
	msg = quotedKeyWords.Replace(msg)
	var out strings.Builder
	last := 0
	for _, q := range quotedSpan.FindAllStringIndex(msg, -1) {
		out.WriteString(mapWords(msg[last:q[0]]))
		out.WriteString(msg[q[0]:q[1]])
		last = q[1]
	}
	out.WriteString(mapWords(msg[last:]))
	return out.String()
}

func mapWords(seg string) string {
	var out strings.Builder
	last := 0
	for _, m := range flowCheckWrd.FindAllStringSubmatchIndex(seg, -1) {
		start, end := m[0], m[1]
		if wordIsIdentifier(seg, start, end) {
			continue
		}
		out.WriteString(seg[last:start])
		if seg[start:end] == keyFlow {
			out.WriteString(keyRecipe)
		} else {
			out.WriteString(keyGate)
		}
		last = end
	}
	out.WriteString(seg[last:])
	return out.String()
}

// wordIsIdentifier reports a match that is part of a path, file name,
// hyphenated id or step id rather than prose.
func wordIsIdentifier(s string, start, end int) bool {
	if start > 0 && strings.ContainsRune("-_/.:", rune(s[start-1])) {
		return true
	}
	if end < len(s) && strings.ContainsRune("-_/", rune(s[end])) {
		return true
	}
	if end < len(s) && fileExtAfter.MatchString(s[end:]) {
		return true
	}
	before := s[:start]
	return strings.HasSuffix(before, "step ") || strings.HasSuffix(before, "steps ")
}

// --- locator -----------------------------------------------------------

// shimFS serves recipe files to spec's scanner and loader in flow
// vocabulary.
type shimFS struct{ fs.FS }

func (s shimFS) ReadFile(name string) ([]byte, error) {
	data, err := fs.ReadFile(s.FS, name)
	if err != nil {
		return nil, err //nolint:wrapcheck // fs error passes through unchanged
	}
	if err := rejectReservedExt(data, name); err != nil {
		return nil, err
	}
	return shimRecipeSource(data, name), nil
}
