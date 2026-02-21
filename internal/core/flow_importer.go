package core

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	defaultTimeout = 30 * time.Second
	userAgent      = "TLC-CLI/1.0"
)

type FlowImporter struct {
	httpClient *http.Client
}

type LLMRequest struct {
	Model    string `json:"model"`
	Messages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
	MaxTokens *int `json:"max_tokens,omitempty"`
}

type LLMResponse struct {
	Choices []struct {
		Message struct {
			Role    string `json:"role"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

type ReferencedFile struct {
	Path    string
	Content string
}

func NewFlowImporter() *FlowImporter {
	return &FlowImporter{
		httpClient: &http.Client{
			Timeout: defaultTimeout,
		},
	}
}

func (fi *FlowImporter) ImportFromURL(url string) (*Flow, error) {
	rawURL, err := fi.convertToRawURL(url)
	if err != nil {
		return nil, fmt.Errorf("failed to convert URL: %w", err)
	}

	mainContent, err := fi.fetchContent(rawURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch content: %w", err)
	}

	referencedFiles := fi.fetchReferencedFiles(mainContent, rawURL)

	fullContent := fi.combineContent(mainContent, referencedFiles)

	return fi.convertMarkdownToFlow(fullContent, rawURL)
}

func (fi *FlowImporter) convertToRawURL(url string) (string, error) {
	if strings.HasPrefix(url, "https://raw.githubusercontent.com/") {
		return url, nil
	}

	if !strings.HasPrefix(url, "https://github.com/") {
		return "", fmt.Errorf("unsupported URL: only GitHub URLs are supported")
	}

	re := regexp.MustCompile(`^https://github\.com/([^/]+)/([^/]+)/blob/([^/]+)/(.+)$`)
	matches := re.FindStringSubmatch(url)

	if len(matches) != 5 {
		return "", fmt.Errorf("invalid GitHub URL format")
	}

	return fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/refs/heads/%s/%s",
		matches[1], matches[2], matches[3], matches[4]), nil
}

func (fi *FlowImporter) fetchContent(url string) (string, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", userAgent)

	resp, err := fi.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d: failed to fetch content from %s", resp.StatusCode, url)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	return string(body), nil
}

func (fi *FlowImporter) extractRelativeLinks(content string) []string {
	links := make(map[string]bool)

	mdLinkRe := regexp.MustCompile(`\[([^\]]+)\]\((\.[^\)]+)\)`)
	for _, match := range mdLinkRe.FindAllStringSubmatch(content, -1) {
		if len(match) > 2 {
			link := strings.TrimSpace(match[2])
			if strings.HasSuffix(link, ".md") || strings.HasSuffix(link, ".mdx") {
				links[link] = true
			}
		}
	}

	hrefRe := regexp.MustCompile(`href=["'](\.[^"']+\.md[^"']*)["']`)
	for _, match := range hrefRe.FindAllStringSubmatch(content, -1) {
		if len(match) > 1 {
			link := strings.TrimSpace(match[1])
			if strings.HasSuffix(link, ".md") || strings.HasSuffix(link, ".mdx") {
				links[link] = true
			}
		}
	}

	result := make([]string, 0, len(links))
	for link := range links {
		result = append(result, link)
	}

	return result
}

func (fi *FlowImporter) fetchReferencedFiles(mainContent, mainURL string) []ReferencedFile {
	links := fi.extractRelativeLinks(mainContent)
	if len(links) == 0 {
		return nil
	}

	baseURL := fi.getBaseURL(mainURL)
	prefixPath := fi.getPrefixPath(mainURL)

	var files []ReferencedFile
	for _, link := range links {
		link = strings.TrimPrefix(link, "./")
		rawURL := baseURL + "/" + prefixPath + "/" + link

		content, err := fi.fetchContent(rawURL)
		if err != nil {
			continue
		}

		files = append(files, ReferencedFile{
			Path:    link,
			Content: content,
		})
	}

	return files
}

func (fi *FlowImporter) getBaseURL(githubURL string) string {
	re := regexp.MustCompile(`^(https://raw\.githubusercontent\.com/[^/]+/[^/]+)/refs/heads/[^/]+/.+`)
	matches := re.FindStringSubmatch(githubURL)
	if len(matches) > 1 {
		return matches[1]
	}

	re2 := regexp.MustCompile(`^(https://raw\.githubusercontent\.com/[^/]+/[^/]+/[^/]+)/.+`)
	matches2 := re2.FindStringSubmatch(githubURL)
	if len(matches2) > 1 {
		return matches2[1]
	}

	return ""
}

func (fi *FlowImporter) getPrefixPath(githubURL string) string {
	re := regexp.MustCompile(`https://raw\.githubusercontent\.com/[^/]+/[^/]+/refs/heads/[^/]+/(.+)/[^/]+$`)
	matches := re.FindStringSubmatch(githubURL)
	if len(matches) > 1 {
		return matches[1]
	}

	re2 := regexp.MustCompile(`https://raw\.githubusercontent\.com/[^/]+/[^/]+/[^/]+/(.+)/[^/]+$`)
	matches2 := re2.FindStringSubmatch(githubURL)
	if len(matches2) > 1 {
		return matches2[1]
	}

	return ""
}

func (fi *FlowImporter) combineContent(mainContent string, referencedFiles []ReferencedFile) string {
	if len(referencedFiles) == 0 {
		return mainContent
	}

	var sb strings.Builder
	sb.WriteString("# MAIN DOCUMENT\n\n")
	sb.WriteString(mainContent)

	for _, file := range referencedFiles {
		sb.WriteString("\n\n---\n\n# ")
		sb.WriteString(strings.ReplaceAll(file.Path, "/", " - "))
		sb.WriteString("\n\n")
		sb.WriteString(file.Content)
	}

	return sb.String()
}

func (fi *FlowImporter) convertMarkdownToFlow(content, sourceURL string) (*Flow, error) {
	apiKey := os.Getenv("LLM_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("OPENROUTER_API_KEY")
	}
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
	}
	if apiKey == "" {
		apiKey = os.Getenv("ANTHROPIC_API_KEY")
	}
	if apiKey == "" {
		return nil, fmt.Errorf("LLM_API_KEY, OPENROUTER_API_KEY, OPENAI_API_KEY, or ANTHROPIC_API_KEY environment variable is required")
	}

	apiURL := os.Getenv("LLM_API_URL")
	if apiURL == "" {
		apiURL = os.Getenv("OPENROUTER_API_URL")
	}
	if apiURL == "" {
		if os.Getenv("OPENROUTER_API_KEY") != "" || apiKey != "" {
			apiURL = "https://openrouter.ai/api/v1/chat/completions"
		} else {
			apiURL = "https://api.openai.com/v1/chat/completions"
		}
	}

	model := os.Getenv("LLM_MODEL")
	if model == "" {
		model = os.Getenv("OPENROUTER_MODEL")
	}
	if model == "" {
		model = "anthropic/claude-3-5-sonnet"
	}

	prompt := fi.buildConversionPrompt(content, sourceURL)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	textContent, err := fi.callLLM(ctx, apiURL, apiKey, model, prompt)
	if err != nil {
		return nil, fmt.Errorf("failed to call LLM: %w", err)
	}

	return fi.parseLLMOutput(textContent)
}

func (fi *FlowImporter) buildConversionPrompt(content, sourceURL string) string {
	return fmt.Sprintf(`You are a flow definition expert. Convert the following markdown content (including any referenced files) into a TLC (Task Line CLI) flow specification.

The flow specification must be a valid YAML with this structure:
flow_id: "flow:name:1.0"
name: "Flow Name"
version: "1.0"
description: "Brief description"
entry_step: "start"
steps:
  step-id-1:
    step_id: "step-id-1"
    type: "task"
    title: "Step Title"
    task_template:
      title: "Task title for agents"
      description: "What the agent should do"
      requirements:
        capabilities: ["capability1", "capability2"]
        tools: ["tool1", "tool2"]
  step-id-2:
    step_id: "step-id-2"
    type: "task"
    title: "Another Step"
    depends_on: ["step-id-1"]
    task_template:
      title: "Second task title"
      description: "Description of second task"
      requirements:
        capabilities: ["capability3"]
config:
  category: "planning|creative|analysis|debugging|writing|general"
  procedure: |
    The full original markdown content, preserved verbatim.
    This provides procedural instructions for executing the flow.

IMPORTANT RULES:
1. Extract key steps/tasks from ALL documents (main + referenced files) as sequential flow steps
2. Use depends_on when steps must execute in order
3. Identify required capabilities (e.g., "code-analysis", "documentation", "research", "web-search")
4. Identify required tools (e.g., "grep", "git", "bash", "webfetch")
5. Infer category based on content type
6. Preserve the ENTIRE original markdown in config.procedure (all documents)
7. Return ONLY valid YAML, no explanations or markdown code blocks
8. Source URL: %s

INPUT MARKDOWN (including referenced files):
%s

OUTPUT (YAML only):`, sourceURL, content)
}

func (fi *FlowImporter) callLLM(ctx context.Context, apiURL, apiKey, model, prompt string) (string, error) {
	maxTokens := 8192
	reqBody := LLMRequest{
		Model: model,
		Messages: []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}{
			{
				Role:    "user",
				Content: prompt,
			},
		},
		MaxTokens: &maxTokens,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("HTTP-Referer", "https://github.com/hop-top/tlc")

	resp, err := fi.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to call LLM API: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("LLM API returned status %d: %s", resp.StatusCode, string(body))
	}

	var llmResp LLMResponse
	if err := json.Unmarshal(body, &llmResp); err != nil {
		return "", fmt.Errorf("failed to unmarshal LLM response: %w", err)
	}

	if len(llmResp.Choices) == 0 {
		return "", fmt.Errorf("no choices in LLM response")
	}

	choice := llmResp.Choices[0]
	if len(choice.Message.Content) == 0 {
		return "", fmt.Errorf("no content in LLM response")
	}

	for _, block := range choice.Message.Content {
		if block.Type == "text" {
			return strings.TrimSpace(block.Text), nil
		}
	}

	return "", fmt.Errorf("no text block in LLM response")
}

func (fi *FlowImporter) parseLLMOutput(textContent string) (*Flow, error) {
	if textContent == "" {
		return nil, fmt.Errorf("LLM returned empty content")
	}

	var flow Flow
	if err := yaml.Unmarshal([]byte(textContent), &flow); err != nil {
		return nil, fmt.Errorf("failed to parse LLM output as YAML: %w\nLLM Output:\n%s", err, textContent)
	}

	if flow.ID == "" || flow.Name == "" || len(flow.Steps) == 0 {
		return nil, fmt.Errorf("invalid flow structure: missing required fields (id, name, or steps)")
	}

	return &flow, nil
}
