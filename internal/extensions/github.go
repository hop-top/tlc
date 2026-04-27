package extensions

import (
	"hop.top/kit/go/ai/ext"
	"hop.top/tlc/internal/auth"
)

// GitHubSync is the ext.Extension for GitHub issue synchronisation.
// Capabilities:
//   - CapRegistry — registers as a sync provider
//   - CapHook     — stub; bus not available until Track C
//   - CapConfig   — auth.github config section
type GitHubSync struct{ syncExtBase }

// NewGitHubSync returns a GitHub sync extension backed by the given store.
func NewGitHubSync(store auth.Store) *GitHubSync {
	return &GitHubSync{syncExtBase{
		meta: ext.Metadata{
			Name:        "github-sync",
			Version:     "0.1.0",
			Description: "GitHub issue synchronisation",
		},
		caps:  defaultCaps,
		store: store,
		svc:   "github",
	}}
}
