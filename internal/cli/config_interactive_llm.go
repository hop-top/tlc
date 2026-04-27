package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"charm.land/huh/v2"

	"hop.top/kit/go/ai/llm"
	llmerrors "hop.top/kit/go/ai/llm/errors"
)

// providerAuthInfo maps URI schemes to auth requirements.
type providerAuthInfo struct {
	EnvVar   string // env var for API key (empty = no key needed)
	DocsURL  string // docs / signup URL
	NeedsKey bool
}

var providerAuth = map[string]providerAuthInfo{
	"openai": {
		EnvVar:   "OPENAI_API_KEY",
		DocsURL:  "https://platform.openai.com/api-keys",
		NeedsKey: true,
	},
	"anthropic": {
		EnvVar:   "ANTHROPIC_API_KEY",
		DocsURL:  "https://console.anthropic.com/settings/keys",
		NeedsKey: true,
	},
	"openrouter": {
		EnvVar:   "OPENROUTER_API_KEY",
		DocsURL:  "https://openrouter.ai/keys",
		NeedsKey: true,
	},
	"ollama": {
		NeedsKey: false,
	},
	"xai": {
		EnvVar:   "XAI_API_KEY",
		DocsURL:  "https://console.x.ai",
		NeedsKey: true,
	},
	"lmstudio": {
		NeedsKey: false,
	},
}

// validateLLMProvider checks if the selected provider URI is usable.
// Tries to resolve + ping. On auth failure, prompts for API key.
// On connection failure, shows remediation hint.
func validateLLMProvider(uri string, w io.Writer) (string, error) {
	if uri == "" {
		return uri, nil
	}

	parsed, err := llm.ParseURI(uri)
	if err != nil {
		return uri, fmt.Errorf("invalid URI %q: %w", uri, err)
	}

	info := providerAuth[parsed.Scheme]

	// Try to resolve (creates provider instance).
	provider, err := llm.Resolve(uri)
	if err != nil {
		// Resolve failed — likely missing API key (anthropic checks at New).
		if info.NeedsKey {
			return handleMissingKey(uri, parsed.Scheme, info, w)
		}
		return uri, fmt.Errorf(
			"provider %q unavailable: %w", parsed.Scheme, err,
		)
	}
	defer provider.Close()

	// Try a minimal completion to verify connectivity + auth.
	comp, ok := provider.(llm.Completer)
	if !ok {
		// Provider doesn't support Complete — accept it.
		fmt.Fprintf(w, "\n  %s configured (cannot verify: no complete capability)\n", uri)
		return uri, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = comp.Complete(ctx, llm.Request{
		Messages:  []llm.Message{{Role: "user", Content: "ping"}},
		MaxTokens: 1,
	})
	if err == nil {
		fmt.Fprintf(w, "\n  ✓ %s verified\n", uri)
		return uri, nil
	}

	// Classify the error.
	var authErr *llmerrors.ErrAuth
	if errors.As(err, &authErr) {
		if info.NeedsKey {
			return handleMissingKey(uri, parsed.Scheme, info, w)
		}
		return uri, fmt.Errorf(
			"auth failed for %q: %w; check credentials", parsed.Scheme, err,
		)
	}

	var modelErr *llmerrors.ErrModel
	if errors.As(err, &modelErr) {
		fmt.Fprintf(w, "\n  ⚠ model %q not found on %s (provider accepted, model may need pulling)\n",
			parsed.Model, parsed.Scheme)
		if parsed.Scheme == "ollama" {
			fmt.Fprintf(w, "  run: ollama pull %s\n", parsed.Model)
		}
		return uri, nil // accept — user can fix model later
	}

	// Connection / network error (likely ollama not running).
	if parsed.Scheme == "ollama" {
		fmt.Fprintf(w, "\n  ⚠ cannot reach ollama; run: ollama serve\n")
		return uri, nil // accept — user can start later
	}

	fmt.Fprintf(w, "\n  ⚠ connection failed: %s (saved anyway)\n", err)
	return uri, nil
}

// handleMissingKey prompts the user for an API key via huh.
func handleMissingKey(
	uri, scheme string, info providerAuthInfo, w io.Writer,
) (string, error) {
	fmt.Fprintf(w, "\n  %s requires an API key\n", scheme)
	if info.DocsURL != "" {
		fmt.Fprintf(w, "  Get one at: %s\n", info.DocsURL)
	}

	var choice string
	opts := []huh.Option[string]{
		huh.NewOption[string](
			fmt.Sprintf("Set %s now", info.EnvVar), "env"),
		huh.NewOption[string]("Enter API key", "key"),
		huh.NewOption[string]("Skip (configure later)", "skip"),
	}

	err := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Authentication required").
				Options(opts...).
				Value(&choice),
		),
	).WithOutput(w).Run()
	if err != nil {
		return uri, nil // aborted — keep URI anyway
	}

	switch choice {
	case "env":
		fmt.Fprintf(w, "\n  Add to your shell profile:\n")
		fmt.Fprintf(w, "    export %s=<your-key>\n", info.EnvVar)
		fmt.Fprintf(w, "  Then restart your terminal or run:\n")
		fmt.Fprintf(w, "    source ~/.zshrc\n")
		return uri, nil

	case "key":
		var key string
		err := huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title(info.EnvVar).
					Placeholder("sk-...").
					EchoMode(huh.EchoModePassword).
					Value(&key),
			),
		).WithOutput(w).Run()
		if err != nil || strings.TrimSpace(key) == "" {
			return uri, nil
		}

		// Append api_key to URI params.
		if strings.Contains(uri, "?") {
			uri += "&api_key=" + strings.TrimSpace(key)
		} else {
			uri += "?api_key=" + strings.TrimSpace(key)
		}
		fmt.Fprintf(w, "\n  ✓ API key added to provider URI\n")
		fmt.Fprintf(w, "  (key stored in config file; for security, prefer %s env var)\n",
			info.EnvVar)
		return uri, nil

	default: // skip
		return uri, nil
	}
}
