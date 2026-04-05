// Package anthropic adapts the Anthropic Messages API for the llm
// package. It registers the "anthropic" URI scheme and implements
// [llm.Completer], [llm.Streamer], and [llm.ToolCaller].
package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
	"github.com/anthropics/anthropic-sdk-go/packages/ssestream"

	"hop.top/kit/llm"
	llmerrors "hop.top/kit/llm/errors"
)

const (
	defaultMaxTokens = 1024
	scheme           = "anthropic"
)

// Adapter wraps an anthropic-sdk-go client.
type Adapter struct {
	client anthropic.Client
	model  string
}

// compile-time interface checks
var (
	_ llm.Provider   = (*Adapter)(nil)
	_ llm.Completer  = (*Adapter)(nil)
	_ llm.Streamer   = (*Adapter)(nil)
	_ llm.ToolCaller = (*Adapter)(nil)
)

// New creates an Adapter from a resolved config.
func New(cfg llm.ResolvedConfig) (llm.Provider, error) {
	if cfg.Provider.APIKey == "" {
		return nil, fmt.Errorf("anthropic: API key required")
	}

	opts := []option.RequestOption{
		option.WithAPIKey(cfg.Provider.APIKey),
	}
	if cfg.Provider.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(cfg.Provider.BaseURL))
	}

	return &Adapter{
		client: anthropic.NewClient(opts...),
		model:  cfg.Provider.Model,
	}, nil
}

// Close is a no-op; the HTTP client does not need explicit teardown.
func (a *Adapter) Close() error { return nil }

func (a *Adapter) effectiveModel(req llm.Request) string {
	if req.Model != "" {
		return req.Model
	}
	return a.model
}

// Complete sends a single completion request and returns the
// aggregated response.
func (a *Adapter) Complete(
	ctx context.Context, req llm.Request,
) (llm.Response, error) {
	params := a.buildParams(req)
	msg, err := a.client.Messages.New(ctx, params)
	if err != nil {
		return llm.Response{}, mapError(err, a.effectiveModel(req))
	}
	return responseFromMessage(msg), nil
}

// Stream opens a streaming completion and returns a TokenIterator.
func (a *Adapter) Stream(
	ctx context.Context, req llm.Request,
) (llm.TokenIterator, error) {
	params := a.buildParams(req)
	stream := a.client.Messages.NewStreaming(ctx, params)
	// The SDK returns a stream immediately; check for init error.
	if err := stream.Err(); err != nil {
		_ = stream.Close()
		return nil, mapError(err, a.effectiveModel(req))
	}
	return &streamIter{stream: stream}, nil
}

// CallWithTools sends a tool-use request and maps the response back
// to [llm.ToolResponse].
func (a *Adapter) CallWithTools(
	ctx context.Context, req llm.Request, tools []llm.ToolDef,
) (llm.ToolResponse, error) {
	params := a.buildParams(req)
	params.Tools = mapTools(tools)
	msg, err := a.client.Messages.New(ctx, params)
	if err != nil {
		return llm.ToolResponse{}, mapError(err, a.effectiveModel(req))
	}
	return toolResponseFromMessage(msg), nil
}

// ---------------------------------------------------------------------------
// Request building
// ---------------------------------------------------------------------------

func (a *Adapter) buildParams(req llm.Request) anthropic.MessageNewParams {
	model := a.model
	if req.Model != "" {
		model = req.Model
	}

	maxTokens := int64(req.MaxTokens)
	if maxTokens <= 0 {
		maxTokens = defaultMaxTokens
	}

	system, msgs := extractSystem(req.Messages)

	p := anthropic.MessageNewParams{
		Model:     anthropic.Model(model),
		MaxTokens: maxTokens,
		Messages:  msgs,
	}

	if system != "" {
		p.System = []anthropic.TextBlockParam{
			{Text: system},
		}
	}

	if req.Temperature > 0 {
		p.Temperature = param.NewOpt(req.Temperature)
	}

	if len(req.StopSequences) > 0 {
		p.StopSequences = req.StopSequences
	}

	return p
}

// extractSystem separates any leading system message from the
// conversation. Anthropic requires system as a top-level field.
func extractSystem(msgs []llm.Message) (string, []anthropic.MessageParam) {
	var system string
	out := make([]anthropic.MessageParam, 0, len(msgs))

	for _, m := range msgs {
		if m.Role == "system" {
			system = m.Content
			continue
		}
		out = append(out, anthropic.MessageParam{
			Role:    anthropic.MessageParamRole(m.Role),
			Content: []anthropic.ContentBlockParamUnion{anthropic.NewTextBlock(m.Content)},
		})
	}

	return system, out
}

// mapTools converts llm.ToolDef slice to Anthropic tool params.
func mapTools(defs []llm.ToolDef) []anthropic.ToolUnionParam {
	out := make([]anthropic.ToolUnionParam, 0, len(defs))
	for _, d := range defs {
		var schema anthropic.ToolInputSchemaParam
		if len(d.Parameters) > 0 {
			_ = json.Unmarshal(d.Parameters, &schema)
		}
		out = append(out, anthropic.ToolUnionParam{
			OfTool: &anthropic.ToolParam{
				Name:        d.Name,
				Description: param.NewOpt(d.Description),
				InputSchema: schema,
			},
		})
	}
	return out
}

// ---------------------------------------------------------------------------
// Response mapping
// ---------------------------------------------------------------------------

func responseFromMessage(msg *anthropic.Message) llm.Response {
	var text string
	for _, block := range msg.Content {
		if block.Type == "text" {
			text += block.Text
		}
	}
	return llm.Response{
		Content:      text,
		Role:         "assistant",
		FinishReason: string(msg.StopReason),
		Usage: llm.Usage{
			PromptTokens:     int(msg.Usage.InputTokens),
			CompletionTokens: int(msg.Usage.OutputTokens),
			TotalTokens:      int(msg.Usage.InputTokens + msg.Usage.OutputTokens),
		},
	}
}

func toolResponseFromMessage(msg *anthropic.Message) llm.ToolResponse {
	var text string
	var calls []llm.ToolCall

	for _, block := range msg.Content {
		switch block.Type {
		case "text":
			text += block.Text
		case "tool_use":
			calls = append(calls, llm.ToolCall{
				ID:        block.ID,
				Name:      block.Name,
				Arguments: block.Input,
			})
		}
	}

	return llm.ToolResponse{
		Content:   text,
		ToolCalls: calls,
	}
}

// ---------------------------------------------------------------------------
// Streaming iterator
// ---------------------------------------------------------------------------

type streamIter struct {
	stream *ssestream.Stream[anthropic.MessageStreamEventUnion]
	done   bool
}

func (s *streamIter) Next() (llm.Token, error) {
	for {
		if s.done {
			return llm.Token{}, io.EOF
		}
		if !s.stream.Next() {
			s.done = true
			if err := s.stream.Err(); err != nil {
				return llm.Token{}, mapError(err, "")
			}
			return llm.Token{Done: true}, nil
		}
		evt := s.stream.Current()
		switch evt.Type {
		case "content_block_delta":
			if evt.Delta.Type == "text_delta" {
				return llm.Token{Content: evt.Delta.Text}, nil
			}
		case "message_stop":
			s.done = true
			return llm.Token{Done: true}, nil
		}
		// skip other event types
	}
}

func (s *streamIter) Close() error {
	return s.stream.Close()
}

// ---------------------------------------------------------------------------
// Error mapping
// ---------------------------------------------------------------------------

func mapError(err error, model string) error {
	var apiErr *anthropic.Error
	if !errors.As(err, &apiErr) {
		return err
	}

	switch apiErr.StatusCode {
	case 401, 403:
		return llmerrors.NewAuth(scheme, err)
	case 429:
		return llmerrors.NewRateLimit(scheme, 0)
	case 404:
		return llmerrors.NewModel(model, scheme)
	default:
		if apiErr.StatusCode >= 500 {
			return llmerrors.NewHTTPStatusError(
				apiErr.StatusCode, fmt.Sprintf("anthropic: %s", err),
			)
		}
	}
	return err
}

// ---------------------------------------------------------------------------
// Registration
// ---------------------------------------------------------------------------

func init() {
	llm.Register(scheme, New)
}
