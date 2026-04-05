package xrr

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// envelope is the on-disk wrapper for both req and resp files.
type envelope struct {
	XRR         string `yaml:"xrr"`
	Adapter     string `yaml:"adapter"`
	Fingerprint string `yaml:"fingerprint"`
	RecordedAt  string `yaml:"recorded_at"`
	Payload     any    `yaml:"payload"`
}

// FileCassette stores interactions as YAML files in a directory.
type FileCassette struct {
	dir string
}

// NewFileCassette creates a FileCassette that reads/writes to dir.
func NewFileCassette(dir string) *FileCassette {
	return &FileCassette{dir: dir}
}

// Save writes req and resp as two YAML files under dir.
func (c *FileCassette) Save(adapterID, fingerprint string, req, resp any) error {
	now := time.Now().UTC().Format(time.RFC3339)
	if err := c.write(adapterID, fingerprint, "req", now, req); err != nil {
		return err
	}
	return c.write(adapterID, fingerprint, "resp", now, resp)
}

func (c *FileCassette) write(adapterID, fingerprint, kind, recordedAt string, payload any) error {
	env := envelope{
		XRR:         "1",
		Adapter:     adapterID,
		Fingerprint: fingerprint,
		RecordedAt:  recordedAt,
		Payload:     payload,
	}
	data, err := yaml.Marshal(env)
	if err != nil {
		return fmt.Errorf("xrr: marshal %s: %w", kind, err)
	}
	path := filepath.Join(c.dir, fmt.Sprintf("%s-%s.%s.yaml", adapterID, fingerprint, kind))
	return os.WriteFile(path, data, 0o644)
}

// Load reads the req and resp files and unmarshals payloads into targets.
func (c *FileCassette) Load(adapterID, fingerprint string, reqTarget, respTarget any) error {
	if err := c.read(adapterID, fingerprint, "req", reqTarget); err != nil {
		return err
	}
	return c.read(adapterID, fingerprint, "resp", respTarget)
}

func (c *FileCassette) read(adapterID, fingerprint, kind string, target any) error {
	path := filepath.Join(c.dir, fmt.Sprintf("%s-%s.%s.yaml", adapterID, fingerprint, kind))
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ErrCassetteMiss
		}
		return fmt.Errorf("xrr: read %s: %w", kind, err)
	}

	// First pass: decode full envelope to extract payload node.
	var raw map[string]yaml.Node
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("xrr: unmarshal envelope %s: %w", kind, err)
	}
	payloadNode, ok := raw["payload"]
	if !ok {
		return fmt.Errorf("xrr: missing payload in %s", kind)
	}

	// Second pass: decode payload node into target.
	if err := payloadNode.Decode(target); err != nil {
		return fmt.Errorf("xrr: decode payload %s: %w", kind, err)
	}
	return nil
}
