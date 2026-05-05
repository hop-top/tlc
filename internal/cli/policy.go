package cli

import (
	"context"
	"fmt"
	"os"
	"sync"

	"hop.top/kit/go/runtime/bus"
	"hop.top/kit/go/runtime/policy"
	"hop.top/kit/go/runtime/policy/withcel"
	"hop.top/tlc/internal/config"
)

// policyEng is the process-wide policy.Engine, initialised on first
// PersistentPreRunE invocation.  Nil until kit/runtime/policy has been
// wired (or wiring failed — tests bypass the bootstrap).
var (
	policyEng    *policy.Engine
	policyUnwire func()
	policyOnce   sync.Once
	policyErr    error
)

// initPolicyEngine loads the policy YAML, builds a CEL-backed engine,
// and wires the engine to the supplied bus.  Idempotent: subsequent
// calls return the previously-constructed engine (or the captured
// boot error).  Misconfig fails loud — the caller surfaces the error
// from PersistentPreRunE so the CLI exits before running the user's
// command.
//
// Resolution order for the YAML source:
//
//  1. $TLC_POLICY_FILE if set (lets tests / CI override).
//  2. $XDG_CONFIG_HOME/tlc/policies.yaml (config.PoliciesPath()),
//     seeded from the bundled default on first boot when missing.
//
// The user file is only auto-seeded when absent or empty so adopter-
// authored rules are never clobbered.
func initPolicyEngine(b bus.Bus) (*policy.Engine, error) {
	policyOnce.Do(func() {
		cfg, err := loadPolicyConfig()
		if err != nil {
			policyErr = err
			return
		}
		eng, err := withcel.New(cfg, policy.WithPrincipalResolver(tlcPrincipalResolver))
		if err != nil {
			policyErr = fmt.Errorf("policy: build engine: %w", err)
			return
		}
		policyEng = eng
		if b != nil {
			policyUnwire = policy.Wire(b, eng)
		}
	})
	return policyEng, policyErr
}

// loadPolicyConfig resolves the policy YAML source and returns the
// parsed Config.  An empty bundled fallback (no policies) is never
// produced — when the user file is missing or empty we seed it with
// the bundled default and load that.
func loadPolicyConfig() (*policy.Config, error) {
	if path := os.Getenv("TLC_POLICY_FILE"); path != "" {
		cfg, err := policy.LoadConfig(path)
		if err != nil {
			return nil, fmt.Errorf("policy: load %s: %w", path, err)
		}
		return cfg, nil
	}

	path, err := config.EnsureDefaultPoliciesFile()
	if err != nil {
		// Couldn't write the user file (permissions, FS error).  Fall
		// back to parsing the embedded default so policy enforcement
		// still applies — adopters notice when they try to edit and
		// the file isn't there.
		cfg, perr := policy.ParseConfig(config.DefaultPoliciesYAML())
		if perr != nil {
			return nil, fmt.Errorf("policy: parse bundled default after seed failure %v: %w", err, perr)
		}
		return cfg, nil
	}
	cfg, err := policy.LoadConfig(path)
	if err != nil {
		return nil, fmt.Errorf("policy: load %s: %w", path, err)
	}
	return cfg, nil
}

// tlcPrincipalResolver picks principal from ctx → KIT_POLICY_ROLE env
// → $USER.  Aps profile lookup is a deliberate follow-up — kit cannot
// import aps directly and the wsm/aps integration is not yet wired
// through tlc's CLI.  The default kit resolver suffices today;
// tlcPrincipalResolver exists as the seam where aps resolution lands.
func tlcPrincipalResolver(ctx context.Context) policy.Principal {
	return policy.DefaultPrincipalResolver(ctx)
}

// closePolicy unsubscribes the engine handlers.  Called from Execute's
// shutdown defer, before the bus is closed, so the unsubscribe is a
// no-op against a still-open bus.
func closePolicy() {
	if policyUnwire != nil {
		policyUnwire()
		policyUnwire = nil
	}
}

// policyOnceReset clears the bootstrap state so tests can re-run
// initPolicyEngine in a single process.  Test-only helper.
func policyOnceReset() {
	if policyUnwire != nil {
		policyUnwire()
		policyUnwire = nil
	}
	policyEng = nil
	policyErr = nil
	policyOnce = sync.Once{}
}
