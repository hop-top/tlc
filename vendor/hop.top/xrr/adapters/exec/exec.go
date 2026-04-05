package exec

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"

	xrr "hop.top/xrr"
	"gopkg.in/yaml.v3"
)

// Request represents an exec interaction request.
type Request struct {
	Argv  []string          `yaml:"argv"  json:"argv"`
	Stdin string            `yaml:"stdin,omitempty" json:"stdin,omitempty"`
	Env   map[string]string `yaml:"env,omitempty"   json:"env,omitempty"`
}

func (r *Request) AdapterID() string { return "exec" }

// Response represents an exec interaction response.
type Response struct {
	Stdout     string `yaml:"stdout"`
	Stderr     string `yaml:"stderr,omitempty"`
	ExitCode   int    `yaml:"exit_code"`
	DurationMs int64  `yaml:"duration_ms,omitempty"`
}

func (r *Response) AdapterID() string { return "exec" }

// Adapter implements xrr.Adapter for exec interactions.
type Adapter struct{}

// NewAdapter returns a new exec Adapter.
func NewAdapter() *Adapter { return &Adapter{} }

func (a *Adapter) ID() string { return "exec" }

// Fingerprint returns sha256(argv+stdin canonical JSON)[:8].
func (a *Adapter) Fingerprint(req xrr.Request) (string, error) {
	r, ok := req.(*Request)
	if !ok {
		return "", fmt.Errorf("exec: unexpected request type %T", req)
	}
	canonical, err := json.Marshal(map[string]any{
		"argv":  r.Argv,
		"stdin": r.Stdin,
	})
	if err != nil {
		return "", fmt.Errorf("exec: fingerprint marshal: %w", err)
	}
	sum := sha256.Sum256(canonical)
	return fmt.Sprintf("%x", sum[:4]), nil // 4 bytes = 8 hex chars
}

// Serialize marshals v as YAML.
func (a *Adapter) Serialize(v any) ([]byte, error) {
	return yaml.Marshal(v)
}

// Deserialize unmarshals data into target.
func (a *Adapter) Deserialize(data []byte, target any) error {
	return yaml.Unmarshal(data, target)
}
