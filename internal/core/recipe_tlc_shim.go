package core

import (
	"encoding/json"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// Document side of the tlc-owned keys. shimRecipeYAML/JSON (recipe_compat.go)
// rename the plain keys to in-flight x-tlc.<key> extension keys on the way
// in; the functions here refuse an authored x-tlc and move the keys back
// out of an encoded document.

func isReservedExtKey(k string) bool { return k == extTLC || strings.HasPrefix(k, tlcInflight) }

// rejectReservedExt refuses a document that writes x-tlc itself, on the
// recipe root or on any step: the key is tlc's own carrier. Other x-*
// keys are the author's and pass through.
func rejectReservedExt(data []byte, filename string) error {
	if isJSONName(filename) {
		var doc map[string]json.RawMessage
		if json.Unmarshal(data, &doc) != nil {
			return nil // spec reports the syntax error
		}
		return reservedInJSON(doc, filename)
	}
	var doc yaml.Node
	if yaml.Unmarshal(data, &doc) != nil {
		return nil
	}
	root := &doc
	if root.Kind == yaml.DocumentNode && len(root.Content) > 0 {
		root = root.Content[0]
	}
	return reservedInYAML(root, filename)
}

func reservedErr(filename string, line int, key string) error {
	where := filename
	if line > 0 {
		where = fmt.Sprintf("%s: line %d", filename, line)
	}
	return fmt.Errorf("%s: key %q is reserved for tlc; write track:, tags:, meta: or priority: instead", where, key)
}

func reservedInYAML(m *yaml.Node, filename string) error {
	if m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		k, v := m.Content[i], m.Content[i+1]
		if isReservedExtKey(k.Value) {
			return reservedErr(filename, k.Line, k.Value)
		}
		if k.Value == keySteps && v.Kind == yaml.SequenceNode {
			for _, step := range v.Content {
				if err := reservedInYAML(step, filename); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func reservedInJSON(m map[string]json.RawMessage, filename string) error {
	for k, raw := range m {
		if isReservedExtKey(k) {
			return reservedErr(filename, 0, k)
		}
		if k != keySteps {
			continue
		}
		var steps []map[string]json.RawMessage
		if json.Unmarshal(raw, &steps) != nil {
			continue
		}
		for _, s := range steps {
			if err := reservedInJSON(s, filename); err != nil {
				return err
			}
		}
	}
	return nil
}

// unshimTLCNode moves the x-tlc entry of an encoded recipe root back to
// the plain track: key, placed before steps.
func unshimTLCNode(root *yaml.Node) {
	idx, inner := takeTLC(root)
	if idx < 0 {
		return
	}
	at := len(root.Content)
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == keySteps {
			at = i
			break
		}
	}
	tail := append([]*yaml.Node(nil), root.Content[at:]...)
	root.Content = append(append(root.Content[:at], inner...), tail...)
}

// unshimTLCStep replaces a step's x-tlc entry with the plain tags:,
// meta: and priority: keys it carries.
func unshimTLCStep(step *yaml.Node) {
	idx, inner := takeTLC(step)
	if idx < 0 {
		return
	}
	tail := append([]*yaml.Node(nil), step.Content[idx:]...)
	step.Content = append(append(step.Content[:idx], inner...), tail...)
}

// takeTLC removes the x-tlc pair from mapping m and returns where it was
// and the key/value nodes of its value; -1 when m has none.
func takeTLC(m *yaml.Node) (int, []*yaml.Node) {
	if m.Kind != yaml.MappingNode {
		return -1, nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value != extTLC {
			continue
		}
		inner := m.Content[i+1]
		m.Content = append(m.Content[:i:i], m.Content[i+2:]...)
		if inner.Kind != yaml.MappingNode {
			return i, nil
		}
		return i, inner.Content
	}
	return -1, nil
}

// PlainStep is a step as tlc writes it: tags, meta and priority as plain
// keys, never x-tlc. Use it wherever a step is encoded for a person or a
// file.
type PlainStep struct{ RecipeStep }

// MarshalYAML implements yaml.Marshaler.
func (p PlainStep) MarshalYAML() (any, error) {
	var n yaml.Node
	if err := n.Encode(p.RecipeStep); err != nil {
		return nil, fmt.Errorf("encode step %s: %w", p.ID, err)
	}
	unshimTLCSteps(&n)
	return &n, nil
}

// unshimTLCSteps is unshimTLCStep over a step mapping and the steps
// nested in it.
func unshimTLCSteps(step *yaml.Node) {
	unshimTLCStep(step)
	for i := 0; i+1 < len(step.Content); i += 2 {
		if step.Content[i].Value != keySteps || step.Content[i+1].Kind != yaml.SequenceNode {
			continue
		}
		for _, nested := range step.Content[i+1].Content {
			unshimTLCSteps(nested)
		}
	}
}

// MarshalJSON implements json.Marshaler.
func (p PlainStep) MarshalJSON() ([]byte, error) {
	part := tlcPart(p.Ext)
	if len(part) == 0 {
		return p.RecipeStep.MarshalJSON()
	}
	bare := p.RecipeStep
	bare.Ext = dropTLC(bare.Ext)
	b, err := bare.MarshalJSON()
	if err != nil {
		return nil, err
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(b, &obj); err != nil {
		return nil, fmt.Errorf("encode step %s: %w", p.ID, err)
	}
	for k, v := range part {
		if obj[k], err = json.Marshal(v); err != nil {
			return nil, fmt.Errorf("encode step %s: %s: %w", p.ID, k, err)
		}
	}
	return json.Marshal(obj)
}

// PlainSteps wraps steps for encoding.
func PlainSteps(steps []RecipeStep) []PlainStep {
	out := make([]PlainStep, len(steps))
	for i, s := range steps {
		out[i] = PlainStep{s}
	}
	return out
}
