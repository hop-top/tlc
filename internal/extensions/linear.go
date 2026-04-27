package extensions

import (
	"hop.top/kit/go/ai/ext"
	"hop.top/tlc/internal/auth"
)

// LinearSync is the ext.Extension for Linear issue synchronisation.
// Capabilities:
//   - CapRegistry — registers as a sync provider
//   - CapHook     — stub; bus not available until Track C
//   - CapConfig   — auth.linear config section
type LinearSync struct{ syncExtBase }

// NewLinearSync returns a Linear sync extension backed by the given store.
func NewLinearSync(store auth.Store) *LinearSync {
	return &LinearSync{syncExtBase{
		meta: ext.Metadata{
			Name:        "linear-sync",
			Version:     "0.1.0",
			Description: "Linear issue synchronisation",
		},
		caps:  defaultCaps,
		store: store,
		svc:   "linear",
	}}
}
