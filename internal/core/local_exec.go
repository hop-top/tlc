package core

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// Env vars every locally executed agent receives: the context file tlc
// wrote for it and the results file tlc reads back. Container runs export
// the same two names for their /workspace paths, so an agent never has to
// guess either path from its working directory.
const (
	EnvContextPath = "TLC_CONTEXT_PATH"
	EnvResultsPath = "TLC_RESULTS_PATH"
)

// LocalExecManager executes an agent binary directly on the host,
// bypassing container isolation. It implements the same result contract
// as PodShell (stdout JSON status + results file) and announces the
// contract's file paths through EnvContextPath and EnvResultsPath.
type LocalExecManager struct {
	runner      CommandRunner
	resultsPath string // path to results.json (default: .tlc/results.json)
}

// LocalExecOpts configures a local agent execution.
type LocalExecOpts struct {
	Binary      string            // agent binary name or path
	Args        []string          // extra args for the binary
	EnvVars     map[string]string // injected env vars
	RepoRoot    string            // working directory for exec
	ResultPath  string            // override results file path
	ContextPath string            // context file the agent reads; exported as EnvContextPath when set
}

// NewLocalExecManager returns a LocalExecManager that delegates to runner.
// resultsPath is the default path to the results file; if empty,
// ".tlc/results.json" is used relative to the repo root.
func NewLocalExecManager(runner CommandRunner, resultsPath string) *LocalExecManager {
	if resultsPath == "" {
		resultsPath = filepath.Join(".tlc", "results.json")
	}
	return &LocalExecManager{
		runner:      runner,
		resultsPath: resultsPath,
	}
}

// WarnIgnoredFlags logs warnings for container-specific flags that
// have no effect in local mode.
func WarnIgnoredFlags(flags map[string]bool) {
	for flag, set := range flags {
		if set {
			log.Printf("warning: --%s ignored in --local mode", flag)
		}
	}
}

// Exec runs the agent binary locally with injected env vars and
// working directory, returning stdout, stderr, and exit code.
func (m *LocalExecManager) Exec(
	ctx context.Context, opts LocalExecOpts,
) (stdout string, stderr string, exitCode int, err error) {
	if opts.Binary == "" {
		return "", "", 1, fmt.Errorf(
			"agent binary not specified; set --agent or configure agents.yaml",
		)
	}

	cmd := exec.CommandContext(ctx, opts.Binary, opts.Args...)

	// Set working directory if specified.
	if opts.RepoRoot != "" {
		cmd.Dir = opts.RepoRoot
	}

	// Inherit the current env, overlay opts.EnvVars, then announce the
	// protocol paths last so they win over anything inherited.
	env := os.Environ()
	for k, v := range opts.EnvVars {
		env = append(env, k+"="+v)
	}
	env = append(env, EnvResultsPath+"="+m.ResultsFilePath(opts.RepoRoot))
	if opts.ContextPath != "" {
		env = append(env, EnvContextPath+"="+opts.ContextPath)
	}
	cmd.Env = env

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	runErr := cmd.Run()
	stdout = outBuf.String()
	stderr = errBuf.String()

	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			if ws, ok := exitErr.Sys().(syscall.WaitStatus); ok {
				exitCode = ws.ExitStatus()
			} else {
				exitCode = exitErr.ExitCode()
			}
			runErr = nil // non-zero exit is not an execution error
		}
	}
	err = runErr
	return
}

// ResultsFilePath returns the absolute path to the results file,
// resolved against repoRoot.
func (m *LocalExecManager) ResultsFilePath(repoRoot string) string {
	if filepath.IsAbs(m.resultsPath) {
		return m.resultsPath
	}
	return filepath.Join(repoRoot, m.resultsPath)
}

// ReadResult reads and parses the results file. Returns nil result
// (not error) if the file does not exist.
func (m *LocalExecManager) ReadResult(repoRoot string) (*AgentResult, error) {
	path := m.ResultsFilePath(repoRoot)

	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("stat results file: %w", err)
	}

	const maxSize = 50 * 1024 * 1024 // 50 MB
	if info.Size() > maxSize {
		return nil, fmt.Errorf(
			"results file too large (%d bytes, max %d); "+
				"reduce agent output or split artifacts",
			info.Size(), maxSize,
		)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read results file: %w", err)
	}

	var result AgentResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf(
			"parse results file: %w; raw content preserved at %s",
			err, path,
		)
	}

	return &result, nil
}

// ParseStdoutResult extracts the last JSON line from stdout and parses
// it as an AgentResult (status signal).
func ParseStdoutResult(stdout string) (*AgentResult, error) {
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) == 0 {
		return nil, fmt.Errorf("agent produced no stdout output")
	}

	// Find last JSON line (may have non-JSON log lines before).
	var lastJSON string
	for i := len(lines) - 1; i >= 0; i-- {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "{") {
			lastJSON = trimmed
			break
		}
	}
	if lastJSON == "" {
		return nil, fmt.Errorf(
			"no JSON status line found in agent stdout",
		)
	}

	var result AgentResult
	if err := json.Unmarshal([]byte(lastJSON), &result); err != nil {
		return nil, fmt.Errorf("parse stdout JSON: %w", err)
	}

	return &result, nil
}
