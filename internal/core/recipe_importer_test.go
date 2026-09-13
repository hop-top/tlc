package core

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeLLM answers each Complete call from answers in order, repeating the
// last one, and records every prompt it saw.
type fakeLLM struct {
	answers []string
	prompts []string
	err     error
}

func (f *fakeLLM) Complete(_ context.Context, prompt string) (string, error) {
	f.prompts = append(f.prompts, prompt)
	if f.err != nil {
		return "", f.err
	}
	i := min(len(f.prompts)-1, len(f.answers)-1)
	return f.answers[i], nil
}

const importedRecipeYAML = "```yaml\nrecipe: deploy\nversion: 0.1.0\nvars:\n  env: {required: true}\nsteps:\n  - id: build\n    title: Build\n  - id: ship\n    title: Ship to {{env}}\n    depends_on: [build]\n```"

const importedBadYAML = "recipe: deploy\nversion: 0.1.0\nsteps:\n  - id: ship\n    depends_on: [build]\n"

func TestParseImportedRecipe_Valid(t *testing.T) {
	llm := &fakeLLM{answers: []string{importedRecipeYAML}}

	rec, err := parseImportedRecipe(context.Background(), llm, recipeImportPrompt("# Deploy\n\nBuild, then ship.", "https://example.test/deploy.md"))
	if err != nil {
		t.Fatalf("parseImportedRecipe: %v", err)
	}
	if rec.Name != "deploy" || len(rec.Steps) != 2 || rec.Steps[1].DependsOn[0] != "build" {
		t.Errorf("recipe = %+v; want the fenced document parsed", rec)
	}
	if len(llm.prompts) != 1 {
		t.Fatalf("model calls = %d; want one on a valid answer", len(llm.prompts))
	}
	for _, want := range []string{"recipe:", "depends_on", "https://example.test/deploy.md", "Build, then ship."} {
		if !strings.Contains(llm.prompts[0], want) {
			t.Errorf("prompt lacks %q:\n%s", want, llm.prompts[0])
		}
	}
	if strings.Contains(llm.prompts[0], "flow_id") || strings.Contains(llm.prompts[0], "entry_step") {
		t.Errorf("prompt still describes the flow format:\n%s", llm.prompts[0])
	}
}

func TestParseImportedRecipe_InvalidSchema(t *testing.T) {
	llm := &fakeLLM{answers: []string{importedBadYAML}}

	_, err := parseImportedRecipe(context.Background(), llm, "prompt")
	if err == nil {
		t.Fatal("expected an error when both answers are invalid")
	}
	if !strings.Contains(err.Error(), "build") || !strings.Contains(err.Error(), "retry") {
		t.Errorf("err = %v; want the dangling dependency and the retry mentioned", err)
	}
	if len(llm.prompts) != 2 {
		t.Errorf("model calls = %d; want exactly one retry", len(llm.prompts))
	}

	llm = &fakeLLM{err: errors.New("boom")}
	if _, err := parseImportedRecipe(context.Background(), llm, "prompt"); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err = %v; want the model failure surfaced", err)
	}
}

func TestParseImportedRecipe_RetryFeedsError(t *testing.T) {
	llm := &fakeLLM{answers: []string{importedBadYAML, importedRecipeYAML}}

	rec, err := parseImportedRecipe(context.Background(), llm, "prompt")
	if err != nil {
		t.Fatalf("parseImportedRecipe: %v", err)
	}
	if rec.Name != "deploy" || len(rec.Steps) != 2 {
		t.Errorf("recipe = %+v; want the corrected answer", rec)
	}
	if len(llm.prompts) != 2 {
		t.Fatalf("model calls = %d; want the original and one retry", len(llm.prompts))
	}
	retry := llm.prompts[1]
	if !strings.HasPrefix(retry, "prompt") {
		t.Errorf("retry prompt does not restate the task:\n%s", retry)
	}
	for _, want := range []string{"unknown step \"build\"", importedBadYAML} {
		if !strings.Contains(retry, want) {
			t.Errorf("retry prompt lacks %q:\n%s", want, retry)
		}
	}
}

func TestRecipeImporter_ImportFromURL_PlanKeepsDocument(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/docs/deploy.md":
			_, _ = w.Write([]byte("# Deploy\n\nSee [checks](./checks.md) and [ext](https://other.test/x.md).\n"))
		case "/docs/checks.md":
			_, _ = w.Write([]byte("# Checks\n\nRun the suite.\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	llm := &fakeLLM{answers: []string{importedRecipeYAML}}

	rec, err := NewRecipeImporter(llm).ImportFromURL(context.Background(), srv.URL+"/docs/deploy.md")
	if err != nil {
		t.Fatalf("ImportFromURL: %v", err)
	}
	if rec.Track == nil || !strings.Contains(rec.Track.Plan, "# Deploy") || !strings.Contains(rec.Track.Plan, "Run the suite.") {
		t.Errorf("track.plan = %+v; want the main document and the linked file", rec.Track)
	}
	if !strings.Contains(llm.prompts[0], "Run the suite.") {
		t.Errorf("prompt lacks the linked file:\n%s", llm.prompts[0])
	}

	_, err = NewRecipeImporter(llm).ImportFromURL(context.Background(), srv.URL+"/missing.md")
	if err == nil || !strings.Contains(err.Error(), "404") {
		t.Errorf("err = %v; want the HTTP status", err)
	}
}

func TestRawContentURL(t *testing.T) {
	cases := map[string]string{
		"https://github.com/o/r/blob/main/docs/a.md":       "https://raw.githubusercontent.com/o/r/main/docs/a.md",
		"https://raw.githubusercontent.com/o/r/main/a.md":  "https://raw.githubusercontent.com/o/r/main/a.md",
		"https://example.test/procedure.md":                "https://example.test/procedure.md",
		"https://github.com/o/r/tree/main/docs/a.md":       "https://github.com/o/r/tree/main/docs/a.md",
		"http://localhost:8080/a.md":                       "http://localhost:8080/a.md",
		"https://github.com/o/r/blob/feature/x/docs/a.mdx": "https://raw.githubusercontent.com/o/r/feature/x/docs/a.mdx",
	}
	for in, want := range cases {
		got, err := rawContentURL(in)
		if err != nil || got != want {
			t.Errorf("rawContentURL(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := rawContentURL("ftp://x/a.md"); err == nil {
		t.Error("expected an error for a non-http scheme")
	}
}
