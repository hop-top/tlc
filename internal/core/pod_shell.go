package core

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// MountSpec describes a bind mount for a pod container.
type MountSpec struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Mode   string `json:"mode,omitempty"` // "ro", "rw", or empty (default rw)
}

// PodCreateOpts configures a pod container creation request.
type PodCreateOpts struct {
	Image    string            `json:"image"`
	Mounts   []MountSpec       `json:"mounts,omitempty"`
	EnvVars  map[string]string `json:"env_vars,omitempty"`
	Labels   map[string]string `json:"labels,omitempty"`
	Network  string            `json:"network,omitempty"`
	Provider string            `json:"provider,omitempty"`
}

// PodInfo holds metadata returned by pod create.
type PodInfo struct {
	Name     string `json:"name"`
	ID       string `json:"id"`
	Status   string `json:"status"`
	Provider string `json:"provider"`
}

// CommandRunner abstracts subprocess execution for testability.
type CommandRunner interface {
	// Run executes a command and returns stdout, stderr, exit code, and
	// any execution error (distinct from non-zero exit).
	Run(ctx context.Context, name string, args ...string) (
		stdout string, stderr string, exitCode int, err error,
	)
}

// PodShell wraps the pod CLI binary for container lifecycle management.
// It never imports pod as a library; all interaction is via subprocess.
type PodShell struct {
	runner    CommandRunner
	destroyed bool
}

// NewPodShell returns a PodShell that delegates to runner.
func NewPodShell(runner CommandRunner) *PodShell {
	return &PodShell{runner: runner}
}

// CheckAvailable verifies the pod binary is in PATH and returns a
// supported version.
func (p *PodShell) CheckAvailable(ctx context.Context) error {
	stdout, stderr, code, err := p.runner.Run(ctx, "pod", "--version")
	if err != nil {
		return fmt.Errorf(
			"pod not found; run 'hop install pod' or ensure pod is in PATH",
		)
	}
	if code != 0 {
		return fmt.Errorf(
			"pod version check failed (exit %d): %s",
			code, strings.TrimSpace(stderr),
		)
	}
	if stdout == "" {
		return fmt.Errorf("pod --version returned empty output")
	}
	return nil
}

// Create runs pod create with the given options and parses the JSON
// output into PodInfo.
func (p *PodShell) Create(
	ctx context.Context, opts PodCreateOpts,
) (*PodInfo, error) {
	args := []string{"create", "--format", "json"}

	if opts.Provider != "" {
		args = append(args, "--provider", opts.Provider)
	}
	if opts.Image != "" {
		args = append(args, "--image", opts.Image)
	}
	for _, m := range opts.Mounts {
		spec := m.Source + ":" + m.Target
		if m.Mode != "" {
			spec += ":" + m.Mode
		}
		args = append(args, "--mount", spec)
	}
	for k, v := range opts.EnvVars {
		args = append(args, "--env", k+"="+v)
	}
	for k, v := range opts.Labels {
		args = append(args, "--label", k+"="+v)
	}
	if opts.Network != "" {
		args = append(args, "--network", opts.Network)
	}

	stdout, stderr, code, err := p.runner.Run(ctx, "pod", args...)
	if err != nil {
		return nil, fmt.Errorf("pod create exec failed: %w", err)
	}
	if code != 0 {
		msg := strings.TrimSpace(stderr)
		if code == 137 {
			return nil, fmt.Errorf(
				"container OOM killed (exit 137); add --memory flag",
			)
		}
		return nil, fmt.Errorf("pod create failed (exit %d): %s", code, msg)
	}

	var info PodInfo
	if err := json.Unmarshal([]byte(stdout), &info); err != nil {
		return nil, fmt.Errorf("pod create: failed to parse JSON output: %w", err)
	}
	if info.Name == "" || info.ID == "" || info.Status == "" {
		return nil, fmt.Errorf(
			"unexpected pod output; expected fields: name, id, status",
		)
	}

	p.destroyed = false
	return &info, nil
}

// CopyTo uploads a local file into the pod container.
func (p *PodShell) CopyTo(
	ctx context.Context, podName, localPath, remotePath string,
) error {
	_, stderr, code, err := p.runner.Run(
		ctx, "pod", "cp", localPath, podName+":"+remotePath,
	)
	if err != nil {
		return fmt.Errorf("pod cp exec failed: %w", err)
	}
	if code != 0 {
		return fmt.Errorf(
			"pod cp failed (exit %d): %s", code, strings.TrimSpace(stderr),
		)
	}
	return nil
}

// VerifyFile checks that a file exists inside the pod container.
func (p *PodShell) VerifyFile(
	ctx context.Context, podName, remotePath string,
) error {
	_, stderr, code, err := p.runner.Run(
		ctx, "pod", "exec", podName, "--", "test", "-f", remotePath,
	)
	if err != nil {
		return fmt.Errorf("pod exec test failed: %w", err)
	}
	if code != 0 {
		return fmt.Errorf(
			"context upload failed; file not found in container: %s; %s",
			remotePath, strings.TrimSpace(stderr),
		)
	}
	return nil
}

// Exec runs a command inside the pod and returns stdout, stderr, and
// exit code.
func (p *PodShell) Exec(
	ctx context.Context, podName string, cmd string, cmdArgs ...string,
) (stdout string, stderr string, exitCode int, err error) {
	args := []string{"exec", podName, "--", cmd}
	args = append(args, cmdArgs...)
	return p.runner.Run(ctx, "pod", args...)
}

// CopyFrom downloads a file from the pod container to a local path.
func (p *PodShell) CopyFrom(
	ctx context.Context, podName, remotePath, localPath string,
) error {
	_, stderr, code, err := p.runner.Run(
		ctx, "pod", "cp", podName+":"+remotePath, localPath,
	)
	if err != nil {
		return fmt.Errorf("pod cp exec failed: %w", err)
	}
	if code != 0 {
		return fmt.Errorf(
			"pod cp download failed (exit %d): %s",
			code, strings.TrimSpace(stderr),
		)
	}
	return nil
}

// Destroy tears down the pod container. Idempotent: subsequent calls
// after the first successful destroy are no-ops.
func (p *PodShell) Destroy(ctx context.Context, podName string) error {
	if p.destroyed {
		return nil
	}
	_, stderr, code, err := p.runner.Run(
		ctx, "pod", "destroy", podName, "--yes",
	)
	if err != nil {
		return fmt.Errorf("pod destroy exec failed: %w", err)
	}
	if code != 0 {
		return fmt.Errorf(
			"pod destroy failed (exit %d): %s",
			code, strings.TrimSpace(stderr),
		)
	}
	p.destroyed = true
	return nil
}

// Destroyed reports whether this PodShell has already destroyed its pod.
func (p *PodShell) Destroyed() bool {
	return p.destroyed
}
