package flowtest

import (
	"errors"
	"fmt"
	"slices"

	"gopkg.in/yaml.v3"
)

// Gate decisions a run can script.
const (
	GateApprove = "approve"
	GateReject  = "reject"
)

// GateDecision is the scripted decision on one human step.
//
// In test.yaml it is either a bare decision, which only approve may be, or
// a mapping:
//
//	gates:
//	  review: approve
//	  sign-off:
//	    decision: reject
//	    by: lead
//	    reason: not ready
type GateDecision struct {
	Decision string `yaml:"decision"`
	By       string `yaml:"by,omitempty"`
	Note     string `yaml:"note,omitempty"`   // approve only
	Reason   string `yaml:"reason,omitempty"` // reject only, required
}

// gateSet is the `gates:` mapping of a manifest, step id to decision. It
// decodes itself because yaml.v3 applies strict (unknown-key) decoding only
// to the decoder's own structs, not to what a custom unmarshaler decodes.
type gateSet map[string]GateDecision

// UnmarshalYAML implements yaml.Unmarshaler.
func (g *gateSet) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode && node.Tag == "!!null" {
		return nil
	}
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("line %d: gates must map step ids to decisions", node.Line)
	}
	out := make(gateSet, len(node.Content)/2)
	for i := 0; i+1 < len(node.Content); i += 2 {
		id := node.Content[i].Value
		if _, dup := out[id]; dup {
			return fmt.Errorf("line %d: gate %q declared twice", node.Content[i].Line, id)
		}
		d, err := decodeGate(node.Content[i+1])
		if err != nil {
			return fmt.Errorf("gate %q: %w", id, err)
		}
		if err := d.validate(); err != nil {
			return fmt.Errorf("gate %q: %w", id, err)
		}
		out[id] = d
	}
	*g = out
	return nil
}

var gateKeys = []string{"decision", "by", "note", "reason"}

func decodeGate(node *yaml.Node) (GateDecision, error) {
	switch node.Kind {
	case yaml.ScalarNode:
		return GateDecision{Decision: node.Value}, nil
	case yaml.MappingNode:
		for i := 0; i+1 < len(node.Content); i += 2 {
			if key := node.Content[i].Value; !slices.Contains(gateKeys, key) {
				return GateDecision{}, fmt.Errorf("line %d: unknown key %q (want one of %v)", node.Content[i].Line, key, gateKeys)
			}
		}
		var d GateDecision
		if err := node.Decode(&d); err != nil {
			return GateDecision{}, fmt.Errorf("decode: %w", err)
		}
		return d, nil
	}
	return GateDecision{}, errors.New("want approve, reject or a mapping with a decision")
}

// validate checks a decision on its own: its value and the fields that go
// with it.
func (d GateDecision) validate() error {
	switch d.Decision {
	case GateApprove:
		if d.Reason != "" {
			return errors.New("reason belongs to reject; an approval takes note")
		}
	case GateReject:
		if d.Reason == "" {
			return errors.New("reject needs a reason: use the mapping form with decision: reject and reason:")
		}
		if d.Note != "" {
			return errors.New("note belongs to approve; a rejection takes reason")
		}
	case "":
		return errors.New("decision is required (approve or reject)")
	default:
		return fmt.Errorf("decision %q: want approve or reject", d.Decision)
	}
	return nil
}
