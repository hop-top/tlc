# US-NL-007 - NL Prompt — Cross-domain Classifier: Typo-tolerant Resolution

**ID**: US-NL-007
**Feature**: NL Prompt — Cross-domain Classifier
**Persona**: [AI Agent](../personas/ai-agent.md)
**Related Personas**: [Solo Developer](../personas/solo-developer.md)
**Priority**: P2

## Story

As an AI Agent, I want typo-tolerant prompts like `"list trakcs"` to still
resolve correctly so typos don't force LLM escalation.

## Acceptance Scenarios

1. **Given** a prompt `"list trakcs"` (typo: trakcs vs tracks, distance 2),
   **When** the classifier pipeline runs,
   **Then** `ClassifyPromptCrossDomain` resolves to `track list`
   with confidence ≥ 0.75 (fuzzy match, distance 2 → 0.8), no LLM call.

2. **Given** a prompt `"compleet T-42"` (typo in verb),
   **When** the classifier pipeline runs,
   **Then** the existing regex classifier (`ClassifyPrompt`) handles it
   with confidence 1.0 before the cross-domain path is reached.

3. **Given** a prompt with a noun typo of distance >2 (e.g., `"list trakkkks"`),
   **When** the classifier pipeline runs,
   **Then** `ClassifyPromptCrossDomain` returns nil and the pipeline falls
   through to LLM escalation.

## Tests

### E2E
- `internal/cli/task_prompt_e2e_test.go`
  - `TestCrossDomainE2E_FuzzyTypoNoLLM`

### Unit
- `internal/cli/prompt_fuzzy_test.go`
  - `TestFuzzyMatchNoun_*`
  - `TestLevenshteinDistance_*`
- `internal/cli/prompt_domain_test.go`
  - `TestClassifyPromptCrossDomain_FuzzyNoun`
