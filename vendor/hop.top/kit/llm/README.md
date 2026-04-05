# llm -- Provider-Agnostic LLM Package

Unified LLM abstraction for CLI tools. Adapters register via URI
scheme; Client facade handles capability probing, fallback chains,
event hooks.

Available in Go (primary), TypeScript, Python.

## Quick Start

```
import "hop.top/kit/llm"
import _ "hop.top/kit/llm/openai"   // register openai scheme

provider, _ := llm.Resolve("openai://gpt-4o")
client := llm.NewClient(provider)

resp, _ := client.Complete(ctx, llm.Request{
    Messages: []llm.Message{{Role: "user", Content: "hello"}},
})
print(resp.Content)
```

## URI Format

```
scheme://[host:port/]model[?param=val&param2=val2]
```

| Scheme       | Provider    | Example                               |
|--------------|-------------|---------------------------------------|
| `openai`     | OpenAI      | `openai://gpt-4o`                     |
| `anthropic`  | Anthropic   | `anthropic://claude-sonnet-4-20250514`|
| `ollama`     | Ollama      | `ollama://localhost:11434/llama3`    |
| `openrouter` | OpenRouter  | `openrouter://meta-llama/llama-3-70b` |
| `xai`        | xAI (Grok)  | `xai://grok-2`                        |
| `lmstudio`   | LM Studio   | `lmstudio://localhost:1234/model`     |

Query params: `?api_key=sk-...&base_url=https://...`

## Config File

Path: `{xdg.ConfigDir("hop")}/llm.yaml`

```yaml
default: "openai://gpt-4o"

providers:
  openai:
    api_key: "sk-..."
    base_url: "https://api.openai.com/v1"   # optional
    model: "gpt-4o"                          # default model
  anthropic:
    api_key: "sk-ant-..."
    model: "claude-sonnet-4-20250514"
  ollama:
    base_url: "http://localhost:11434"
    model: "llama3"

fallback:
  - "anthropic://claude-sonnet-4-20250514"
  - "ollama://llama3"
```

Three-layer merge: config file < URI < env vars.

## Environment Variables

| Variable       | Purpose                      | Example                    |
|----------------|------------------------------|----------------------------|
| `LLM_PROVIDER` | Default URI (no arg needed)  | `openai://gpt-4o`         |
| `LLM_API_KEY`  | Override API key             | `sk-...`                   |
| `LLM_BASE_URL` | Override base URL            | `https://api.openai.com/v1`|
| `LLM_FALLBACK` | Comma-sep fallback URIs      | `anthropic://...,ollama://`|

## Adapters

### OpenAI (`openai`, `openrouter`, `xai`, `lmstudio`)

- SDK: `openai/openai-go`
- Capabilities: Complete, Stream, ToolCall
- Registers four schemes via `init()`
- Default base URL: `https://api.openai.com/v1`

### Anthropic (`anthropic`)

- SDK: `anthropics/anthropic-sdk-go`
- Capabilities: Complete, Stream, ToolCall
- API key required
- Default max tokens: 1024

### Ollama (`ollama`)

- Thin HTTP client (no heavy dep)
- Capabilities: Complete, Stream (no ToolCall)
- Default base URL: `http://localhost:11434`
- Model required in URI

## Custom Adapters

Implement `Provider` + one or more capability interfaces.
Register via `init()`.

```
package myprovider

import "hop.top/kit/llm"

type Adapter struct { /* ... */ }

func (a *Adapter) Close() error { return nil }
func (a *Adapter) Complete(ctx, req) (Response, error) { /* ... */ }

func New(cfg llm.ResolvedConfig) (llm.Provider, error) {
    return &Adapter{model: cfg.Provider.Model}, nil
}

func init() {
    llm.Register("myscheme", New)
}
```

Import the package for side-effect registration:

```
import _ "mymodule/myprovider"
```

## Fallback Chain

Client tries providers in order: primary, then fallbacks.
Only fallbackable errors trigger next provider.

**Fallbackable** (retry next):
- Network errors (`net.OpError`)
- Rate limits (`ErrRateLimit`, 429)
- Server errors (`HTTPStatusError`, 5xx)

**Non-fallbackable** (fail immediately):
- Auth errors (`ErrAuth`, 401/403)
- Model not found (`ErrModel`, 404)
- Capability not supported (`ErrCapabilityNotSupported`)
- Context cancelled (`ErrContext`)

When all providers fail: `ErrFallbackExhausted` with all errors.

## Event Hooks

```
client := llm.NewClient(provider,
    llm.OnRequest(func(req llm.Request) {
        log("sending", req.Model)
    }),
    llm.OnResponse(func(resp llm.Response, dur time.Duration) {
        log("tokens", resp.Usage.TotalTokens, "in", dur)
    }),
    llm.OnError(func(err error) {
        log("failed", err)
    }),
    llm.OnFallback(func(from, to int, err error) {
        log("fallback", from, "->", to, err)
    }),
)
```

## Core Interfaces

| Interface    | Method                                   |
|--------------|------------------------------------------|
| `Provider`   | `Close() error`                          |
| `Completer`  | `Complete(ctx, Request) (Response, err)` |
| `Streamer`   | `Stream(ctx, Request) (TokenIterator, err)` |
| `ToolCaller` | `CallWithTools(ctx, Request, []ToolDef) (ToolResponse, err)` |

Client probes capabilities via type assertion. Call
`client.Capabilities()` to list supported ops.

## Cross-Language

Same URI format, config schema, env vars, error codes, and
capability names across Go, TypeScript, Python.

Config path: `{xdg.ConfigDir("hop")}/llm.yaml` on all platforms.
Go loads via viper/yaml; TS/Python via js-yaml/PyYAML.
All three parse the same YAML schema (`default`, `providers`,
`fallback`) and apply the same 3-layer merge order.
