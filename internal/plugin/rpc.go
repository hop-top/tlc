package plugin

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
)

// RPCClient handles JSON-RPC communication over stdio
type RPCClient struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Scanner
}

type Request struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params"`
	ID      int         `json:"id"`
}

type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
	ID      int             `json:"id"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// NewRPCClient starts a plugin and returns an RPC client
func NewRPCClient(binPath string) (*RPCClient, error) {
	cmd := exec.Command(binPath)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	return &RPCClient{
		cmd:    cmd,
		stdin:  stdin,
		stdout: bufio.NewScanner(stdout),
	}, nil
}

// Call makes a JSON-RPC call
func (c *RPCClient) Call(method string, params interface{}, result interface{}) error {
	req := Request{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
		ID:      1, // Simplified
	}

	data, err := json.Marshal(req)
	if err != nil {
		return err
	}

	if _, err := fmt.Fprintf(c.stdin, "%s\n", data); err != nil {
		return err
	}

	if !c.stdout.Scan() {
		return fmt.Errorf("plugin terminated unexpectedly")
	}

	var resp Response
	if err := json.Unmarshal(c.stdout.Bytes(), &resp); err != nil {
		return err
	}

	if resp.Error != nil {
		return fmt.Errorf("plugin error [%d]: %s", resp.Error.Code, resp.Error.Message)
	}

	if result != nil {
		return json.Unmarshal(resp.Result, result)
	}

	return nil
}

// Close terminates the plugin
func (c *RPCClient) Close() error {
	c.stdin.Close()
	return c.cmd.Wait()
}
