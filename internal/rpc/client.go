// Package rpc provides a lightweight JSON-RPC 2.0 client for communicating
// with external binary plugins over stdio.
package rpc

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
)

// Client handles JSON-RPC communication over stdio with an external binary.
type Client struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Scanner
	nextID int
}

// Request is a JSON-RPC 2.0 request.
type Request struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params"`
	ID      int         `json:"id"`
}

// Response is a JSON-RPC 2.0 response.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
	ID      int             `json:"id"`
}

// Error is a JSON-RPC 2.0 error object.
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// NewClient starts a plugin binary and returns a Client for RPC calls.
func NewClient(binPath string) (*Client, error) {
	cmd := exec.CommandContext(context.Background(), binPath)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("create stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("create stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start plugin: %w", err)
	}

	// 1MB initial buffer, 10MB max for large responses.
	scanner := bufio.NewScanner(stdout)
	buf := make([]byte, 0, 1024*1024)
	scanner.Buffer(buf, 10*1024*1024)

	return &Client{
		cmd:    cmd,
		stdin:  stdin,
		stdout: scanner,
		nextID: 1,
	}, nil
}

// Call makes a JSON-RPC call and decodes the result into result.
func (c *Client) Call(method string, params interface{}, result interface{}) error {
	id := c.nextID
	c.nextID++

	req := Request{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
		ID:      id,
	}

	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	if _, err := fmt.Fprintf(c.stdin, "%s\n", data); err != nil {
		return fmt.Errorf("write request: %w", err)
	}

	if !c.stdout.Scan() {
		return fmt.Errorf("plugin terminated unexpectedly")
	}

	var resp Response
	if err := json.Unmarshal(c.stdout.Bytes(), &resp); err != nil {
		return fmt.Errorf("unmarshal response: %w", err)
	}

	if resp.Error != nil {
		return fmt.Errorf("plugin error [%d]: %s", resp.Error.Code, resp.Error.Message)
	}

	if result != nil {
		if err := json.Unmarshal(resp.Result, result); err != nil {
			return fmt.Errorf("unmarshal result: %w", err)
		}
	}

	return nil
}

// Close terminates the plugin process.
func (c *Client) Close() error {
	if err := c.stdin.Close(); err != nil {
		return fmt.Errorf("close stdin: %w", err)
	}
	if err := c.cmd.Wait(); err != nil {
		return fmt.Errorf("wait for plugin: %w", err)
	}
	return nil
}
