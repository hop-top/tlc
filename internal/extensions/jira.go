package extensions

import (
	"hop.top/kit/ext"
	"hop.top/tlc/internal/auth"
)

// JiraSync is the ext.Extension for Jira issue synchronisation.
// Capabilities:
//   - CapRegistry — registers as a sync provider
//   - CapHook     — stub; bus not available until Track C
//   - CapConfig   — auth.jira config section
type JiraSync struct{ syncExtBase }

// NewJiraSync returns a Jira sync extension backed by the given store.
func NewJiraSync(store auth.Store) *JiraSync {
	return &JiraSync{syncExtBase{
		meta: ext.Metadata{
			Name:        "jira-sync",
			Version:     "0.1.0",
			Description: "Jira issue synchronisation",
		},
		caps:  defaultCaps,
		store: store,
		svc:   "jira",
	}}
}
