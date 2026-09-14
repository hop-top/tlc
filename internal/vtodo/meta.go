package vtodo

import (
	"encoding/json"
	"sort"
	"strings"

	vstar "hop.top/vstar"
	"hop.top/vstar/ext"

	"hop.top/tlc/internal/core"
)

// XPropMeta carries the JSON serialisation of an entity's Meta map.
// Shape follows the ratified vtodo sync spec: a single property whose
// value is a JSON object, rather than one property per key. JSON keeps
// non-string values (numbers, bools, nested maps, arrays) round-tripping
// with their Go types intact, which a flat X-TLC-META-<KEY> family
// cannot do without inventing a type-tagging convention.
const XPropMeta = "X-TLC-META"

// MetaXTLCKey is the Meta key under which unrecognised X-TLC-*
// properties from an incoming calendar are parked. The value is a
// map[string]interface{} of full property name → raw string value, so
// nothing an unknown producer wrote is lost across an import/export
// cycle.
const MetaXTLCKey = "x_tlc"

// tlcSystemSlug is the ext.SystemName() owner slug for the X-TLC-*
// extension namespace. ext classifies X-TLC-EFFORT and friends as
// ext.ScopeSystem with this owner.
const tlcSystemSlug = "TLC"

// derivedMetaKeys are Meta entries reconstructed from other wire
// constructs on decode, so re-emitting them inside X-TLC-META would
// duplicate state and corrupt round-trip stability.
//
//   - blocked_by      → RELATED-TO;RELTYPE=DEPENDS-ON rows
//   - external_uid    → the UID property itself
//   - x_tlc           → re-emitted as real X-TLC-* properties; carrying
//     it inside the JSON too would double it on every cycle.
//   - priority_source, priority_rule → XPropPrioritySource and
//     XPropPriorityRule, the typed properties decode reads them back
//     from; inside the JSON as well they were written twice and the
//     JSON copy silently won on import.
//   - effective_status → X-VSTAR-EFFECTIVE-STATUS on a supersession
//     journal, the property decode preserved it from.
//   - last_sync_hash → nothing on the wire. It is the sync layer's
//     change-detection baseline for this store, meaningful only
//     against the local row; exported, it would make a task's hash
//     move on every sync and tell a foreign reader nothing.
var derivedMetaKeys = map[string]bool{
	"blocked_by":            true,
	"external_uid":          true,
	MetaXTLCKey:             true,
	core.MetaPrioritySource: true,
	core.MetaPriorityRule:   true,
	MetaEffectiveStatusKey:  true,
	core.MetaLastSyncHash:   true,
}

// knownXProps is the set of X-TLC-* names that already populate a typed
// field on the model. Everything else found on the wire is unknown and
// gets preserved under MetaXTLCKey.
var knownXProps = map[string]bool{
	XPropEffort:    true,
	XPropAssignee:  true,
	XPropProjectID: true,
	XPropTaskSeq:   true,
	XPropTrackSlug: true,
	XPropTrackType: true,
	XPropTrackKind: true,
	XPropTrackSeq:  true,
	XPropLogAction: true,
	XPropLogBy:     true,
	XPropLogTaskID: true,
	XPropMeta:      true,

	XPropPriority:       true,
	XPropPrioritySource: true,
	XPropPriorityRule:   true,

	XPropStatus:      true,
	XPropTrackStatus: true,
	XPropArchived:    true,

	// Derived at encode from the entity (TrackID, Action), never
	// carried on the model; registered so the wire copy is not parked
	// and re-emitted beside the encoder's own.
	XPropConcept: true,
}

// metaProperty renders the non-derived entries of meta as a single
// X-TLC-META property. Returns ok=false when there is nothing to emit,
// so callers skip the property entirely rather than writing "{}".
//
// Keys are emitted in sorted order (encoding/json sorts map keys), so
// the wire form is deterministic across runs.
func metaProperty(meta map[string]interface{}) (vstar.Property, bool) {
	if len(meta) == 0 {
		return vstar.Property{}, false
	}
	out := make(map[string]interface{}, len(meta))
	for k, v := range meta {
		if derivedMetaKeys[k] {
			continue
		}
		out[k] = v
	}
	if len(out) == 0 {
		return vstar.Property{}, false
	}
	raw, err := json.Marshal(out)
	if err != nil {
		// A Meta value that will not marshal (channel, func, NaN, …)
		// must not take the whole export down. Fall back to a
		// best-effort map of the keys that do marshal.
		salvaged := make(map[string]interface{}, len(out))
		for k, v := range out {
			if _, err := json.Marshal(v); err == nil {
				salvaged[k] = v
			}
		}
		if len(salvaged) == 0 {
			return vstar.Property{}, false
		}
		raw, err = json.Marshal(salvaged)
		if err != nil {
			return vstar.Property{}, false
		}
	}
	return vstar.Property{Name: XPropMeta, Value: string(raw)}, true
}

// addMeta appends the X-TLC-META property to c when meta has content.
func addMeta(c *vstar.Component, meta map[string]interface{}) {
	if p, ok := metaProperty(meta); ok {
		c.Add(p)
	}
}

// tlcExtensions returns every X-TLC-* property on c, using vstar's ext
// classifier rather than a hand-maintained name list. Deliberately
// scoped to the TLC owner slug: extensions belonging to other systems
// are handled by foreignExtensions.
//
// ext.ExtensionsByScope does not recurse into c.Sub; sub-components
// (VALARM) are walked explicitly by the callers that need them.
func tlcExtensions(c vstar.Component) []vstar.Property {
	var out []vstar.Property
	for _, p := range ext.ExtensionsByScope(c, ext.ScopeSystem) {
		if slug, ok := ext.SystemName(p.Name); ok && slug == tlcSystemSlug {
			out = append(out, p)
		}
	}
	return out
}

// unknownTLCExtensions returns the X-TLC-* properties on c that no typed
// field consumes. These are what decode must preserve instead of drop.
func unknownTLCExtensions(c vstar.Component) []vstar.Property {
	var out []vstar.Property
	for _, p := range tlcExtensions(c) {
		if knownXProps[strings.ToUpper(p.Name)] {
			continue
		}
		out = append(out, p)
	}
	return out
}

// applyMeta merges the wire Meta state of component c into dst: first the
// JSON payload of X-TLC-META, then any unrecognised X-TLC-* property
// under the MetaXTLCKey sub-map.
//
// dst is returned so callers can assign the result back — the map is
// allocated lazily and stays nil when the component carries no Meta at
// all, keeping entities that never had Meta serialising exactly as
// before.
func applyMeta(dst map[string]interface{}, c vstar.Component) map[string]interface{} {
	if p, ok := c.Get(XPropMeta); ok {
		var decoded map[string]interface{}
		if err := json.Unmarshal([]byte(p.Value), &decoded); err == nil {
			for k, v := range decoded {
				if derivedMetaKeys[k] {
					// Never let wire Meta clobber state that decode
					// rebuilds from UID / RELATED-TO.
					continue
				}
				if dst == nil {
					dst = map[string]interface{}{}
				}
				dst[k] = v
			}
		}
	}

	unknown := unknownTLCExtensions(c)
	if len(unknown) == 0 {
		return dst
	}
	if dst == nil {
		dst = map[string]interface{}{}
	}
	bag, _ := dst[MetaXTLCKey].(map[string]interface{})
	if bag == nil {
		bag = map[string]interface{}{}
	}
	for _, p := range unknown {
		bag[strings.ToUpper(p.Name)] = p.Value
	}
	dst[MetaXTLCKey] = bag
	return dst
}

// addUnknownTLCProps re-emits the properties parked under MetaXTLCKey as
// real X-TLC-* properties, closing the import→export half of the
// round-trip. Names are sorted for deterministic output.
func addUnknownTLCProps(c *vstar.Component, meta map[string]interface{}) {
	bag, ok := meta[MetaXTLCKey].(map[string]interface{})
	if !ok || len(bag) == 0 {
		return
	}
	names := make([]string, 0, len(bag))
	for name := range bag {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if !ext.IsExtension(name) {
			continue
		}
		slug, ok := ext.SystemName(name)
		if !ok || slug != tlcSystemSlug {
			continue
		}
		if knownXProps[strings.ToUpper(name)] {
			continue
		}
		s, ok := bag[name].(string)
		if !ok {
			continue
		}
		c.Add(vstar.Property{Name: strings.ToUpper(name), Value: s})
	}
}
