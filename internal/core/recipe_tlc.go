package core

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v3"

	"hop.top/flo/spec"
)

// tlc-owned recipe keys. The recipe model comes from hop.top/flo/spec,
// which knows nothing of tracks, tags, meta or priority, so tlc carries
// them in the spec's extension slot:
//
//	flow root   track:                        -> Ext["x-tlc"]["track"]
//	each step   tags: / meta: / priority:     -> Ext["x-tlc"][<key>]
//
// Authors write the plain keys; recipe_compat.go moves them under x-tlc
// before spec parses a document and unshimTLCNode moves them back before
// one is written. Everything that reads the values goes through the
// accessors below, so the extension layout lives in this file only.

// extTLC is the single extension key tlc owns; a recipe may not write it.
const extTLC = spec.ExtPrefix + "tlc"

// tlcInflight prefixes the temporary extension keys the document shim
// renames the plain keys to. A rename keeps the key on its own line, so
// the line numbers in spec's errors stay true to the file; foldTLCExt
// merges the renamed keys into Ext["x-tlc"] right after the parse.
const tlcInflight = extTLC + "."

// Plain tlc keys.
const (
	keyTrack    = "track"
	keyTags     = "tags"
	keyMeta     = "meta"
	keyPriority = "priority"
)

var tlcStepKeys = []string{keyTags, keyMeta, keyPriority}

// RecipeTrack is the recipe's track block: what `track create --recipe`
// builds the track from.
type RecipeTrack struct {
	Title string         `yaml:"title,omitempty" json:"title,omitempty"`
	Type  string         `yaml:"type,omitempty" json:"type,omitempty"`
	Plan  string         `yaml:"plan,omitempty" json:"plan,omitempty"`
	Meta  map[string]any `yaml:"meta,omitempty" json:"meta,omitempty"`
}

// StepExtras are the tlc-owned fields of one step: they land on the
// task the step materializes.
type StepExtras struct {
	Tags     []string       `yaml:"tags,omitempty" json:"tags,omitempty"`
	Meta     map[string]any `yaml:"meta,omitempty" json:"meta,omitempty"`
	Priority string         `yaml:"priority,omitempty" json:"priority,omitempty"`
}

// IsZero reports whether no field is set.
func (x StepExtras) IsZero() bool { return len(x.Tags) == 0 && len(x.Meta) == 0 && x.Priority == "" }

// RecipeTrackOf returns the recipe's track block, nil when it has none.
func RecipeTrackOf(r *Recipe) (*RecipeTrack, error) {
	raw, ok := tlcPart(r.Ext)[keyTrack]
	if !ok {
		return nil, nil
	}
	var t RecipeTrack
	if err := decodeTLC(raw, &t); err != nil {
		return nil, fmt.Errorf("recipe %s: track: %w", r.Name, err)
	}
	return &t, nil
}

// SetRecipeTrack stores t as the recipe's track block; nil removes it.
func SetRecipeTrack(r *Recipe, t *RecipeTrack) error {
	if t == nil {
		r.Ext = dropTLCKey(r.Ext, keyTrack)
		return nil
	}
	v, err := encodeTLC(t)
	if err != nil {
		return fmt.Errorf("recipe %s: track: %w", r.Name, err)
	}
	r.Ext = setTLCKey(r.Ext, keyTrack, v)
	return nil
}

// StepExtrasOf returns the tlc-owned fields of a step, as rendered when
// the step came out of RenderStep.
func StepExtrasOf(s RecipeStep) (StepExtras, error) {
	var x StepExtras
	if part := tlcPart(s.Ext); len(part) > 0 {
		if err := decodeTLC(part, &x); err != nil {
			return StepExtras{}, fmt.Errorf("step %s: %w", s.ID, err)
		}
	}
	return x, nil
}

// SetStepExtras stores x on the step; the zero value removes them.
func SetStepExtras(s *RecipeStep, x StepExtras) error {
	s.Ext = dropTLC(s.Ext)
	if x.IsZero() {
		return nil
	}
	v, err := encodeTLC(x)
	if err != nil {
		return fmt.Errorf("step %s: %w", s.ID, err)
	}
	part, ok := v.(map[string]any)
	if !ok {
		return fmt.Errorf("step %s: tlc fields did not encode to a map", s.ID)
	}
	if s.Ext == nil {
		s.Ext = map[string]any{}
	}
	s.Ext[extTLC] = part
	return nil
}

// RenderRecipeMap renders every string leaf of m against scope.
func RenderRecipeMap(m map[string]any, scope Scope) (map[string]any, error) {
	out, err := spec.RenderMap(m, scope)
	return out, recipeWording(err)
}

func tlcPart(ext map[string]any) map[string]any {
	if part, ok := ext[extTLC].(map[string]any); ok {
		return part
	}
	return nil
}

func dropTLC(ext map[string]any) map[string]any {
	if _, ok := ext[extTLC]; !ok {
		return ext
	}
	out := make(map[string]any, len(ext))
	for k, v := range ext {
		if k != extTLC {
			out[k] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func setTLCKey(ext map[string]any, key string, v any) map[string]any {
	out := make(map[string]any, len(ext)+1)
	for k, e := range ext {
		out[k] = e
	}
	part := map[string]any{}
	for k, e := range tlcPart(ext) {
		part[k] = e
	}
	part[key] = v
	out[extTLC] = part
	return out
}

func dropTLCKey(ext map[string]any, key string) map[string]any {
	part := tlcPart(ext)
	if _, ok := part[key]; !ok {
		return ext
	}
	rest := map[string]any{}
	for k, v := range part {
		if k != key {
			rest[k] = v
		}
	}
	out := dropTLC(ext)
	if len(rest) == 0 {
		return out
	}
	if out == nil {
		out = map[string]any{}
	}
	out[extTLC] = rest
	return out
}

// decodeTLC decodes v, an Ext value, into out, refusing unknown keys.
func decodeTLC(v any, out any) error {
	b, err := yaml.Marshal(v)
	if err != nil {
		return fmt.Errorf("encode: %w", err)
	}
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(out); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("%w", err)
	}
	return nil
}

// encodeTLC turns a typed value into the generic maps and lists Ext holds.
func encodeTLC(v any) (any, error) {
	b, err := yaml.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("encode: %w", err)
	}
	var out any
	if err := yaml.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf("encode: %w", err)
	}
	return out, nil
}

// foldTLCExt merges the in-flight x-tlc.<key> extension keys of a parsed
// recipe into Ext["x-tlc"], on the root and on every step at any depth.
func foldTLCExt(r *Recipe) {
	r.Ext = foldExt(r.Ext)
	foldStepExt(r.Steps)
}

func foldStepExt(steps []RecipeStep) {
	for i := range steps {
		steps[i].Ext = foldExt(steps[i].Ext)
		foldStepExt(steps[i].Steps)
	}
}

func foldExt(ext map[string]any) map[string]any {
	var part map[string]any
	for k, v := range ext {
		key, ok := strings.CutPrefix(k, tlcInflight)
		if !ok {
			continue
		}
		if part == nil {
			part = map[string]any{}
		}
		part[key] = v
		delete(ext, k)
	}
	if part == nil {
		return ext
	}
	if ext == nil {
		ext = map[string]any{}
	}
	ext[extTLC] = part
	return ext
}

// finishTLC completes a recipe spec has parsed from a shimmed document:
// it folds the in-flight keys into Ext["x-tlc"] and checks them. Every
// path that parses a recipe ends here.
func finishTLC(r *Recipe) error {
	foldTLCExt(r)
	return validateTLC(r)
}

// validateTLC checks the tlc-owned keys of a recipe and of every step at
// any depth: shapes, and meta keys tlc reserves for itself. Values that
// are still templates are checked after rendering, at materialization.
func validateTLC(r *Recipe) error {
	t, err := RecipeTrackOf(r)
	if err != nil {
		return err
	}
	if t != nil {
		if err := ValidateTrackMetaWrite(t.Meta, nil); err != nil {
			return fmt.Errorf("recipe %s: track.meta: %w", r.Name, err)
		}
	}
	return validateStepsTLC(r.Name, r.Steps)
}

func validateStepsTLC(recipe string, steps []RecipeStep) error {
	for _, s := range steps {
		if _, err := checkStepExtras(s); err != nil {
			return fmt.Errorf("recipe %s: %w", recipe, err)
		}
		if err := validateStepsTLC(recipe, s.Steps); err != nil {
			return err
		}
	}
	return nil
}

// checkStepExtras decodes a step's extras and applies the checks that do
// not depend on config: the reserved meta keys.
func checkStepExtras(s RecipeStep) (StepExtras, error) {
	x, err := StepExtrasOf(s)
	if err != nil {
		return x, err
	}
	if err := ValidateUserMetaWrite(x.Meta, nil); err != nil {
		return x, fmt.Errorf("step %s: meta: %w", s.ID, err)
	}
	return x, nil
}
