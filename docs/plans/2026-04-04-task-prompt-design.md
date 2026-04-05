# Task Prompt Design

## Task List

- [ ] Local classifier: regex/keyword → `[]ResolvedCommand` + confidence
- [ ] kit/llm router: provider-agnostic via hop.top/kit/llm
- [ ] Xray context injection: read cached chunks or generate JIT
- [ ] Command schema generator: export available `task` subcommands as JSON
- [ ] Confidence-gated executor: auto/confirm/clarify bands
- [ ] Inline clarification: single question for disambiguation
- [ ] REPL clarification: multi-turn for complex generative prompts
- [ ] Destructive command guard: always confirm delete/unclaim/unassign
- [ ] `tlc prompt task <id>`: enriched context dump (markdown + `--json`)
- [ ] `tlc <prompt>`: NL entry point wired into RootCmd
- [ ] Story: NL prompt happy path (classifier hit)
- [ ] Story: NL prompt with router escalation
- [ ] Story: NL prompt with clarification
- [ ] Story: context dump for agent consumption
- [ ] E2E tests derived from story acceptance criteria

---

## Problem

Agents and users interact with `tlc task` through explicit subcommands.
Simple operations require knowing exact syntax. Complex operations
(e.g. "create 5 tasks for the auth epic") require multiple sequential
commands.

Two gaps exist:

1. **No natural language interface.** Agents must construct exact CLI
   invocations. Freeform intent like "mark the auth task done" requires
   the caller to resolve task IDs and map verbs to subcommands.

2. **No structured context dump.** Agents feeding task context into
   LLM prompts must scrape `task show` output. No purpose-built format
   includes dependency summaries, track info, and audit history.

## Command Surface

### `tlc <prompt>`

Natural language to command(s). Prompt is a freeform string.

```
tlc "mark T-42 done"
tlc "create 3 tasks for auth: login, logout, refresh"
tlc "what tasks are blocked?"
```

Flags:
- `--execute` / `-x` — force execution, skip confirmation
- `--no-prompt` — skip batch confirmation (existing convention)
- `--dry-run` — show resolved commands, never execute
- `--json` — output resolved commands as JSON

### `tlc prompt task <id>`

Enriched context dump for agent consumption.

```
tlc prompt task T-0042
tlc prompt task T-0042 --json
```

Flags:
- `--json` — structured JSON (default: markdown)

Output includes:
- Task fields (title, status, description, tags, assignee, priority)
- Blocked-by task summaries (ID, title, status)
- Blocking task summaries
- Track details + current phase
- Recent audit log entries (parsed from description)

## Architecture

```
tlc <prompt>
       │
  ┌────▼─────┐
  │Classifier │  Local regex/keyword matching
  │(Go)       │  Match → commands[] + confidence 1.0
  └────┬─────┘
       │
  miss │
       │
  ┌────▼──────────┐
  │kit/llm client  │  Provider-agnostic via hop.top/kit/llm
  │(Go)            │  URI-based config (ollama://, anthropic://, etc.)
  └────┬──────────┘
       │
  ┌────▼──────────┐
  │Executor        │  Confidence-gated
  │(Go)            │  High → run
  │                │  Mid  → confirm → run
  │                │  Low  → clarify
  └───────────────┘
```

### Components

**Classifier** (`internal/cli/prompt_classify.go`)

Regex and keyword patterns for obvious mappings. Handles:
- `"complete T-42"` → `task complete T-0042`
- `"mark T-42 done"` → `task complete T-0042`
- `"list my tasks"` → `task list --mine`
- `"show T-42"` → `task show T-0042`

Returns `[]ResolvedCommand` with confidence 1.0 on match, nil on miss.

**kit/llm router** (`internal/cli/prompt_router.go`)

Uses `hop.top/kit/llm` for prompts the classifier cannot handle.
Provider-agnostic: works with any registered adapter (ollama, anthropic,
openai, etc.) via URI-based config.

Provider resolution:
1. Config key `prompt.llm_provider` (e.g. `ollama://llama3.2`)
2. Env `TLC_PROMPT_LLM` override
3. Falls back to `kit/llm.LoadConfig("")` default
4. All unavailable → classifier-only mode with error message

Uses `kit/llm.Client.Complete()` with system prompt containing:
- Available command schema (from `GenerateSchemaJSON(RootCmd)`)
- Xray root chunk (cached `.xray_*.md` or JIT-generated)
- Task description content when prompt references a task
- User prompt

Parses structured JSON from LLM response:
```json
{
  "commands": [
    {"cmd": "task", "args": ["create", "Login flow", "--tag", "type:feat"]}
  ],
  "confidence": 0.92
}
```

**Xray context injection**

Enriches router calls with repo-aware context:
- Read cached `.xray_*.md` root chunk if it exists (cheap)
- Generate `xray map` JIT if no cache and prompt references
  code/files/artifacts
- Root chunk only (≈2k tokens) for file/directory awareness
- Hardcoded 4096 token cap on xray content sent to router

**Executor** (`internal/cli/prompt_exec.go`)

Confidence-gated command runner:
- **High (≥0.9):** auto-execute
- **Medium (0.7–0.9):** show resolved commands, wait for confirmation
- **Low (<0.7):** trigger clarification

Override flags:
- `--execute` / `--no-prompt` → skip confirmation at any confidence
- `--dry-run` → never execute at any confidence

Destructive commands (`delete`, `unclaim`, `unassign`) always require
confirmation regardless of confidence. `--no-prompt` overrides this.

Multi-command execution: sequential, stop on first error. Output shows
completed commands, the failure, and remaining unexecuted commands.

**Clarifier** (`internal/cli/prompt_clarify.go`)

Two modes based on router response metadata:

1. **Inline** — single follow-up question for simple ambiguity.
   Example: "Which task? T-0042 (JWT refresh) or T-0068 (login page)"
   User answers, combined context re-sent to router.

2. **REPL** — multi-turn conversational loop for complex generative
   prompts. LLM response signals `multi_turn: true` in response. User exits
   with `ctrl+c` or `quit`. Each turn re-classifies with accumulated
   context.

## Data Flow

### NL prompt (classifier hit)

```
1. tlc "complete T-42"
2. Classifier matches: task complete T-0042 (confidence 1.0)
3. Executor: 1.0 ≥ 0.9 → auto-execute
4. Output: Completed task T-0042
```

### NL prompt (router escalation)

```
1. tlc "create 3 tasks for auth: login, logout, refresh"
2. Classifier: no match
3. kit/llm call with command schema + xray context
4. LLM returns 3 create commands, confidence 0.92
5. Executor: 0.92 ≥ 0.9 → auto-execute all three
6. Output:
   Created T-0231: Login flow
   Created T-0232: Logout flow
   Created T-0233: Refresh token flow
```

### NL prompt (clarification)

```
1. tlc "mark the auth task done"
2. Classifier: no exact match (ambiguous "auth task")
3. LLM returns confidence 0.55 + disambiguation candidates
4. Executor: 0.55 < 0.7 → clarify
5. Inline: "Which task? T-0042 (JWT refresh) or T-0068 (login page)"
6. User: "42"
7. Re-classify with combined context → confidence 1.0
8. Execute: task complete T-0042
```

### Context dump

```
1. tlc prompt task T-0042
2. Load task T-0042 from storage
3. Load related:
   - blocked-by T-0038 [DONE]: Add token storage
   - blocks T-0045 [TODO]: Session invalidation
   - track: auth-system (active, phase 2/4)
   - audit entries from description
4. Render markdown:

   # T-0042: Implement JWT refresh endpoint
   Status: IN_PROGRESS | Assigned: @jadb | Priority: P1
   Tags: type:feat, domain:auth
   Track: auth-system (active, phase 2/4)

   ## Description
   <description without audit block>

   ## Dependencies
   - blocked-by T-0038 [DONE]: Add token storage
   - blocks T-0045 [TODO]: Session invalidation

   ## Recent Activity
   - 2026-04-03 14:22 · @jadb · CLAIMED
   - 2026-04-02 09:15 · @exo · created
```

## Error Handling

**LLM unavailable:** Classifier-only mode. If classifier misses too,
print: `no LLM provider configured; use explicit commands or set
TLC_PROMPT_LLM (e.g. ollama://llama3.2)`

**Ambiguous task references:** Low confidence → inline clarification
with candidate list.

**Destructive commands:** Always confirm regardless of confidence.
`--no-prompt` is the escape hatch.

**Multi-command partial failure:** Stop on first error. Show what
completed, what failed, what remains.

**Task not found (prompt <id>):** Standard error:
`task T-9999 not found; run 'tlc task list' to see available tasks`

**LLM timeout/error:** kit/llm handles fallback chain automatically.
If all providers exhausted, fail fast. Suggest explicit commands.

## Testing Strategy

**Unit tests:**
- Classifier: table-driven, input prompt → expected commands + confidence
- Router client: kit/llm mock provider, response parsing, malformed JSON
- Executor: confidence bands, destructive guard, partial failure
- Context dump: markdown format, JSON format, missing deps

**E2E tests:**
- Derived from `docs/stories/` acceptance criteria (stories TBD)
- Classifier-only path (no router dependency)
- Context dump rendering
- LLM-unavailable degradation

## File Layout

```
internal/cli/
  prompt.go              # cobra wiring: RootCmd NL handler
  prompt_classify.go     # local regex/keyword classifier
  prompt_classify_test.go
  prompt_router.go       # kit/llm HTTP + shell client
  prompt_router_test.go
  prompt_exec.go         # confidence-gated executor
  prompt_exec_test.go
  prompt_clarify.go      # inline + REPL clarification
  prompt_clarify_test.go
  prompt_context.go      # `tlc prompt task <id>` context dump
  prompt_context_test.go
  prompt_schema.go       # schema generator (all top-level commands)
  prompt_schema_test.go
  prompt_xray.go         # xray context injection
  prompt_xray_test.go
  prompt_e2e_test.go     # E2E tests
```
