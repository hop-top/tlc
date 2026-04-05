// Package llm provides a provider-agnostic LLM abstraction for CLI tools.
//
// Adapters register via [Register] with a URI scheme. The [Client] facade
// wraps a resolved adapter, probes capabilities via type assertion, and
// supports fallback chains and event hooks.
package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	llmerrors "hop.top/kit/llm/errors"
)

// ---------------------------------------------------------------------------
// Core interfaces
// ---------------------------------------------------------------------------

// Provider is the base interface all adapters implement.
type Provider interface {
	Close() error
}

// Completer produces a single completion response.
type Completer interface {
	Complete(ctx context.Context, req Request) (Response, error)
}

// Streamer produces a streaming token iterator.
type Streamer interface {
	Stream(ctx context.Context, req Request) (TokenIterator, error)
}

// ToolCaller invokes tool-use / function-calling.
type ToolCaller interface {
	CallWithTools(ctx context.Context, req Request, tools []ToolDef) (ToolResponse, error)
}

// ---------------------------------------------------------------------------
// Data types
// ---------------------------------------------------------------------------

// Message is a single role+content pair in a conversation.
type Message struct {
	Role    string
	Content string
}

// Request carries all parameters for a completion call.
type Request struct {
	Messages      []Message
	Model         string
	Temperature   float64
	MaxTokens     int
	StopSequences []string
	Extensions    map[string]any
}

// Response is the result of a completion call.
type Response struct {
	Content      string
	Role         string
	Usage        Usage
	FinishReason string
}

// Usage reports token consumption.
type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// Token is a single streaming chunk.
type Token struct {
	Content string
	Done    bool
}

// TokenIterator yields streaming tokens.
//
// Contract: the final token has Done=true and a nil error. Every
// subsequent call to Next returns (Token{}, io.EOF). Adapters MUST
// follow this two-phase termination so consumers can rely on either
// signal.
type TokenIterator interface {
	Next() (Token, error)
	Close() error
}

// ToolDef describes a tool the model may call.
type ToolDef struct {
	Name        string
	Description string
	Parameters  json.RawMessage
}

// ToolCall is a single tool invocation returned by the model.
type ToolCall struct {
	ID        string
	Name      string
	Arguments json.RawMessage
}

// ToolResponse is the result of a tool-calling completion.
type ToolResponse struct {
	Content   string
	ToolCalls []ToolCall
}

// ---------------------------------------------------------------------------
// Registry
// ---------------------------------------------------------------------------

// Factory creates a Provider from a resolved configuration.
type Factory func(cfg ResolvedConfig) (Provider, error)

// Registry maps URI schemes to adapter factories.
type Registry struct {
	mu        sync.RWMutex
	factories map[string]Factory
}

// DefaultRegistry is the process-wide registry used by adapter init
// functions and the package-level [Register] / [Resolve] helpers.
var DefaultRegistry = NewRegistry()

// Register adds a factory for the given scheme to [DefaultRegistry].
func Register(scheme string, f Factory) { DefaultRegistry.Register(scheme, f) }

// Resolve looks up and creates a provider via [DefaultRegistry].
func Resolve(uri string) (Provider, error) { return DefaultRegistry.Resolve(uri) }

// NewRegistry creates an empty adapter registry.
func NewRegistry() *Registry {
	return &Registry{factories: make(map[string]Factory)}
}

// Register adds a factory for the given scheme. Panics on duplicate.
func (r *Registry) Register(scheme string, f Factory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.factories[scheme]; ok {
		panic(fmt.Sprintf(
			"llm: adapter already registered for scheme %q", scheme,
		))
	}
	r.factories[scheme] = f
}

// Resolve parses a provider URI, looks up the factory by scheme,
// and creates a Provider. It uses [ParseURI] to build a minimal
// [ResolvedConfig] directly from the URI components and parameters.
func (r *Registry) Resolve(uri string) (Provider, error) {
	parsed, err := ParseURI(uri)
	if err != nil {
		return nil, fmt.Errorf("llm: invalid URI %q: %w", uri, err)
	}

	r.mu.RLock()
	f, ok := r.factories[parsed.Scheme]
	r.mu.RUnlock()

	if !ok {
		return nil, llmerrors.NewProviderNotFound(parsed.Scheme)
	}

	cfg := ResolvedConfig{
		URI: parsed,
		Provider: ProviderConfig{
			Model: parsed.Model,
		},
	}
	if parsed.Host != "" {
		cfg.Provider.BaseURL = "http://" + parsed.Host
	}
	if parsed.Params != nil {
		cfg.Provider.Params = parsed.Params
		if v, ok := parsed.Params["api_key"]; ok {
			cfg.Provider.APIKey = v
		}
		if v, ok := parsed.Params["base_url"]; ok {
			cfg.Provider.BaseURL = v
		}
	}

	return f(cfg)
}

// ---------------------------------------------------------------------------
// Client options
// ---------------------------------------------------------------------------

// Option configures a Client.
type Option func(*clientConfig)

type clientConfig struct {
	fallbacks  []Provider
	onRequest  func(Request)
	onResponse func(Response, time.Duration)
	onError    func(error)
	onFallback func(from, to int, err error)
}

// WithFallback appends a fallback provider to the chain.
func WithFallback(p Provider) Option {
	return func(c *clientConfig) {
		c.fallbacks = append(c.fallbacks, p)
	}
}

// OnRequest registers a hook fired before each request.
func OnRequest(fn func(Request)) Option {
	return func(c *clientConfig) { c.onRequest = fn }
}

// OnResponse registers a hook fired after a successful response.
func OnResponse(fn func(Response, time.Duration)) Option {
	return func(c *clientConfig) { c.onResponse = fn }
}

// OnError registers a hook fired when a request fails.
func OnError(fn func(error)) Option {
	return func(c *clientConfig) { c.onError = fn }
}

// OnFallback registers a hook fired when falling back to the next
// provider. from/to are zero-based indices (0 = primary).
func OnFallback(fn func(from, to int, err error)) Option {
	return func(c *clientConfig) { c.onFallback = fn }
}

// ---------------------------------------------------------------------------
// Client
// ---------------------------------------------------------------------------

// Client is a facade that wraps a resolved adapter and provides
// capability probing, fallback chains, and event hooks.
type Client struct {
	primary Provider
	cfg     clientConfig
}

// NewClient creates a Client wrapping the given primary adapter.
func NewClient(primary Provider, opts ...Option) *Client {
	var cfg clientConfig
	for _, o := range opts {
		o(&cfg)
	}
	return &Client{primary: primary, cfg: cfg}
}

// Provider returns the underlying primary adapter for direct access
// or type assertion.
func (c *Client) Provider() Provider { return c.primary }

// Capabilities returns the list of capabilities the primary adapter
// supports, probed via type assertion.
func (c *Client) Capabilities() []string {
	var caps []string
	if _, ok := c.primary.(Completer); ok {
		caps = append(caps, "complete")
	}
	if _, ok := c.primary.(Streamer); ok {
		caps = append(caps, "stream")
	}
	if _, ok := c.primary.(ToolCaller); ok {
		caps = append(caps, "tool_call")
	}
	return caps
}

// Complete delegates to the adapter's Completer. Supports fallback
// and event hooks.
func (c *Client) Complete(ctx context.Context, req Request) (Response, error) {
	if c.cfg.onRequest != nil {
		c.cfg.onRequest(req)
	}

	chain := append([]Provider{c.primary}, c.cfg.fallbacks...)
	var errs []error

	for i, p := range chain {
		comp, ok := p.(Completer)
		if !ok {
			err := llmerrors.NewCapabilityNotSupported(
				"complete", fmt.Sprintf("provider[%d]", i),
			)
			errs = append(errs, err)
			continue
		}

		start := time.Now()
		resp, err := comp.Complete(ctx, req)
		dur := time.Since(start)

		if err == nil {
			if c.cfg.onResponse != nil {
				c.cfg.onResponse(resp, dur)
			}
			return resp, nil
		}

		errs = append(errs, err)

		// Non-fallbackable errors return immediately.
		if !llmerrors.IsFallbackable(err) {
			if c.cfg.onError != nil {
				c.cfg.onError(err)
			}
			return Response{}, err
		}

		// Fire fallback hook before trying next.
		if i < len(chain)-1 && c.cfg.onFallback != nil {
			c.cfg.onFallback(i, i+1, err)
		}
	}

	// If every provider lacked the capability, return that directly.
	if allCapabilityErrors(errs) {
		return Response{}, errs[len(errs)-1]
	}

	exhausted := llmerrors.NewFallbackExhausted(errs)
	if c.cfg.onError != nil {
		c.cfg.onError(exhausted)
	}
	return Response{}, exhausted
}

func allCapabilityErrors(errs []error) bool {
	for _, e := range errs {
		var capErr *llmerrors.ErrCapabilityNotSupported
		if !errors.As(e, &capErr) {
			return false
		}
	}
	return len(errs) > 0
}

// Stream delegates to the adapter's Streamer interface. Supports fallback
// and event hooks.
func (c *Client) Stream(ctx context.Context, req Request) (TokenIterator, error) {
	if c.cfg.onRequest != nil {
		c.cfg.onRequest(req)
	}

	chain := append([]Provider{c.primary}, c.cfg.fallbacks...)
	var errs []error

	for i, p := range chain {
		s, ok := p.(Streamer)
		if !ok {
			err := llmerrors.NewCapabilityNotSupported(
				"stream", fmt.Sprintf("provider[%d]", i),
			)
			errs = append(errs, err)
			continue
		}

		iter, err := s.Stream(ctx, req)
		if err == nil {
			return iter, nil
		}

		errs = append(errs, err)

		if !llmerrors.IsFallbackable(err) {
			if c.cfg.onError != nil {
				c.cfg.onError(err)
			}
			return nil, err
		}

		if i < len(chain)-1 && c.cfg.onFallback != nil {
			c.cfg.onFallback(i, i+1, err)
		}
	}

	if allCapabilityErrors(errs) {
		return nil, errs[len(errs)-1]
	}

	exhausted := llmerrors.NewFallbackExhausted(errs)
	if c.cfg.onError != nil {
		c.cfg.onError(exhausted)
	}
	return nil, exhausted
}

// CallWithTools delegates to the adapter's ToolCaller interface. Supports
// fallback and event hooks.
func (c *Client) CallWithTools(
	ctx context.Context, req Request, tools []ToolDef,
) (ToolResponse, error) {
	if c.cfg.onRequest != nil {
		c.cfg.onRequest(req)
	}

	chain := append([]Provider{c.primary}, c.cfg.fallbacks...)
	var errs []error

	for i, p := range chain {
		tc, ok := p.(ToolCaller)
		if !ok {
			err := llmerrors.NewCapabilityNotSupported(
				"tool_call", fmt.Sprintf("provider[%d]", i),
			)
			errs = append(errs, err)
			continue
		}

		start := time.Now()
		resp, err := tc.CallWithTools(ctx, req, tools)
		dur := time.Since(start)

		if err == nil {
			if c.cfg.onResponse != nil {
				c.cfg.onResponse(Response{Content: resp.Content}, dur)
			}
			return resp, nil
		}

		errs = append(errs, err)

		if !llmerrors.IsFallbackable(err) {
			if c.cfg.onError != nil {
				c.cfg.onError(err)
			}
			return ToolResponse{}, err
		}

		if i < len(chain)-1 && c.cfg.onFallback != nil {
			c.cfg.onFallback(i, i+1, err)
		}
	}

	if allCapabilityErrors(errs) {
		return ToolResponse{}, errs[len(errs)-1]
	}

	exhausted := llmerrors.NewFallbackExhausted(errs)
	if c.cfg.onError != nil {
		c.cfg.onError(exhausted)
	}
	return ToolResponse{}, exhausted
}
