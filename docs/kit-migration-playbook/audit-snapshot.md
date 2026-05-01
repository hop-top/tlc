# kit-layout-migration audit

Generated: 2026-04-28

## TL;DR

- **Total tlc Go files importing kit:** 37
- **Distinct kit packages used:** 16
- **Highest-touch package:** `hop.top/kit/domain` (14 files)
- **Packages with no clear new-layout home:** none — all 16 resolved ✅

---

## Part 1 — File-level inventory

| File | Line | Old import path |
|------|------|-----------------|
| cmd/tlc/main.go | 8 | hop.top/kit/llm/anthropic |
| cmd/tlc/main.go | 9 | hop.top/kit/llm/ollama |
| cmd/tlc/main.go | 10 | hop.top/kit/llm/openai |
| internal/cli/agent_async.go | 11 | hop.top/kit/output |
| internal/cli/agent_list.go | 11 | hop.top/kit/output |
| internal/cli/agent_run.go | 14 | hop.top/kit/output |
| internal/cli/config_interactive_llm.go | 13 | hop.top/kit/llm |
| internal/cli/config_interactive_llm.go | 14 | hop.top/kit/llm/errors |
| internal/cli/flow.go | 13 | hop.top/kit/output |
| internal/cli/formatter.go | 12 | hop.top/kit/markdown |
| internal/cli/formatter.go | 13 | hop.top/kit/output |
| internal/cli/help.go | 9 | hop.top/kit/toolspec |
| internal/cli/hints.go | 5 | hop.top/kit/cli |
| internal/cli/hints.go | 6 | hop.top/kit/output |
| internal/cli/log.go | 12 | hop.top/kit/output |
| internal/cli/project.go | 13 | hop.top/kit/output |
| internal/cli/prompt_router.go | 12 | hop.top/kit/llm |
| internal/cli/prompt_router_test.go | 11 | hop.top/kit/llm |
| internal/cli/prompt_router_test.go | 14 | hop.top/kit/llm/anthropic |
| internal/cli/prompt_router_test.go | 15 | hop.top/kit/llm/ollama |
| internal/cli/prompt_router_test.go | 16 | hop.top/kit/llm/openai |
| internal/cli/prompt_schema.go | 9 | hop.top/kit/toolspec |
| internal/cli/prompt_schema_test.go | 7 | hop.top/kit/toolspec |
| internal/cli/root.go | 15 | hop.top/kit/bus |
| internal/cli/root.go | 16 | hop.top/kit/cli |
| internal/cli/root.go | 17 | hop.top/kit/domain |
| internal/cli/root.go | 18 | hop.top/kit/ext/dispatch |
| internal/cli/root.go | 19 | hop.top/kit/log |
| internal/cli/root.go | 25 | hop.top/kit/upgrade |
| internal/cli/track_graph.go | 9 | hop.top/kit/output |
| internal/cli/track_list.go | 12 | hop.top/kit/output |
| internal/cli/track_show.go | 13 | hop.top/kit/output |
| internal/cli/upgrade.go | 9 | hop.top/kit/xdg |
| internal/cli/upgrade.go | 10 | hop.top/kit/upgrade |
| internal/cli/upgrade.go | 11 | hop.top/kit/upgrade/skill |
| internal/cli/upgrade_test.go | 11 | hop.top/kit/upgrade |
| internal/cli/workspace.go | 10 | hop.top/kit/output |
| internal/config/loader.go | 9 | hop.top/kit/config |
| internal/config/paths.go | 9 | hop.top/kit/xdg |
| internal/core/domain_service_test.go | 8 | hop.top/kit/domain |
| internal/core/domain_statemachine.go | 4 | hop.top/kit/domain |
| internal/core/domain_statemachine_test.go | 7 | hop.top/kit/domain |
| internal/core/entity.go | 3 | hop.top/kit/domain |
| internal/core/service.go | 8 | hop.top/kit/domain |
| internal/core/state_updater.go | 8 | hop.top/kit/bus |
| internal/core/state_updater_test.go | 10 | hop.top/kit/bus |
| internal/core/track_service.go | 8 | hop.top/kit/domain |
| internal/events/adapter.go | 6 | hop.top/kit/bus |
| internal/events/adapter_test.go | 7 | hop.top/kit/bus |
| internal/events/audit.go | 7 | hop.top/kit/bus |
| internal/events/audit_test.go | 8 | hop.top/kit/bus |
| internal/events/domain.go | 3 | hop.top/kit/domain |
| internal/events/domain_test.go | 7 | hop.top/kit/bus |
| internal/events/topics.go | 8 | hop.top/kit/bus |
| internal/events/topics_test.go | 6 | hop.top/kit/bus |
| internal/extensions/extensions.go | 12 | hop.top/kit/bus |
| internal/extensions/extensions.go | 13 | hop.top/kit/ext |
| internal/extensions/extensions_test.go | 7 | hop.top/kit/ext |
| internal/extensions/github.go | 4 | hop.top/kit/ext |
| internal/extensions/jira.go | 4 | hop.top/kit/ext |
| internal/extensions/linear.go | 4 | hop.top/kit/ext |
| internal/extensions/sync_provider.go | 6 | hop.top/kit/ext |
| internal/extensions/sync_provider_test.go | 7 | hop.top/kit/ext |
| internal/flowtest/resolver.go | 10 | hop.top/kit/xdg |
| internal/storage/flow_domain_repo.go | 10 | hop.top/kit/domain |
| internal/storage/flow_domain_repo_test.go | 8 | hop.top/kit/domain |
| internal/storage/migrations.go | 8 | hop.top/kit/sqlstore |
| internal/storage/task_domain_repo.go | 9 | hop.top/kit/domain |
| internal/storage/task_domain_repo_test.go | 10 | hop.top/kit/domain |
| internal/storage/track_domain_repo.go | 10 | hop.top/kit/domain |
| internal/tui/items.go | 8 | hop.top/kit/cli |
| internal/tui/items.go | 9 | hop.top/kit/tui |
| internal/tui/model.go | 11 | hop.top/kit/cli |
| internal/tui/model.go | 12 | hop.top/kit/tui |
| internal/tui/styles/styles.go | 7 | hop.top/kit/cli |
| internal/tui/views.go | 11 | hop.top/kit/tui |

---

## Part 2 — Path mapping table

Verified by `find /Users/jadb/.w/ideacrafterslabs/kit/hops/main/go -type d -name <pkg>`.

| Old path | New path | Verified | Notes |
|----------|----------|----------|-------|
| hop.top/kit/bus | hop.top/kit/go/runtime/bus | ✅ | go/runtime/bus |
| hop.top/kit/cli | hop.top/kit/go/console/cli | ✅ | go/console/cli |
| hop.top/kit/config | hop.top/kit/go/core/config | ✅ | go/core/config |
| hop.top/kit/domain | hop.top/kit/go/runtime/domain | ✅ | go/runtime/domain |
| hop.top/kit/ext | hop.top/kit/go/ai/ext | ✅ | go/ai/ext |
| hop.top/kit/ext/dispatch | hop.top/kit/go/ai/ext/dispatch | ✅ | go/ai/ext/dispatch |
| hop.top/kit/llm | hop.top/kit/go/ai/llm | ✅ | go/ai/llm |
| hop.top/kit/llm/anthropic | hop.top/kit/go/ai/llm/anthropic | ✅ | go/ai/llm/anthropic |
| hop.top/kit/llm/errors | hop.top/kit/go/ai/llm/errors | ✅ | go/ai/llm/errors |
| hop.top/kit/llm/ollama | hop.top/kit/go/ai/llm/ollama | ✅ | go/ai/llm/ollama |
| hop.top/kit/llm/openai | hop.top/kit/go/ai/llm/openai | ✅ | go/ai/llm/openai |
| hop.top/kit/log | hop.top/kit/go/console/log | ✅ | go/console/log |
| hop.top/kit/markdown | hop.top/kit/go/console/markdown | ✅ | go/console/markdown |
| hop.top/kit/output | hop.top/kit/go/console/output | ✅ | go/console/output |
| hop.top/kit/sqlstore | hop.top/kit/go/storage/sqlstore | ✅ | go/storage/sqlstore |
| hop.top/kit/toolspec | hop.top/kit/go/ai/toolspec | ✅ | go/ai/toolspec |
| hop.top/kit/tui | hop.top/kit/go/console/tui | ✅ | go/console/tui |
| hop.top/kit/upgrade | hop.top/kit/go/core/upgrade | ✅ | go/core/upgrade |
| hop.top/kit/upgrade/skill | hop.top/kit/go/core/upgrade/skill | ✅ | go/core/upgrade/skill |
| hop.top/kit/xdg | hop.top/kit/go/core/xdg | ✅ | go/core/xdg |

---

## Part 3 — Per-package consumer summary

### hop.top/kit/bus (imported by 13 files)
- internal/cli/root.go:15
- internal/core/state_updater.go:8
- internal/core/state_updater_test.go:10
- internal/events/adapter.go:6
- internal/events/adapter_test.go:7
- internal/events/audit.go:7
- internal/events/audit_test.go:8
- internal/events/domain_test.go:7
- internal/events/topics.go:8
- internal/events/topics_test.go:6
- internal/extensions/extensions.go:12

### hop.top/kit/cli (imported by 5 files)
- internal/cli/hints.go:5
- internal/cli/root.go:16
- internal/tui/items.go:8
- internal/tui/model.go:11
- internal/tui/styles/styles.go:7

### hop.top/kit/config (imported by 1 file)
- internal/config/loader.go:9

### hop.top/kit/domain (imported by 14 files)
- internal/cli/root.go:17
- internal/core/domain_service_test.go:8
- internal/core/domain_statemachine.go:4
- internal/core/domain_statemachine_test.go:7
- internal/core/entity.go:3
- internal/core/service.go:8
- internal/core/track_service.go:8
- internal/events/domain.go:3
- internal/storage/flow_domain_repo.go:10
- internal/storage/flow_domain_repo_test.go:8
- internal/storage/task_domain_repo.go:9
- internal/storage/task_domain_repo_test.go:10
- internal/storage/track_domain_repo.go:10

### hop.top/kit/ext (imported by 7 files)
- internal/extensions/extensions.go:13
- internal/extensions/extensions_test.go:7
- internal/extensions/github.go:4
- internal/extensions/jira.go:4
- internal/extensions/linear.go:4
- internal/extensions/sync_provider.go:6
- internal/extensions/sync_provider_test.go:7

### hop.top/kit/ext/dispatch (imported by 1 file)
- internal/cli/root.go:18

### hop.top/kit/llm (imported by 3 files)
- internal/cli/config_interactive_llm.go:13
- internal/cli/prompt_router.go:12
- internal/cli/prompt_router_test.go:11

### hop.top/kit/llm/anthropic (imported by 2 files)
- cmd/tlc/main.go:8
- internal/cli/prompt_router_test.go:14

### hop.top/kit/llm/errors (imported by 1 file)
- internal/cli/config_interactive_llm.go:14

### hop.top/kit/llm/ollama (imported by 2 files)
- cmd/tlc/main.go:9
- internal/cli/prompt_router_test.go:15

### hop.top/kit/llm/openai (imported by 2 files)
- cmd/tlc/main.go:10
- internal/cli/prompt_router_test.go:16

### hop.top/kit/log (imported by 1 file)
- internal/cli/root.go:19

### hop.top/kit/markdown (imported by 1 file)
- internal/cli/formatter.go:12

### hop.top/kit/output (imported by 11 files)
- internal/cli/agent_async.go:11
- internal/cli/agent_list.go:11
- internal/cli/agent_run.go:14
- internal/cli/flow.go:13
- internal/cli/formatter.go:13
- internal/cli/hints.go:6
- internal/cli/log.go:12
- internal/cli/project.go:13
- internal/cli/track_graph.go:9
- internal/cli/track_list.go:12
- internal/cli/track_show.go:13
- internal/cli/workspace.go:10

### hop.top/kit/sqlstore (imported by 1 file)
- internal/storage/migrations.go:8

### hop.top/kit/toolspec (imported by 3 files)
- internal/cli/help.go:9
- internal/cli/prompt_schema.go:9
- internal/cli/prompt_schema_test.go:7

### hop.top/kit/tui (imported by 3 files)
- internal/tui/items.go:9
- internal/tui/model.go:12
- internal/tui/views.go:11

### hop.top/kit/upgrade (imported by 3 files)
- internal/cli/root.go:25
- internal/cli/upgrade.go:10
- internal/cli/upgrade_test.go:11

### hop.top/kit/upgrade/skill (imported by 1 file)
- internal/cli/upgrade.go:11

### hop.top/kit/xdg (imported by 3 files)
- internal/cli/upgrade.go:9
- internal/config/paths.go:9
- internal/flowtest/resolver.go:10
