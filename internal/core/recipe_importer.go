package core

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// LLMClient completes a prompt. The HTTP client reads its endpoint, key
// and model from the environment; tests substitute a fake.
type LLMClient interface {
	Complete(ctx context.Context, prompt string) (string, error)
}

// RecipeImporter turns a markdown procedure reachable by URL into a
// recipe: the document (and the relative markdown files it links) is
// fetched, a model rewrites it in the recipe schema, and the answer is
// parsed and validated like any recipe file. The original text is kept
// as track.plan so nothing the procedure said is lost.
type RecipeImporter struct {
	llm    LLMClient
	source *markdownSource
}

// NewRecipeImporter builds an importer that asks llm for the conversion.
func NewRecipeImporter(llm LLMClient) *RecipeImporter {
	return &RecipeImporter{
		llm:    llm,
		source: &markdownSource{client: &http.Client{Timeout: importFetchTimeout}},
	}
}

// ImportFromURL fetches url (a GitHub blob or any http(s) markdown URL)
// and returns the recipe the model describes it as.
func (ri *RecipeImporter) ImportFromURL(ctx context.Context, url string) (*Recipe, error) {
	content, err := ri.source.document(ctx, url)
	if err != nil {
		return nil, err
	}
	rec, err := parseImportedRecipe(ctx, ri.llm, recipeImportPrompt(content, url))
	if err != nil {
		return nil, fmt.Errorf("import %s: %w", url, err)
	}
	if rec.Track == nil {
		rec.Track = &RecipeTrack{}
	}
	rec.Track.Plan = content
	return rec, nil
}

// parseImportedRecipe asks llm for the recipe described by prompt and
// parses the answer; a parse or validation failure is fed back once so
// the model can correct its own output.
func parseImportedRecipe(ctx context.Context, llm LLMClient, prompt string) (*Recipe, error) {
	answer, err := llm.Complete(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("model call failed: %w", err)
	}
	rec, parseErr := parseModelRecipe(answer)
	if parseErr == nil {
		return rec, nil
	}
	answer, err = llm.Complete(ctx, recipeImportRetryPrompt(prompt, answer, parseErr))
	if err != nil {
		return nil, fmt.Errorf("model retry failed: %w", err)
	}
	rec, err = parseModelRecipe(answer)
	if err != nil {
		return nil, fmt.Errorf("model output is not a valid recipe after one retry: %w", err)
	}
	return rec, nil
}

// parseModelRecipe parses a model answer as a recipe document, tolerating
// a markdown code fence around it.
func parseModelRecipe(answer string) (*Recipe, error) {
	text := stripCodeFence(strings.TrimSpace(answer))
	if text == "" {
		return nil, errors.New("model returned no content")
	}
	rec, err := ParseRecipe(strings.NewReader(text), importedRecipeName)
	if err != nil {
		return nil, err
	}
	return rec, nil
}

// importedRecipeName is the file name ParseRecipe reports in errors for a
// model answer; the .yaml extension selects the YAML decoder.
const importedRecipeName = "model-answer.yaml"

// stripCodeFence drops a ```lang ... ``` wrapper; a bare document is
// returned as is.
func stripCodeFence(s string) string {
	if !strings.HasPrefix(s, "```") {
		return s
	}
	_, body, _ := strings.Cut(s, "\n")
	body = strings.TrimSuffix(strings.TrimSpace(body), "```")
	return strings.TrimSpace(body)
}

func recipeImportPrompt(content, sourceURL string) string {
	return fmt.Sprintf(recipeImportPromptTemplate, sourceURL, content)
}

func recipeImportRetryPrompt(prompt, answer string, cause error) string {
	return fmt.Sprintf(recipeImportRetryTemplate, prompt, cause, answer)
}

// recipeImportPromptTemplate describes the recipe format the model must
// emit; %s takes the source URL then the document.
const recipeImportPromptTemplate = `You convert a markdown procedure into a tlc recipe: a versioned YAML template that creates a track of tasks.

Output exactly one YAML document with this shape:

recipe: <name>                 # lowercase letters, digits and hyphens, derived from the document title
version: 0.1.0
description: <one line>
vars:                          # inputs the procedure needs; omit the key when there are none
  <name>: {description: <what it is>, required: true}
track:
  title: <track title>
  type: feature
steps:
  - id: <step-id>              # lowercase letters, digits and hyphens; unique
    title: <imperative title>
    description: |
      <what to do, in enough detail for an agent to act on it>
    depends_on: [<earlier step ids>]   # only for real ordering constraints
    kind: agent                # agent (default), exec for a literal command, human for a sign-off

Rules:
1. One step per distinct action in the document, in the document's order; use depends_on only where the procedure requires an order.
2. Reference inputs as {{name}} in titles and descriptions and declare every one under vars.
3. kind: exec steps need exec: {argv: [<command>, <args>...]}; kind: human steps may carry human: {assignee: "@role"}.
4. Do not copy the original document into the recipe; it is attached separately.
5. Return only the YAML: no code fence, no commentary.

Source URL: %s

DOCUMENT:
%s`

// recipeImportRetryTemplate restates the task with the validation failure
// and the rejected answer; %s takes the prompt, the error, the answer.
const recipeImportRetryTemplate = `%s

Your previous answer was rejected by the recipe validator:
%v

Previous answer:
%s

Return the corrected YAML only.`
