package core

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// ParseRecipe decodes a recipe document and validates it. JSON is selected
// by a .json filename; anything else is YAML. Unknown keys are errors so a
// misspelled field never silently drops out of a task. Validation always
// runs — there is no extension that skips it.
func ParseRecipe(r io.Reader, filename string) (*Recipe, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read recipe %s: %w", filename, err)
	}
	var rec Recipe
	if strings.EqualFold(filepath.Ext(filename), extJSON) {
		err = decodeRecipeJSON(data, &rec)
	} else {
		err = decodeRecipeYAML(data, &rec)
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", filename, err)
	}
	rec.Path = filename
	rec.Hash = recipeHash(data)
	for i := range rec.Steps {
		rec.Steps[i].Ordinal = i + 1
	}
	if err := ValidateRecipe(&rec); err != nil {
		return nil, fmt.Errorf("%s: %w", filename, err)
	}
	return &rec, nil
}

// recipeHash fingerprints the raw document so drift between a recipe and
// the tasks it produced is detectable even without a version bump.
func recipeHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func decodeRecipeYAML(data []byte, rec *Recipe) error {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("invalid yaml: %w", err)
	}
	root := &doc
	if root.Kind == yaml.DocumentNode && len(root.Content) > 0 {
		root = root.Content[0]
	}
	if root.Kind != 0 && root.Kind != yaml.MappingNode {
		return fmt.Errorf("a recipe is a mapping with recipe, version and steps keys")
	}
	if err := rejectFlowShapeYAML(root); err != nil {
		return err
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(rec); err != nil && err != io.EOF { //nolint:errorlint // yaml returns the sentinel itself
		return fmt.Errorf("invalid recipe: %w", err)
	}
	return nil
}

// rejectFlowShapeYAML refuses the flow format's header keys and map-form
// steps with a pointer to the recipe format, instead of decoding them into
// an empty recipe.
func rejectFlowShapeYAML(root *yaml.Node) error {
	for i := 0; i+1 < len(root.Content); i += 2 {
		key, val := root.Content[i], root.Content[i+1]
		if err := rejectFlowKey(key.Value, val.Kind == yaml.MappingNode); err != nil {
			return fmt.Errorf("line %d: %w", key.Line, err)
		}
	}
	return nil
}

// Header keys of the retired flow format, refused with a hint.
const (
	flowKeyID    = "flow_id"
	flowKeyEntry = "entry_step"
	flowKeyFlow  = "flow"
)

func rejectFlowKey(key string, stepsIsMap bool) error {
	switch key {
	case flowKeyID, flowKeyEntry, flowKeyFlow:
		return fmt.Errorf("%q is a flow key; flows are now recipes: name the document with \"recipe:\" and list its steps under \"steps:\"", key)
	case nsSteps:
		if stepsIsMap {
			return fmt.Errorf("steps must be a list of steps with ids, not a map; flows are now recipes")
		}
	}
	return nil
}

func decodeRecipeJSON(data []byte, rec *Recipe) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("invalid json: %w", err)
	}
	for key, val := range raw {
		trimmed := bytes.TrimSpace(val)
		if err := rejectFlowKey(key, len(trimmed) > 0 && trimmed[0] == '{'); err != nil {
			return err
		}
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(rec); err != nil {
		return fmt.Errorf("invalid recipe: %w", err)
	}
	return nil
}
