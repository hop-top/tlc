// Package openai adapts openai/openai-go to the llm provider interfaces.
//
// Registers schemes: openai, openrouter, xai, lmstudio.
// Implements [llm.Completer], [llm.Streamer], [llm.ToolCaller].
package openai

import (
	"context"
	"encoding/json"
	"errors"
	"io"

	oai "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/packages/param"
	"github.com/openai/openai-go/packages/ssestream"
	"github.com/openai/openai-go/shared"

	"hop.top/kit/llm"
	llmerrors "hop.top/kit/llm/errors"
)

// schemes registered by this adapter.
var schemes = []string{"openai", "openrouter", "xai", "lmstudio"}

func init() {
	for _, s := range schemes {
		llm.Register(s, New)
	}
}

// Adapter wraps an openai-go client and model name.
type Adapter struct {
	client oai.Client
	model  string
	scheme string
}

// compile-time interface checks.
var (
	_ llm.Provider   = (*Adapter)(nil)
	_ llm.Completer  = (*Adapter)(nil)
	_ llm.Streamer   = (*Adapter)(nil)
	_ llm.ToolCaller = (*Adapter)(nil)
)

// New creates an Adapter from the resolved config.
func New(cfg llm.ResolvedConfig) (llm.Provider, error) {
	opts := []option.RequestOption{
		option.WithAPIKey(cfg.Provider.APIKey),
	}

	base := cfg.Provider.BaseURL
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	opts = append(opts, option.WithBaseURL(base))

	model := cfg.Provider.Model
	if model == "" {
		model = cfg.URI.Model
	}

	return &Adapter{
		client: oai.NewClient(opts...),
		model:  model,
		scheme: cfg.URI.Scheme,
	}, nil
}

// Close is a no-op; the HTTP client has no persistent connections to tear down.
func (a *Adapter) Close() error { return nil }

func (a *Adapter) effectiveModel(req llm.Request) string {
	if req.Model != "" {
		return req.Model
	}
	return a.model
}

// Complete maps an llm.Request to an OpenAI ChatCompletion and back.
func (a *Adapter) Complete(
	ctx context.Context, req llm.Request,
) (llm.Response, error) {
	model := a.effectiveModel(req)
	params := a.buildParams(req)
	comp, err := a.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return llm.Response{}, mapError(err, a.scheme, model)
	}
	return mapCompletion(comp), nil
}

// Stream returns a TokenIterator wrapping an OpenAI streaming response.
func (a *Adapter) Stream(
	ctx context.Context, req llm.Request,
) (llm.TokenIterator, error) {
	model := a.effectiveModel(req)
	params := a.buildParams(req)
	stream := a.client.Chat.Completions.NewStreaming(ctx, params)
	// Check for immediate errors (e.g. connection refused).
	if err := stream.Err(); err != nil {
		_ = stream.Close()
		return nil, mapError(err, a.scheme, model)
	}
	return &streamIter{stream: stream, scheme: a.scheme, model: model}, nil
}

// CallWithTools maps ToolDefs to OpenAI function tools and returns tool calls.
func (a *Adapter) CallWithTools(
	ctx context.Context, req llm.Request, tools []llm.ToolDef,
) (llm.ToolResponse, error) {
	params := a.buildParams(req)
	params.Tools = mapTools(tools)

	comp, err := a.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return llm.ToolResponse{}, mapError(err, a.scheme, a.effectiveModel(req))
	}

	return mapToolResponse(comp), nil
}

// ---------- param building ----------

func (a *Adapter) buildParams(req llm.Request) oai.ChatCompletionNewParams {
	model := a.model
	if req.Model != "" {
		model = req.Model
	}

	p := oai.ChatCompletionNewParams{
		Model:    model,
		Messages: mapMessages(req.Messages),
	}
	if req.Temperature != 0 {
		p.Temperature = param.NewOpt(req.Temperature)
	}
	if req.MaxTokens > 0 {
		p.MaxTokens = param.NewOpt(int64(req.MaxTokens))
	}
	if len(req.StopSequences) > 0 {
		p.Stop = oai.ChatCompletionNewParamsStopUnion{
			OfStringArray: req.StopSequences,
		}
	}
	return p
}

func mapMessages(
	msgs []llm.Message,
) []oai.ChatCompletionMessageParamUnion {
	out := make([]oai.ChatCompletionMessageParamUnion, 0, len(msgs))
	for _, m := range msgs {
		switch m.Role {
		case "system":
			out = append(out, oai.SystemMessage(m.Content))
		case "assistant":
			out = append(out, oai.AssistantMessage(m.Content))
		default: // "user" or unknown
			out = append(out, oai.UserMessage(m.Content))
		}
	}
	return out
}

func mapTools(defs []llm.ToolDef) []oai.ChatCompletionToolParam {
	out := make([]oai.ChatCompletionToolParam, 0, len(defs))
	for _, d := range defs {
		var params shared.FunctionParameters
		if d.Parameters != nil {
			_ = json.Unmarshal(d.Parameters, &params)
		}
		fd := shared.FunctionDefinitionParam{
			Name:       d.Name,
			Parameters: params,
		}
		if d.Description != "" {
			fd.Description = param.NewOpt(d.Description)
		}
		out = append(out, oai.ChatCompletionToolParam{Function: fd})
	}
	return out
}

// ---------- response mapping ----------

func mapCompletion(c *oai.ChatCompletion) llm.Response {
	var resp llm.Response
	if len(c.Choices) > 0 {
		resp.Content = c.Choices[0].Message.Content
		resp.Role = string(c.Choices[0].Message.Role)
		resp.FinishReason = c.Choices[0].FinishReason
	}
	resp.Usage = llm.Usage{
		PromptTokens:     int(c.Usage.PromptTokens),
		CompletionTokens: int(c.Usage.CompletionTokens),
		TotalTokens:      int(c.Usage.TotalTokens),
	}
	return resp
}

func mapToolResponse(c *oai.ChatCompletion) llm.ToolResponse {
	var resp llm.ToolResponse
	if len(c.Choices) > 0 {
		resp.Content = c.Choices[0].Message.Content
		for _, tc := range c.Choices[0].Message.ToolCalls {
			resp.ToolCalls = append(resp.ToolCalls, llm.ToolCall{
				ID:        tc.ID,
				Name:      tc.Function.Name,
				Arguments: json.RawMessage(tc.Function.Arguments),
			})
		}
	}
	return resp
}

// ---------- streaming ----------

type streamIter struct {
	stream *ssestream.Stream[oai.ChatCompletionChunk]
	scheme string
	model  string
	done   bool
}

func (s *streamIter) Next() (llm.Token, error) {
	if s.done {
		return llm.Token{}, io.EOF
	}
	if !s.stream.Next() {
		s.done = true
		err := s.stream.Err()
		if err != nil {
			return llm.Token{}, mapError(err, s.scheme, s.model)
		}
		return llm.Token{Done: true}, nil
	}

	chunk := s.stream.Current()
	var content string
	if len(chunk.Choices) > 0 {
		content = chunk.Choices[0].Delta.Content
	}
	return llm.Token{Content: content}, nil
}

func (s *streamIter) Close() error {
	return s.stream.Close()
}

// ---------- error mapping ----------

func mapError(err error, scheme, model string) error {
	var apiErr *oai.Error
	if errors.As(err, &apiErr) {
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
					apiErr.StatusCode, apiErr.Message,
				)
			}
		}
	}
	return err
}
