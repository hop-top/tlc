package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	"hop.top/tlc/internal/core"
)

// newExecPodShell builds the PodShell exec-kind tasks run through under
// --with-pod; tests swap it for one over a scripted pod CLI.
var newExecPodShell = func() *core.PodShell { return core.NewPodShell(&execRunner{}) }

// execDispatcherForMode picks where exec-kind tasks run. The host is the
// default: --with-pod's default value governs agents only, so exec tasks
// move into a pod only when the flag is passed (true or an image) and
// stay on the host under --with-pod=false.
//
// The pod path creates one pod per task from the image — the flag's
// value, else the --agent's — copies the working tree to /workspace and
// runs the argv there; core.PodArgvRunner documents what the protocol
// can and cannot enforce.
func execDispatcherForMode(
	cmd *cobra.Command, registry *core.AgentRegistry, local bool, image string,
) (*core.ExecDispatcher, error) {
	if local || !cmd.Flags().Changed("with-pod") {
		return core.NewExecDispatcher(repoRootForMode(true)), nil
	}
	image, err := execPodImage(registry, image)
	if err != nil {
		return nil, err
	}
	return core.NewPodExecDispatcher(&core.PodArgvRunner{
		Shell:     newExecPodShell(),
		Image:     image,
		Workspace: repoRootForMode(true),
		WorkDir:   repoRootForMode(false),
		Labels:    map[string]string{"tlc.kind": "exec"},
	}), nil
}

// execPodImage resolves the image exec tasks run in: the explicit
// --with-pod=<image>, else the --agent's image from the registry.
func execPodImage(registry *core.AgentRegistry, image string) (string, error) {
	if image != "" {
		return image, nil
	}
	if trackExecuteAgent == "" {
		return "", errors.New("exec tasks under --with-pod need an image; " +
			"pass --with-pod=<image> or --agent <name> whose agents.yaml entry sets image")
	}
	resolved, err := resolveAgentName(registry, trackExecuteAgent)
	if err != nil {
		return "", fmt.Errorf("exec tasks under --with-pod: %w", err)
	}
	cfg, err := registry.Get(resolved)
	if err != nil {
		var trust *core.ErrTrustRequired
		if errors.As(err, &trust) {
			return "", fmt.Errorf("%w; re-run with --trust-project to approve", err)
		}
		return "", fmt.Errorf("exec tasks under --with-pod: agent %s: %w", resolved, err)
	}
	if cfg.Image == "" {
		return "", fmt.Errorf("exec tasks under --with-pod: agent %s has no image; "+
			"set image in agents.yaml or pass --with-pod=<image>", resolved)
	}
	return cfg.Image, nil
}
