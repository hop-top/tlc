package core

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

const (
	importFetchTimeout = 30 * time.Second
	importModelTimeout = 5 * time.Minute
	importUserAgent    = "tlc recipe import"
	importMaxBytes     = 4 << 20
	importMaxTokens    = 8192
	importErrorExcerpt = 512

	defaultLLMURL   = "https://openrouter.ai/api/v1/chat/completions"
	defaultLLMModel = "anthropic/claude-3-5-sonnet"
)

// markdownSource fetches a markdown document and the relative markdown
// files it links to.
type markdownSource struct {
	client *http.Client
}

// document fetches url and every relative markdown link it carries,
// concatenated with a heading per file so the model sees one text. A
// linked file that cannot be fetched is skipped: the main document is
// what the procedure is.
func (m *markdownSource) document(ctx context.Context, url string) (string, error) {
	raw, err := rawContentURL(url)
	if err != nil {
		return "", err
	}
	main, err := m.fetch(ctx, raw)
	if err != nil {
		return "", err
	}
	links := relativeMarkdownLinks(main)
	if len(links) == 0 {
		return main, nil
	}
	base, err := neturl.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("resolve links of %s: %w", raw, err)
	}
	var sb strings.Builder
	sb.WriteString("# MAIN DOCUMENT\n\n")
	sb.WriteString(main)
	for _, link := range links {
		ref, err := neturl.Parse(link)
		if err != nil {
			continue
		}
		body, err := m.fetch(ctx, base.ResolveReference(ref).String())
		if err != nil {
			continue
		}
		fmt.Fprintf(&sb, "\n\n---\n\n# %s\n\n%s", link, body)
	}
	return sb.String(), nil
}

func (m *markdownSource) fetch(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("build request for %s: %w", url, err)
	}
	req.Header.Set("User-Agent", importUserAgent)
	resp, err := m.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch %s: HTTP %d", url, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, importMaxBytes))
	if err != nil {
		return "", fmt.Errorf("read %s: %w", url, err)
	}
	return string(body), nil
}

var githubBlobRe = regexp.MustCompile(`^https://github\.com/([^/]+)/([^/]+)/blob/([^/]+)/(.+)$`)

// rawContentURL maps a GitHub blob URL to its raw form; other http(s)
// URLs pass through.
func rawContentURL(url string) (string, error) {
	if m := githubBlobRe.FindStringSubmatch(url); m != nil {
		return fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/%s", m[1], m[2], m[3], m[4]), nil
	}
	if strings.HasPrefix(url, "https://") || strings.HasPrefix(url, "http://") {
		return url, nil
	}
	return "", fmt.Errorf(
		"unsupported source %q: want an http(s) URL to a markdown file (GitHub blob URLs are read raw)", url,
	)
}

// markdownLinkRe matches relative .md/.mdx targets in markdown links and
// html hrefs.
var markdownLinkRe = regexp.MustCompile(`\]\((\.{1,2}/[^)\s]+\.mdx?)\)|href=["'](\.{1,2}/[^"']+\.mdx?)["']`)

// relativeMarkdownLinks lists the relative markdown files content links
// to, in document order, without duplicates.
func relativeMarkdownLinks(content string) []string {
	seen := map[string]bool{}
	var links []string
	for _, m := range markdownLinkRe.FindAllStringSubmatch(content, -1) {
		link := m[1]
		if link == "" {
			link = m[2]
		}
		if !seen[link] {
			seen[link] = true
			links = append(links, link)
		}
	}
	return links
}

// httpLLMClient speaks the chat-completions shape shared by OpenRouter,
// OpenAI and most gateways.
type httpLLMClient struct {
	client *http.Client
	url    string
	key    string
	model  string
}

// LLMClientFromEnv builds the HTTP client from LLM_API_KEY (or
// OPENROUTER_API_KEY, OPENAI_API_KEY, ANTHROPIC_API_KEY), LLM_API_URL
// (or OPENROUTER_API_URL) and LLM_MODEL (or OPENROUTER_MODEL).
func LLMClientFromEnv() (LLMClient, error) {
	key := firstEnv("LLM_API_KEY", "OPENROUTER_API_KEY", "OPENAI_API_KEY", "ANTHROPIC_API_KEY")
	if key == "" {
		return nil, errors.New(
			"no model API key; set LLM_API_KEY (or OPENROUTER_API_KEY, OPENAI_API_KEY, ANTHROPIC_API_KEY)",
		)
	}
	url := firstEnv("LLM_API_URL", "OPENROUTER_API_URL")
	if url == "" {
		url = defaultLLMURL
	}
	model := firstEnv("LLM_MODEL", "OPENROUTER_MODEL")
	if model == "" {
		model = defaultLLMModel
	}
	return &httpLLMClient{client: &http.Client{Timeout: importModelTimeout}, url: url, key: key, model: model}, nil
}

func firstEnv(names ...string) string {
	for _, name := range names {
		if v := os.Getenv(name); v != "" {
			return v
		}
	}
	return ""
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model     string        `json:"model"`
	Messages  []chatMessage `json:"messages"`
	MaxTokens int           `json:"max_tokens,omitempty"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content json.RawMessage `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// Complete posts prompt as a single user message and returns the text of
// the first choice.
func (c *httpLLMClient) Complete(ctx context.Context, prompt string) (string, error) {
	body, err := json.Marshal(chatRequest{
		Model: c.model, Messages: []chatMessage{{Role: "user", Content: prompt}}, MaxTokens: importMaxTokens,
	})
	if err != nil {
		return "", fmt.Errorf("encode model request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build model request for %s: %w", c.url, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.key)
	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("call model endpoint %s: %w", c.url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, importMaxBytes))
	if err != nil {
		return "", fmt.Errorf("read model reply: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("model endpoint %s returned HTTP %d: %s", c.url, resp.StatusCode, excerpt(data))
	}
	var out chatResponse
	if err := json.Unmarshal(data, &out); err != nil {
		return "", fmt.Errorf("decode model reply: %w", err)
	}
	if len(out.Choices) == 0 {
		return "", errors.New("model reply has no choices")
	}
	return contentText(out.Choices[0].Message.Content)
}

// contentText accepts both the string form and the block-list form of a
// message content.
func contentText(raw json.RawMessage) (string, error) {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return strings.TrimSpace(s), nil
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return "", fmt.Errorf("model reply content has an unknown shape: %w", err)
	}
	var sb strings.Builder
	for _, b := range blocks {
		if b.Type == "text" {
			sb.WriteString(b.Text)
		}
	}
	return strings.TrimSpace(sb.String()), nil
}

func excerpt(data []byte) string {
	s := strings.TrimSpace(string(data))
	if len(s) > importErrorExcerpt {
		return s[:importErrorExcerpt] + "…"
	}
	return s
}
