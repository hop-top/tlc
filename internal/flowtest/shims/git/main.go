//go:build shimbin

// Package main is the git shim for the flowtest sandbox.
//
// It does NOT use cassettes. Instead it rewrites GIT_DIR and GIT_WORK_TREE to
// point at the sandbox repo, then execs the real git binary so all git
// operations stay isolated to the sandbox without any recording or replay.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func main() {
	workTree := os.Getenv("GIT_WORK_TREE")
	if workTree == "" {
		fmt.Fprintln(os.Stderr, "git-shim: GIT_WORK_TREE is not set")
		os.Exit(1)
	}

	gitDir := workTree + "/.git"

	realGit, err := findReal("git")
	if err != nil {
		fmt.Fprintln(os.Stderr, "git-shim:", err)
		os.Exit(1)
	}

	cmd := exec.Command(realGit, os.Args[1:]...)
	cmd.Env = rewriteEnv(os.Environ(), map[string]string{
		"GIT_DIR":       gitDir,
		"GIT_WORK_TREE": workTree,
	})
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		fmt.Fprintln(os.Stderr, "git-shim:", err)
		os.Exit(1)
	}
}

// findReal finds the real binary named `name` by scanning PATH and skipping
// the directory that contains this shim binary.
func findReal(name string) (string, error) {
	shimDir := filepath.Dir(os.Args[0])
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir == shimDir {
			continue
		}
		p := filepath.Join(dir, name)
		if info, err := os.Stat(p); err == nil && info.Mode()&0o111 != 0 {
			return p, nil
		}
	}
	return "", fmt.Errorf("git: real binary not found in PATH")
}

// rewriteEnv returns a copy of env with the given overrides applied. Existing
// keys in overrides are replaced; new keys are appended.
func rewriteEnv(env []string, overrides map[string]string) []string {
	result := make([]string, 0, len(env)+len(overrides))
	replaced := make(map[string]bool, len(overrides))

	for _, kv := range env {
		key := envKey(kv)
		if val, ok := overrides[key]; ok {
			result = append(result, key+"="+val)
			replaced[key] = true
		} else {
			result = append(result, kv)
		}
	}

	for k, v := range overrides {
		if !replaced[k] {
			result = append(result, k+"="+v)
		}
	}

	return result
}

// envKey returns the key portion of a "KEY=value" env string.
func envKey(kv string) string {
	for i := 0; i < len(kv); i++ {
		if kv[i] == '=' {
			return kv[:i]
		}
	}
	return kv
}
