# TLC Plugin Specification v0.1

## Overview

This document defines the plugin system for TLC (Task Line CLI), enabling extensibility through external system integrations, custom task executors, output formatters, and other capabilities.

## Design Principles

1. **Well-defined interfaces**: Clear plugin contracts
2. **Isolated execution**: Plugins cannot corrupt TLC state
3. **Security-first**: Permission model for sensitive operations
4. **Convention-based discovery**: Standard directory and naming
5. **Language-agnostic**: Support multiple implementation languages

---

## Plugin Types

### 1. External System Sync Plugins

Integrate with external task management systems (GitHub, Jira, Linear, etc.).

**Capabilities**:
- Pull tasks from external system
- Push local changes to external system
- Map external schema to TLC schema
- Handle authentication
- Detect and report conflicts

**Examples**: `github-sync`, `jira-sync`, `linear-sync`

### 2. Task Executor Plugins

Execute custom task types beyond built-in executors.

**Capabilities**:
- Register custom task types
- Execute tasks with custom logic
- Report execution status and outputs
- Handle timeouts and errors

**Examples**: `docker-executor`, `k8s-job-executor`, `aws-lambda-executor`

### 3. Output Formatter Plugins

Render task data in custom formats.

**Capabilities**:
- Register format names (e.g., `markdown`, `html`)
- Transform task data to format
- Support streaming output

**Examples**: `markdown-formatter`, `html-formatter`, `slack-formatter`

### 4. Notification Plugins

Send notifications on task events.

**Capabilities**:
- Subscribe to task events (CREATED, STATUS_CHANGED, etc.)
- Send notifications via external channels
- Support filtering and routing

**Examples**: `slack-notify`, `email-notify`, `webhook-notify`

### 5. Storage Backend Plugins

Implement alternative storage backends.

**Capabilities**:
- CRUD operations for tasks
- Query and filter support
- Transaction support
- Migration support

**Examples**: `postgres-storage`, `mongodb-storage`, `s3-storage`

---

## Plugin Interface

### Plugin Manifest

Each plugin must provide a manifest file:

**Location**: `<plugin-dir>/manifest.yaml`

```yaml
name: github-sync
version: 0.1.0
type: external-sync
description: GitHub issue synchronization

# Capabilities
capabilities:
  - sync.pull
  - sync.push
  - sync.bidirectional

# Entry point
entry_point: ./bin/github-sync

# Dependencies
requires:
  tlc_version: ">=0.1.0"

# Configuration schema
config_schema:
  type: object
  properties:
    repo:
      type: string
      required: true
    sync_direction:
      type: string
      enum: [pull, push, bidirectional]
      default: bidirectional

# Permissions required
permissions:
  - network.http
  - credential.read
```

### Plugin Lifecycle

```
1. Discovery    → TLC scans plugin directory
2. Validation   → Manifest validated against schema
3. Registration → Plugin registered in TLC registry
4. Activation   → Plugin initialized on first use
5. Execution    → Plugin methods called as needed
6. Deactivation → Plugin cleanup on TLC exit
```

---

## External System Sync Plugin

### Interface

Plugins communicate via JSON-RPC over stdio.

#### Methods

**`sync.pull`** — Pull updates from external system

Request:
```json
{
  "jsonrpc": "2.0",
  "method": "sync.pull",
  "params": {
    "last_sync_at": "2025-01-16T10:00:00Z",
    "filter": {
      "labels": ["tlc:tracked"]
    }
  },
  "id": 1
}
```

Response:
```json
{
  "jsonrpc": "2.0",
  "result": {
    "tasks": [
      {
        "id": "T-0042",
        "title": "Add API rate limiting",
        "status": "IN_PROGRESS",
        "meta": {
          "origin_system": "github",
          "origin_id": "456",
          "origin_url": "https://github.com/org/repo/issues/456"
        }
      }
    ],
    "conflicts": [
      {
        "task_id": "T-0041",
        "field": "title",
        "local_value": "Fix auth bug",
        "remote_value": "Fix authentication bug"
      }
    ],
    "sync_at": "2025-01-16T10:05:00Z"
  },
  "id": 1
}
```

**`sync.push`** — Push local changes to external system

Request:
```json
{
  "jsonrpc": "2.0",
  "method": "sync.push",
  "params": {
    "tasks": [
      {
        "id": "T-0042",
        "title": "Add API rate limiting (updated)",
        "status": "DONE",
        "meta": {
          "origin_id": "456"
        }
      }
    ]
  },
  "id": 2
}
```

Response:
```json
{
  "jsonrpc": "2.0",
  "result": {
    "pushed": ["T-0042"],
    "failed": [],
    "errors": []
  },
  "id": 2
}
```

**`auth.login`** — Authenticate with external system

Request:
```json
{
  "jsonrpc": "2.0",
  "method": "auth.login",
  "params": {
    "mode": "oauth",
    "redirect_uri": "http://localhost:8080/callback"
  },
  "id": 3
}
```

Response:
```json
{
  "jsonrpc": "2.0",
  "result": {
    "auth_url": "https://github.com/login/oauth/authorize?...",
    "poll_interval": 5000
  },
  "id": 3
}
```

**`auth.status`** — Check authentication status

Request:
```json
{
  "jsonrpc": "2.0",
  "method": "auth.status",
  "params": {},
  "id": 4
}
```

Response:
```json
{
  "jsonrpc": "2.0",
  "result": {
    "authenticated": true,
    "user": "codex",
    "expires_at": "2025-07-16T00:00:00Z"
  },
  "id": 4
}
```

### Schema Mapping

Plugin must map external system schema to TLC schema:

```yaml
# mapping.yaml
field_mappings:
  # GitHub → TLC
  github.number: id
  github.title: title
  github.body: description
  github.state: status
  github.assignee.login: assigned_to
  github.labels: tags
  github.milestone.title: meta.milestone

status_mappings:
  open: TODO
  in_progress: IN_PROGRESS
  closed: DONE

reverse_mappings:
  # TLC → GitHub
  TODO: open
  IN_PROGRESS: in_progress
  DONE: closed
```

---

## Task Executor Plugin

### Interface

**`executor.capabilities`** — Declare supported task types

Request:
```json
{
  "jsonrpc": "2.0",
  "method": "executor.capabilities",
  "params": {},
  "id": 1
}
```

Response:
```json
{
  "jsonrpc": "2.0",
  "result": {
    "task_types": ["docker-run", "docker-build"],
    "features": ["timeout", "output_streaming"]
  },
  "id": 1
}
```

**`executor.execute`** — Execute task

Request:
```json
{
  "jsonrpc": "2.0",
  "method": "executor.execute",
  "params": {
    "task_id": "T-0042",
    "task_type": "docker-run",
    "config": {
      "image": "nginx:latest",
      "command": ["nginx", "-g", "daemon off;"]
    },
    "timeout_ms": 30000
  },
  "id": 2
}
```

Response (streaming):
```json
// Progress update
{
  "jsonrpc": "2.0",
  "method": "executor.progress",
  "params": {
    "task_id": "T-0042",
    "status": "RUNNING",
    "stdout": "Pulling image nginx:latest...\n"
  }
}

// Final result
{
  "jsonrpc": "2.0",
  "result": {
    "task_id": "T-0042",
    "status": "SUCCEEDED",
    "exit_code": 0,
    "stdout": "...",
    "stderr": "",
    "duration_ms": 2500
  },
  "id": 2
}
```

---

## Output Formatter Plugin

### Interface

**`formatter.format`** — Format task data

Request:
```json
{
  "jsonrpc": "2.0",
  "method": "formatter.format",
  "params": {
    "format": "markdown",
    "tasks": [
      {
        "id": "T-0042",
        "title": "Add API rate limiting",
        "status": "IN_PROGRESS",
        "assigned_to": "codex"
      }
    ]
  },
  "id": 1
}
```

Response:
```json
{
  "jsonrpc": "2.0",
  "result": {
    "output": "# Tasks\n\n## T-0042: Add API rate limiting\n\n- **Status**: In Progress\n- **Assigned**: @codex\n"
  },
  "id": 1
}
```

---

## Notification Plugin

### Interface

**`notify.subscribe`** — Subscribe to events

Request:
```json
{
  "jsonrpc": "2.0",
  "method": "notify.subscribe",
  "params": {
    "events": ["TASK_CREATED", "STATUS_CHANGED"],
    "filter": {
      "meta.priority": ["high", "critical"]
    }
  },
  "id": 1
}
```

**`notify.event`** — Receive event (plugin → TLC)

Notification:
```json
{
  "jsonrpc": "2.0",
  "method": "notify.event",
  "params": {
    "event": "STATUS_CHANGED",
    "task_id": "T-0042",
    "old_status": "TODO",
    "new_status": "IN_PROGRESS",
    "actor": "codex",
    "timestamp": "2025-01-16T10:30:00Z"
  }
}
```

Plugin handles event and sends notification (e.g., Slack, email).

---

## Plugin Discovery

### Directory Structure

```
~/.config/tlc/plugins/
├── github-sync/
│   ├── manifest.yaml
│   ├── bin/
│   │   └── github-sync
│   └── config.yaml
├── slack-notify/
│   ├── manifest.yaml
│   ├── bin/
│   │   └── slack-notify
│   └── config.yaml
└── custom-executor/
    ├── manifest.yaml
    └── executor.py
```

### Discovery Process

1. TLC scans `~/.config/tlc/plugins/`
2. For each subdirectory, looks for `manifest.yaml`
3. Validates manifest against schema
4. Registers plugin if valid
5. Marks enabled plugins for activation

### Plugin Naming

Plugin directory name MUST match `manifest.yaml` name field:

```yaml
# ~/.config/tlc/plugins/github-sync/manifest.yaml
name: github-sync  # Must match directory name
```

---

## Plugin Configuration

### User Configuration

Plugin settings in TLC config:

```yaml
# ~/.config/tlc/config.yaml
plugins:
  directory: ~/.config/tlc/plugins
  enabled:
    - github-sync
    - slack-notify

  github-sync:
    repo: myorg/myrepo
    sync_direction: bidirectional

  slack-notify:
    webhook_url: https://hooks.slack.com/services/...
    channel: "#dev"
    notify_on:
      - TASK_CREATED
      - STATUS_CHANGED
```

### Plugin-Specific Config

Plugin can have its own config file:

```yaml
# ~/.config/tlc/plugins/github-sync/config.yaml
repo: myorg/myrepo
sync_direction: bidirectional
import_labels: true
label_prefix: "tlc:"
```

TLC passes merged config to plugin on activation.

---

## Security & Permissions

### Permission Model

Plugins declare required permissions in manifest:

```yaml
permissions:
  - network.http          # Make HTTP requests
  - network.webhook       # Listen on HTTP port
  - credential.read       # Read credentials from keychain
  - credential.write      # Write credentials to keychain
  - filesystem.read       # Read files
  - filesystem.write      # Write files
  - process.spawn         # Spawn subprocesses
  - database.read         # Read from TLC database
  - database.write        # Write to TLC database
```

### Permission Grant

On first plugin activation, TLC prompts user:

```
Plugin "github-sync" requests permissions:
  - network.http (Make HTTP requests)
  - credential.read (Read GitHub token from keychain)

Grant permissions? (y/n):
```

Granted permissions stored in:
```yaml
# ~/.config/tlc/plugin-permissions.yaml
github-sync:
  granted:
    - network.http
    - credential.read
  denied: []
  granted_at: "2025-01-16T10:00:00Z"
```

### Credential Isolation

Plugins cannot access credentials for other plugins:

```yaml
# Plugin can only access its own credentials
# github-sync can read "github-sync:token"
# github-sync CANNOT read "jira-sync:token"
```

### Sandboxing

**Recommended** (but not required for v0.1):
- Run plugins in separate processes
- Use IPC (stdio, JSON-RPC) for communication
- Limit filesystem access to plugin directory
- Use OS-level sandboxing (containers, VMs)

---

## Plugin Development

### Minimal Example (Bash)

```bash
#!/bin/bash
# ~/.config/tlc/plugins/hello/bin/hello

# Read JSON-RPC request from stdin
read -r request

# Parse method (simplified)
method=$(echo "$request" | jq -r '.method')

case "$method" in
  "hello.greet")
    # Return JSON-RPC response
    echo '{
      "jsonrpc": "2.0",
      "result": {"message": "Hello from plugin!"},
      "id": 1
    }'
    ;;
  *)
    echo '{
      "jsonrpc": "2.0",
      "error": {"code": -32601, "message": "Method not found"},
      "id": 1
    }'
    ;;
esac
```

**Manifest**:
```yaml
# ~/.config/tlc/plugins/hello/manifest.yaml
name: hello
version: 0.1.0
type: custom
description: Hello world plugin
entry_point: ./bin/hello
requires:
  tlc_version: ">=0.1.0"
permissions: []
```

**Usage**:
```bash
tlc plugin call hello hello.greet
# Output: {"message": "Hello from plugin!"}
```

### Python Example

```python
#!/usr/bin/env python3
# ~/.config/tlc/plugins/example/executor.py

import sys
import json

def handle_request(request):
    method = request.get('method')
    params = request.get('params', {})
    req_id = request.get('id')

    if method == 'executor.capabilities':
        return {
            'jsonrpc': '2.0',
            'result': {
                'task_types': ['python-script'],
                'features': ['timeout']
            },
            'id': req_id
        }

    elif method == 'executor.execute':
        # Execute task logic
        return {
            'jsonrpc': '2.0',
            'result': {
                'task_id': params['task_id'],
                'status': 'SUCCEEDED',
                'exit_code': 0
            },
            'id': req_id
        }

    else:
        return {
            'jsonrpc': '2.0',
            'error': {
                'code': -32601,
                'message': 'Method not found'
            },
            'id': req_id
        }

if __name__ == '__main__':
    for line in sys.stdin:
        request = json.loads(line)
        response = handle_request(request)
        print(json.dumps(response))
        sys.stdout.flush()
```

### Go Example

```go
// ~/.config/tlc/plugins/example/main.go
package main

import (
    "bufio"
    "encoding/json"
    "fmt"
    "os"
)

type Request struct {
    JSONRPC string          `json:"jsonrpc"`
    Method  string          `json:"method"`
    Params  json.RawMessage `json:"params"`
    ID      int             `json:"id"`
}

type Response struct {
    JSONRPC string      `json:"jsonrpc"`
    Result  interface{} `json:"result,omitempty"`
    Error   *Error      `json:"error,omitempty"`
    ID      int         `json:"id"`
}

type Error struct {
    Code    int    `json:"code"`
    Message string `json:"message"`
}

func main() {
    scanner := bufio.NewScanner(os.Stdin)
    for scanner.Scan() {
        var req Request
        if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
            continue
        }

        resp := handleRequest(req)
        json.NewEncoder(os.Stdout).Encode(resp)
    }
}

func handleRequest(req Request) Response {
    switch req.Method {
    case "executor.capabilities":
        return Response{
            JSONRPC: "2.0",
            Result: map[string]interface{}{
                "task_types": []string{"go-build"},
                "features":   []string{"timeout"},
            },
            ID: req.ID,
        }
    default:
        return Response{
            JSONRPC: "2.0",
            Error: &Error{
                Code:    -32601,
                Message: "Method not found",
            },
            ID: req.ID,
        }
    }
}
```

---

## Plugin CLI Commands

### List Plugins

```bash
tlc plugin list

# Output
Name            Type            Version  Status
github-sync     external-sync   0.1.0    enabled
slack-notify    notification    0.1.0    enabled
jira-sync       external-sync   0.1.0    disabled
```

### Enable/Disable Plugin

```bash
tlc plugin enable github-sync
tlc plugin disable slack-notify
```

### Call Plugin Method

```bash
tlc plugin call <plugin> <method> [params]

# Examples
tlc plugin call github-sync auth.status
tlc plugin call slack-notify notify.test '{"message":"Hello"}'
```

### Install Plugin

```bash
# From URL
tlc plugin install https://github.com/user/tlc-plugin-name

# From local directory
tlc plugin install ./path/to/plugin

# Output
Installing github-sync v0.1.0...
✓ Downloaded plugin
✓ Validated manifest
✓ Installed to ~/.config/tlc/plugins/github-sync
✓ Plugin ready to use

Run: tlc plugin enable github-sync
```

### Uninstall Plugin

```bash
tlc plugin uninstall github-sync

# Confirmation prompt
Remove plugin "github-sync"? (y/n): y
✓ Plugin uninstalled
```

---

## Plugin Registry (Future)

### Public Registry

Centralized plugin registry for discovery:

```bash
# Search registry
tlc plugin search github

# Output
github-sync          Official GitHub integration
github-actions       Run GitHub Actions locally
github-webhook       GitHub webhook receiver

# Install from registry
tlc plugin install github-sync
```

### Registry Structure

```
https://registry.tlc.dev/
├── plugins/
│   ├── github-sync/
│   │   ├── 0.1.0/
│   │   │   ├── manifest.yaml
│   │   │   └── plugin.tar.gz
│   │   └── latest -> 0.1.0
│   └── slack-notify/
│       └── 0.1.0/
└── index.json
```

---

## Error Handling

### JSON-RPC Errors

Standard JSON-RPC error codes:

| Code | Message | Meaning |
|------|---------|---------|
| -32700 | Parse error | Invalid JSON |
| -32600 | Invalid request | Missing required fields |
| -32601 | Method not found | Unknown method |
| -32602 | Invalid params | Bad parameter types |
| -32603 | Internal error | Plugin error |

Custom error codes (plugin-specific):

```json
{
  "jsonrpc": "2.0",
  "error": {
    "code": 1001,
    "message": "GitHub API rate limit exceeded",
    "data": {
      "reset_at": "2025-01-16T11:00:00Z"
    }
  },
  "id": 1
}
```

### Plugin Failures

If plugin process exits unexpectedly:

```bash
tlc sync pull github

# Output
Error: Plugin "github-sync" crashed
  Exit code: 1
  Stderr: Failed to connect to GitHub API

Run 'tlc plugin logs github-sync' for details
```

### Plugin Logs

```bash
tlc plugin logs github-sync

# Output (last 50 lines)
2025-01-16T10:30:00Z INFO  Starting sync
2025-01-16T10:30:01Z DEBUG Fetching issues from GitHub
2025-01-16T10:30:02Z ERROR Failed to connect: timeout
2025-01-16T10:30:02Z INFO  Retrying in 5s...
```

---

## Best Practices

### For Plugin Developers

1. **Validate inputs**: Check all params before processing
2. **Handle timeouts**: Respect timeout params from TLC
3. **Stream output**: Use progress notifications for long operations
4. **Graceful degradation**: Handle API failures gracefully
5. **Log verbosely**: Help users debug issues
6. **Version manifests**: Update version on breaking changes
7. **Document permissions**: Explain why each permission is needed

### For Plugin Users

1. **Review permissions**: Only grant necessary permissions
2. **Use trusted sources**: Prefer official/verified plugins
3. **Keep updated**: Update plugins regularly for security fixes
4. **Test in isolation**: Test new plugins before production use
5. **Monitor logs**: Check plugin logs for errors

---

## Testing

### Plugin Testing Framework

```bash
tlc plugin test ./my-plugin

# Output
Running tests for my-plugin...
✓ Manifest validation
✓ Entry point exists and executable
✓ Capabilities method responds
✓ Execute method works
✓ Error handling works
✓ Timeout handling works

6/6 tests passed
```

### Integration Testing

```bash
# Test sync plugin
tlc plugin test github-sync --integration

# Runs real sync operations against test repository
```

---

## References

- [tlc-cli-spec-0.1.md](tlc-cli-spec-0.1.md) — CLI commands
- [tlc-config-spec-0.1.md](tlc-config-spec-0.1.md) — Configuration
- [tlc-auth-spec-0.1.md](tlc-auth-spec-0.1.md) — Authentication (to be created)
- [task-exec-spec-0.1.md](task-exec-spec-0.1.md) — Execution protocol
- [sync-architecture-0.1.md](sync-architecture-0.1.md) — Sync architecture
- [JSON-RPC 2.0 Specification](https://www.jsonrpc.org/specification)

---

**Version**: 0.1
**Last Updated**: 2025-01-16
**Status**: Normative
