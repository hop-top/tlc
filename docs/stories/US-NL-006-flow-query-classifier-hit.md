---
status: shipped-no-e2e
---

# US-NL-006 - NL Prompt — Cross-domain Classifier: Flow Queries

**ID**: US-NL-006
**Feature**: NL Prompt — Cross-domain Classifier
**Persona**: [AI Agent](../personas/ai-agent.md)
**Related Personas**: [Solo Developer](../personas/solo-developer.md)
**Priority**: P1

## Story

As an AI Agent, I want `"run deploy flow"` to resolve without LLM
so flow invocations are instant and cost-free.

## Acceptance Scenarios

1. **Given** a natural language prompt `"run deploy flow"`,
   **When** the classifier pipeline runs,
   **Then** `ClassifyPromptCrossDomain` resolves to `flow run deploy`
   with confidence ≥ 0.9, and no LLM call is made.

2. **Given** a prompt `"list flows"`,
   **When** the classifier pipeline runs,
   **Then** resolves to `flow list` with confidence ≥ 0.9, no LLM call.

## Tests

### E2E
- `internal/cli/task_prompt_e2e_test.go`
  - `TestCrossDomainE2E_FlowRunNoLLM`

### Unit
- `internal/cli/prompt_domain_test.go`
  - `TestClassifyPromptCrossDomain_RunDeployFlow`
  - `TestClassifyPromptCrossDomain_ListFlows`
  - `TestClassifyPromptCrossDomain_ShowFlows`
