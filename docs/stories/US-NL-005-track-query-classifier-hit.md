# US-NL-005 - NL Prompt — Cross-domain Classifier: Track Queries

**ID**: US-NL-005
**Feature**: NL Prompt — Cross-domain Classifier
**Persona**: [AI Agent](../personas/ai-agent.md)
**Related Personas**: [Solo Developer](../personas/solo-developer.md)
**Priority**: P1

## Story

As an AI Agent, I want `"list active tracks"` to resolve without LLM
so responses are instant and cost-free.

## Acceptance Scenarios

1. **Given** a natural language prompt `"list active tracks"`,
   **When** the classifier pipeline runs,
   **Then** `ClassifyPromptCrossDomain` resolves to `track list --status active`
   with confidence ≥ 0.9, and no LLM call is made.

2. **Given** a prompt `"count active tracks"`,
   **When** the classifier pipeline runs,
   **Then** resolves to `track list --status active --counters`
   with confidence ≥ 0.9, no LLM call.

3. **Given** a prompt `"stale tracks"`,
   **When** the classifier pipeline runs,
   **Then** resolves to `track list --stale` with confidence ≥ 0.9, no LLM call.

## Tests

### E2E
- `internal/cli/task_prompt_e2e_test.go`
  - `TestCrossDomainE2E_TrackQueryNoLLM`
  - `TestCrossDomainE2E_CountActiveTracksNoLLM`

### Unit
- `internal/cli/prompt_domain_test.go`
  - `TestClassifyPromptCrossDomain_ListTracks`
  - `TestClassifyPromptCrossDomain_CountActiveTracks`
  - `TestClassifyPromptCrossDomain_ActiveTracks`
  - `TestClassifyPromptCrossDomain_StaleTracks`
  - `TestBuildCommand_TrackList`
  - `TestBuildCommand_TrackStatusDoneModifier`
