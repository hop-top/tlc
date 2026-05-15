package cli

import (
	"testing"
)

// TestStrictValidationPasses guards the kit 12fcc conformance contract:
// every cobra leaf reachable from RootCmd must carry the required
// annotations (kit/side-effect, kit/idempotent), a Long description on
// non-trivial commands, and the structural rules kit checks in
// Root.Validate. If anyone adds a new command without proper annotations,
// this test fails before merge.
//
// kit.Config.EnforceValidate defaults to true (see kit/go/console/cli/cli.go),
// so Execute() already runs this check at boot. We invoke it directly so a
// regression surfaces as a test failure rather than only at binary startup.
//
// See docs/12fcc-conformance-split-plan.md for the contract.
func TestStrictValidationPasses(t *testing.T) {
	// kitRootInstance is the same Root that cmd/tlc/main.go drives via
	// cli.Execute(). All subcommand init() funcs have registered against
	// RootCmd by the time the test binary's package init completes, so the
	// tree we validate here matches the production tree.
	if err := kitRootInstance.Validate(); err != nil {
		t.Fatalf("kit Root.Validate failed: %v", err)
	}
}
