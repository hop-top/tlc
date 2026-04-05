package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
)

// xrayMaxBytes is the hard cap on xray content injected into LLM prompts.
const xrayMaxBytes = 4096

// loadXrayContext returns the xray root chunk content for enriching
// LLM prompts. It tries cached files first, then JIT generation.
// Returns empty string if xray is unavailable (non-fatal).
func loadXrayContext() string {
	if content := findXrayCache(); content != "" {
		return truncateBytes(content, xrayMaxBytes)
	}
	if content := generateXrayJIT(); content != "" {
		return truncateBytes(content, xrayMaxBytes)
	}
	return ""
}

// findXrayCache globs for .xray_*.md files in the current directory and
// returns the content of the first one (alphabetically sorted). Returns
// empty string if no cached files exist or on any read error.
func findXrayCache() string {
	matches, err := filepath.Glob(".xray_*.md")
	if err != nil || len(matches) == 0 {
		return ""
	}
	sort.Strings(matches)

	data, err := os.ReadFile(matches[0])
	if err != nil {
		return ""
	}
	return string(data)
}

// generateXrayJIT runs xray map to produce a root-level codebase overview.
// Returns empty string if xray is not installed or the command fails.
func generateXrayJIT() string {
	out, err := exec.Command("xray", "map", "--no-chunk", "--limit", "4096").Output()
	if err != nil {
		return ""
	}
	return string(out)
}

// truncateBytes returns at most n bytes of s. It does not attempt to
// preserve UTF-8 boundaries because the content is markdown/ascii.
func truncateBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
