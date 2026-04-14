package core

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// maxResultFileSize is the hard limit for results files (50 MB).
const maxResultFileSize = 50 * 1024 * 1024

// ResultCollector merges agent results from stdout (status signal) and
// the results file on the volume (artifact payload).
type ResultCollector struct{}

// NewResultCollector returns a new ResultCollector.
func NewResultCollector() *ResultCollector {
	return &ResultCollector{}
}

// CollectFromExec merges stdout and volume results into a single
// AgentResult. The merging rules are:
//
//  1. stdout says failed -> status is failed; volume artifacts discarded
//  2. stdout says succeeded + volume exists -> use volume for rich data
//  3. stdout says succeeded + no volume -> accept; warn about missing artifacts
//  4. Neither -> error: agent produced no results
//
// resultsFilePath may be empty if no volume file is expected.
func (c *ResultCollector) CollectFromExec(
	stdout string, resultsFilePath string,
) (*AgentResult, error) {
	stdoutResult, stdoutErr := parseStdoutJSON(stdout)
	volumeResult, volumeErr := readResultsFile(resultsFilePath)

	// Case 4: neither source available.
	if stdoutErr != nil && volumeErr != nil && volumeResult == nil {
		if stdoutErr != nil && volumeErr != nil {
			return nil, fmt.Errorf(
				"agent produced no results; stdout: %v; volume: %v",
				stdoutErr, volumeErr,
			)
		}
	}

	// If stdout has no result but volume does, use volume.
	if stdoutErr != nil && volumeResult != nil {
		if err := volumeResult.Validate(); err != nil {
			return nil, fmt.Errorf("volume result invalid: %w", err)
		}
		return volumeResult, nil
	}

	// If stdout has no result and no volume, error.
	if stdoutErr != nil {
		return nil, fmt.Errorf(
			"agent produced no results; check container logs with 'pod logs <name>'",
		)
	}

	// Case 1: stdout says failed -> discard volume.
	if stdoutResult.Status == AgentStatusFailed {
		if err := stdoutResult.Validate(); err != nil {
			return nil, fmt.Errorf("stdout result invalid: %w", err)
		}
		return stdoutResult, nil
	}

	// Case 2: stdout says succeeded + volume exists -> use volume.
	if volumeResult != nil && volumeErr == nil {
		if err := volumeResult.Validate(); err != nil {
			return nil, fmt.Errorf("volume result invalid: %w", err)
		}
		return volumeResult, nil
	}

	// Case 3: stdout succeeded + no volume -> use stdout.
	if err := stdoutResult.Validate(); err != nil {
		return nil, fmt.Errorf("stdout result invalid: %w", err)
	}
	return stdoutResult, nil
}

// parseStdoutJSON extracts the last JSON line from stdout as an
// AgentResult.
func parseStdoutJSON(stdout string) (*AgentResult, error) {
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "{") {
			var r AgentResult
			if err := json.Unmarshal([]byte(trimmed), &r); err != nil {
				return nil, fmt.Errorf("parse stdout JSON: %w", err)
			}
			return &r, nil
		}
	}
	return nil, fmt.Errorf("no JSON status line found in agent stdout")
}

// readResultsFile reads and parses the volume results file. Returns
// (nil, nil) if the file does not exist.
func readResultsFile(path string) (*AgentResult, error) {
	if path == "" {
		return nil, nil
	}

	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("stat results file: %w", err)
	}

	if info.Size() > maxResultFileSize {
		return nil, fmt.Errorf(
			"results file too large (%d bytes, max %d bytes); "+
				"reduce agent output or split artifacts",
			info.Size(), maxResultFileSize,
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

	if result.Version != AgentResultVersion {
		return nil, fmt.Errorf(
			"unsupported result version %d in volume file; expected %d",
			result.Version, AgentResultVersion,
		)
	}

	return &result, nil
}
