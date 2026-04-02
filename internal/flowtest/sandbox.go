package flowtest

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	xrr "hop.top/xrr"
)

// Sandbox is a hermetic test environment for one flow test run.
type Sandbox struct {
	RootDir string
	RepoDir string
	BinDir  string
	HomeDir string
	Keep    bool
}

// NewSandbox creates tmp/tlc-flow-test-<uuid>/{repo,bin,home} and
// git clone --local <cwd> into repo/.
func NewSandbox(cwd string) (*Sandbox, error) {
	root, err := os.MkdirTemp("", "tlc-flow-test-*")
	if err != nil {
		return nil, fmt.Errorf("flowtest: create sandbox root: %w", err)
	}

	sb := &Sandbox{
		RootDir: root,
		RepoDir: filepath.Join(root, "repo"),
		BinDir:  filepath.Join(root, "bin"),
		HomeDir: filepath.Join(root, "home"),
	}

	for _, dir := range []string{sb.RepoDir, sb.BinDir, sb.HomeDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			_ = os.RemoveAll(root)
			return nil, fmt.Errorf("flowtest: create sandbox dir %s: %w", dir, err)
		}
	}

	// git clone --local <cwd> into repo/
	cloneCmd := exec.Command("git", "clone", "--local", cwd, sb.RepoDir)
	cloneCmd.Stdout = os.Stderr
	cloneCmd.Stderr = os.Stderr
	if err := cloneCmd.Run(); err != nil {
		_ = os.RemoveAll(root)
		return nil, fmt.Errorf("flowtest: git clone sandbox repo: %w", err)
	}

	return sb, nil
}

// Env returns the os.Environ()-style slice to inject into each step subprocess.
func (s *Sandbox) Env(stepID string, mode xrr.Mode, run *Run) []string {
	passthrough := ""
	if run != nil && len(run.Passthrough) > 0 {
		passthrough = strings.Join(run.Passthrough, ",")
	}

	cassetteDir := ""
	if run != nil {
		cassetteDir = filepath.Join(run.RecordDir, stepID)
	}

	overrides := map[string]string{
		"HOME":                       s.HomeDir,
		"GIT_DIR":                    filepath.Join(s.RepoDir, ".git"),
		"GIT_WORK_TREE":              s.RepoDir,
		"GH_CONFIG_DIR":              filepath.Join(s.HomeDir, ".config", "gh"),
		"TLC_FLOW_TEST_MODE":         string(mode),
		"TLC_FLOW_TEST_STEP":         stepID,
		"TLC_FLOW_TEST_CASSETTE_DIR": cassetteDir,
		"TLC_FLOW_TEST_PASSTHROUGH":  passthrough,
	}

	// prepend BinDir to PATH
	origPath := os.Getenv("PATH")
	overrides["PATH"] = s.BinDir + string(os.PathListSeparator) + origPath

	base := os.Environ()
	result := make([]string, 0, len(base)+len(overrides))
	replaced := make(map[string]bool, len(overrides))

	for _, kv := range base {
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

// Teardown removes the sandbox root unless Keep is set.
func (s *Sandbox) Teardown() {
	if s.Keep {
		return
	}
	_ = os.RemoveAll(s.RootDir)
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
