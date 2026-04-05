<div align="center">
    <img src="./media/logo_large.webp" alt="Spec Kit Logo" width="200" height="200"/>
    <h1>☑︎ TLC</h1>
    <h3><em>Task </em></h3>
</div>

<p align="center">
    <strong>An open source toolkit that allows you to focus on product scenarios and predictable outcomes instead of vibe coding every piece from scratch.</strong>
</p>

<p align="center">
    <a href="https://github.com/hop-top/tlc/releases"><img src="https://img.shields.io/github/release/hop-top/tlc.svg" alt="Release"/></a>
    <a href="https://pkg.go.dev/hop.top/tlc?tab=doc"><img src="https://pkg.go.dev/badge/hop.top/tlc.svg" alt="GoDoc"/></a>
    <a href="https://github.com/hop-top/tlc/actions"><img src="https://github.com/hop-top/tlc/actions/workflows/ci.yml/badge.svg" alt="Build Status"/></a>
    <a href="https://github.com/hop-top/tlc/stargazers"><img src="https://img.shields.io/github/stars/hop-top/tlc?style=social" alt="GitHub stars"/></a>
    <a href="https://github.com/hop-top/tlc/blob/main/LICENSE"><img src="https://img.shields.io/github/license/hop-top/tlc" alt="License"/></a>
</p>



☑︎ TLC

TLC is a high-performance, multi-agent task orchestration tool designed for developers and AI agents. It uses a human-friendly "Task Line Syntax" (TLS) to manage tasks in a flat text file while maintaining a synchronized SQLite database for advanced querying and state management.

## 🚀 Key Features

- **Hybrid Storage Model**: Edit tasks directly in a human-friendly `todo.txt` (Task Line Syntax) or use the synchronized SQLite database for high-performance querying.
- **Complex Orchestration (Task Flows)**: Define declarative workflows with support for sequential and parallel execution, conditional branching, synchronization joins, and automated retries.
- **Flows & Assignees**: Procedural workflow templates that generate task sequences with capability-based auto-assignment to specialized executors (inspired by superpowers plugin).
- **Multi-Agent Collaboration**: Safe coordination between humans and AI agents using task claiming, responsibility transfer, delegation protocols, and time-bounded ownership leases (TTL).
- **Deterministic Task Execution**: Robust execution contract with stdout/stderr capture, error normalization, and configurable timeouts.
- **Audit-Ready Logging**: A canonical, reverse-chronological `CHANGELOG` capturing every state transition, collaboration action, and execution attempt.
- **Natural Language Interface**: Type `tlc task "complete the auth task"`
  instead of memorizing exact syntax. Local classifier handles common
  patterns instantly; LLM router (via `hop.top/kit/llm`) resolves complex
  prompts with confidence-gated execution and clarification.
- **Agent Context Dump**: `tlc task prompt <id>` renders enriched task
  context (deps, track info, audit log) in markdown or JSON optimized
  for LLM consumption.
- **Advanced Query Engine**: Power-user filtering with intuitive shorthands (`@me`, `#tag`), logical operators (AND/OR/NOT), and metadata-aware searching.
- **External System Sync**: Bidirectional integration with GitHub Issues, Jira, and Linear while maintaining a strict boundary for internal agent tasks.
- **Auto-Archiving**: Automatically clean up your workspace by archiving completed tasks after a configurable duration (default: 7 days), keeping your active list focused and high-performance.
- **Configurable Workflows**: Define custom task statuses, state
  machine transitions, and per-tag workflow overrides via config.
  Semantic roles drive claim/unclaim/complete behavior.
- **Workspace Model**: Organize multiple projects into workspaces
  and spaces. Query tasks across projects with `--workspace` and
  `--space` flags, or get a summary view with `--summary`.
- **Track Registry**: First-class work streams that group related
  tasks with lifecycle management, computed health state, and phase
  progress. Auto-transition from pending to active on first task
  claim. Project health pulse with overcommit warnings.
- **Modern TUI & CLI**: A keyboard-driven Terminal User Interface
  built with Bubble Tea, featuring a Kanban board, dashboard, and
  real-time flow monitoring.
- **XDG Specification Compliance**: Zero-config persistence
  following standard OS paths for data, logs, and configuration.

## 🛠 Installation

### Quick Install (Build from source)
```bash
curl -fsSL https://raw.githubusercontent.com/hop-top/tlc/main/install.sh | bash
```

### Build manually
```bash
# Clone the repository
git clone https://github.com/hop-top/tlc.git
cd tlc

# Build the binary
mkdir -p bin
go build -o bin/tlc cmd/tlc/main.go
```

### Docker

Run TLC in a containerized environment with persistent storage:

```bash
# Build the image
docker build -t tlc-cli .

# Run interactively with TUI
docker run -it --rm \
  -v tlc-data:/home/tlc/.local/share/tlc \
  -v tlc-config:/home/tlc/.config/tlc \
  tlc-cli tui

# Run one-off commands
docker run --rm \
  -v tlc-data:/home/tlc/.local/share/tlc \
  tlc-cli task list

# Or use docker-compose
docker-compose up -d
docker-compose exec tlc tlc tui
```

For more Docker usage examples, see [docs/docker.md](docs/docker.md).

## 🧪 Testing

### Run all tests
```bash
go test ./...
```

### Generate Coverage Report
```bash
./scripts/coverage.sh
```
The HTML report will be available at `coverage/all.html`.

## 👨‍💻 Development

### Quick Start

```bash
make tools    # Install dev tools (golangci-lint, air)
make dev      # Run all checks (format, lint, test)
make watch    # Auto-rebuild on changes
```

See [docs/development-setup.md](docs/development-setup.md) for comprehensive development workflow, watch modes, and editor integration.

### Available Make Targets

| Command | Description |
|---------|-------------|
| `make build` | Build binary to `bin/tlc` |
| `make test` | Run tests with race detector |
| `make lint` | Run golangci-lint |
| `make fmt` | Format code with gofmt + goimports |
| `make watch` | Watch and rebuild on changes |
| `make watch-lint` | Watch and lint on changes |
| `make dev` | Run format + lint + test |
| `make tools` | Install development tools |

### Watch Mode (Auto-detection)

Get instant feedback while coding:
```bash
make watch-lint  # Lint on file save
make watch       # Build on file save
make watch-test  # Test on file save
```

## 📖 Usage

### Quick Start
Initialize a new project:
```bash
./bin/tlc init
```

### AI Agent Integration

Configure AI agents to use TLC for task management:

**For Claude Code users:**

Add this to your global Claude configuration (`~/.claude/CLAUDE.md`):

```markdown
## Task Management with TLC

Use TLC (Task Line CLI) for all task tracking instead of TodoWrite.

**Setup (run once per project):**
- Check if initialized: `ls .tlc/` or run `tlc init` if not found
- Optional GitHub sync: Run `tlc sync pull github` or `tlc sync push github` to auto-configure (syncs with GitHub Issues)

**Common commands:**
- Create: `tlc task create "Task title" --assigned-to @me --tag feature`
- Create blocked task: `tlc task create "Task title" --blocked-by T-0041 --blocked-by other-project/T-0007`
- List: `tlc task list --mine` (shows IN_PROGRESS first, then TODO by default)
- List stale: `tlc task list --stale` (only tasks past their stale threshold)
- Stale subcommand: `tlc task stale` (dedicated stale view; fires hooks with `--run-hooks`)
- List blocked: `tlc task list --blocked` (only tasks with a blocked reason)
- List by priority: `tlc task list --priority P0` or `--priority P0,P1`
- List blocked by: `tlc task list --blocked-by T-0041` (tasks that depend on T-0041)
- Update: `tlc task update T-0042 --status IN_PROGRESS`
- Update blockers: `tlc task update T-0042 --add-blocked-by T-0009 --remove-blocked-by T-0003`
- Mark blocked: `tlc task update T-0042 --blocked "waiting on T-0009"`
- Clear blocked: `tlc task update T-0042 --unblock`
- Set stale timeout: `tlc task update T-0042 --timeout 2h`
- Create with timeout: `tlc task create "Task title" --timeout 4h`
- Complete: `tlc task complete T-0042`
- Assign: `tlc task assign T-0042 codex`
- Claim: `tlc task claim T-0042`
- View logs: `tlc log T-0042`
- Track create: `tlc track create "Title" --type feature`
- Track list: `tlc track list --status active`
- Track show: `tlc track show <id>`
- Track summary: `tlc track summary`
- Link task to track: `tlc task create "Title" --track <track-id>`

**Tool definition:** Run `tlc help llm --format mcp` for MCP tool schema.
```

**For other AI platforms:**

Add to your project's `AGENTS.md` file:

```markdown
## Task Management

This project uses TLC for task tracking.

Setup: `tlc init`
Tool definition: `tlc help llm` (or `--format openai` for OpenAI platforms)
```

**Natural language prompts:**

Use freeform text instead of exact CLI syntax:

```bash
# Classifier handles common patterns instantly (confidence 1.0)
tlc task "complete T-42"
tlc task "list my tasks"
tlc task "show T-42"

# Complex prompts route to LLM (requires LLM provider config)
tlc task "create 3 tasks for auth: login, logout, refresh"

# Flags
tlc task "mark the auth task done" --dry-run   # preview only
tlc task "delete T-42" --execute               # skip confirmation
tlc task "list blocked tasks" --json           # JSON output
```

Configure the LLM provider:
```bash
export TLC_PROMPT_LLM="ollama://llama3.2"  # or anthropic://, openai://
```

**Agent context dump:**

Get enriched task context for LLM consumption:

```bash
tlc task prompt T-0042           # markdown (default)
tlc task prompt T-0042 --json    # structured JSON
```

Output includes: task fields, blocked-by/blocking summaries, track
details with phase, and recent audit log entries.

### Managing Tasks
List active tasks (IN_PROGRESS first, then TODO):
```bash
./bin/tlc task list
```

List archived tasks:
```bash
./bin/tlc task list --archived
```

Claim a task for work:
```bash
./bin/tlc task claim T-0042
```

Release a claimed task:
```bash
./bin/tlc task unclaim T-0042
```

Complete a task:
```bash
./bin/tlc task complete T-0042
```

Assign or unassign a task:
```bash
./bin/tlc task assign T-0042 codex
./bin/tlc task unassign T-0042 --note "reason"
```

Create a new task:
```bash
./bin/tlc task create "Design API schema" --assigned-to engineer-1 --tag infra
```

Update a task status:
```bash
./bin/tlc task update T-0042 --status DONE
```

### Stale Task Detection

Tasks are stale when not updated within their timeout. Use `tlc task stale` to list them:

```bash
# List stale tasks (uses per-task timeout; falls back to task.stale.default_timeout config)
tlc task stale

# List stale tasks and fire configured hooks (sets StaleFiredAt; re-fires even if set)
tlc task stale --run-hooks
```

Configure project-wide defaults and hooks in `.tlc.yaml`:

```yaml
task:
  stale:
    default_timeout: 6h           # applied to tasks with no per-task timeout
    hooks:
      - command: 'echo "stale: {{.ID}} ({{.Title}})" >> /tmp/stale.log'
```

Per-task timeout via CLI:

```bash
tlc task update T-0042 --timeout 2h
tlc task create "Design API schema" --timeout 4h
```

### Workspaces

Organize projects into workspaces for cross-project task management:

```bash
# List workspaces and their projects
tlc workspace list

# Query tasks across a workspace
tlc task list --workspace myws

# Filter to a specific space
tlc task list --workspace myws --space labs

# Summary view (grouped counts by status)
tlc task list --summary
```

### Tracks

Organize related tasks into tracks — cohesive work streams with
their own lifecycle, progress, and health state:

```bash
# Create a feature track
tlc track create "Browser rendering" --type feature

# Link tasks to the track
tlc task create "Parse HTML" --track browser-rendering --tag phase:1
tlc task create "Render DOM" --track browser-rendering --tag phase:1
tlc task create "CSS engine" --track browser-rendering --tag phase:2

# View track progress with phase breakdown
tlc track show browser-rendering
# Output:
#   Track: browser-rendering
#   Progress: 1/3 tasks (33%)
#   Phase 1 — 1/2
#   Phase 2 — 0/1

# List tracks with state and progress
tlc track list --status active
# Output: ID, Title, Type, Status, State, Progress, Assignee

# Project health pulse
tlc track summary
# Shows: status counts, overcommit warning, active track table

# Link a plan with automatic task extraction
tlc track update browser-rendering \
  --add-plan docs/plans/rendering.md

# Lifecycle commands
tlc track update browser-rendering --status completed
tlc track archive browser-rendering
```

Track statuses: `pending` -> `active` -> `completed`/`abandoned`
-> `archived`. A pending track auto-transitions to active when a
linked task is claimed.

Configure health thresholds in `.tlc/config.yaml`:

```yaml
tracks:
  stale_threshold: 48h
  health:
    max_active: 3
    min_progress_to_start: 50
```

### Flows & Assignees

TLC includes a comprehensive workflow suite with capability-based task assignment:

**List available assignees:**
```bash
./bin/tlc assignee list
```

**View assignee details:**
```bash
./bin/tlc assignee show assignee:code-analyst:1.0
```

**Invoke a flow to generate tasks:**
```bash
./bin/tlc flow invoke examples/flows/brainstorming.yaml
```

**Available workflow flows:**
- **Creative & Planning**: brainstorming, writing-plans
- **Development**: test-driven-development, executing-plans
- **Quality Assurance**: systematic-debugging, code-review, verification-before-completion
- **Workflow**: finishing-development-branch

See [docs/flows-and-assignees.md](docs/flows-and-assignees.md) for complete documentation.

### Interactive TUI
Launch the interactive terminal interface:
```bash
./bin/tlc tui
```

**Keybindings:**
- `j`/`k`: Navigate tasks
- `enter`: View task details (including markdown description)
- `n`: Create new task (interactive form)
- `c`/`u`: Claim / Unclaim task
- `s`: Cycle task status
- `/`: Search/Filter tasks
- `t`: Open theme picker
- `v`: Cycle views (Dashboard -> Kanban -> Flows)
- `r`: Refresh data
- `q`: Quit

### 🎨 Theme Customization
TLC features a powerful theme switcher that allows you to customize your terminal experience.

- **Built-in Themes**: Start with the high-contrast default theme.
- **250+ Community Themes**: Pull themes directly from the popular `mbadolato/iTerm2-Color-Schemes` repository.
- **Lazy Loading**: The picker fetches theme names instantly and downloads color data only when you highlight a theme, ensuring a smooth experience even with hundreds of options.

**Inside the Theme Picker:**
- `j`/`k`: Scroll through themes
- `R` (Shift+R): Fetch/Refresh the full list of 250+ remote themes
- `enter`: Apply and save the selected theme
- `esc`: Cancel and return to dashboard

### Local Configuration
You can use a local configuration file to override global settings:
```bash
./bin/tlc --config tlc-local.yaml task list
```

### Multi-Clone Workflows
TLC supports working with the same repository in multiple clones (different machines, worktrees, or branches). It automatically detects your project from git remote and provides flexible options for managing tasks across clones.

**Automatic Project Detection:**

When you run any TLC command in a clone without explicit configuration:
```bash
cd ~/repo-clone
tlc task list
# TLC detects project from git remote and tasks work correctly
```

**Choosing Between Shared or Isolated Tasks:**

When running `tlc init` in a clone, you can choose how tasks are managed:

```bash
# Share tasks across all clones (default)
tlc init --duplicate-id-strategy share

# Isolate tasks for this clone
tlc init --duplicate-id-strategy unique

# Ask interactively
tlc init --duplicate-id-strategy prompt
```

**Common Multi-Clone Scenarios:**

- **Feature Branch Worktrees**: Isolated tasks per branch
  ```bash
  cd ~/main-repo
  tlc init --duplicate-id-strategy unique
  git worktree add ../feature-X feature-X
  cd ../feature-X
  tlc init --duplicate-id-strategy unique  # Isolated tasks
  ```

- **Multiple Machines**: Shared tasks across devices
  ```bash
  # Machine A (work laptop)
  tlc init --duplicate-id-strategy share

  # Machine B (home computer)
  tlc init --duplicate-id-strategy share  # Same tasks
  ```

- **CI/CD Environments**: Auto-detect without config files
  ```bash
  export TLC_PROJECT_FALLBACK_MODE=detected
  tlc task list  # Works in CI without .tlc directory
  ```

For detailed multi-clone setup guides, see:
- [Project Detection](docs/project-detection.md) - How TLC detects projects automatically
- [Multi-Clone Setup](docs/multi-clone-setup.md) - Strategies for shared vs isolated tasks

**Configuration Reference:**

Add to your `.tlc/config.yaml`:
```yaml
project:
  id: "user/my-repo"              # Auto-detected from git remote
  fallback_mode: "auto"            # auto, detected, or prompt
  duplicate_id_strategy: "share"     # share, unique, or prompt
```

## 📝 Task Line Syntax (TLS)
Tasks are stored in a canonical single-line format:
`[status] <ID> <Title> @assignee #tag prio:<P> domain:<D> ref:<R>`

Example:
`[~] T-0002 Fix token refresh race @engineer-3 #auth #bug ref:docs/rfc/012.md`

## ⚙️ Configuration
TLC follows a hierarchical configuration:
1. Environment variables (`TLC_*`)
2. Local config (`.tlc/config.yaml` or custom `--config`)
3. User config (`<os user config dir>/tlc/config.yaml`)
4. System config (`/etc/tlc/config.yaml`)
5. Built-in defaults

Write behavior:
- `tlc config set` writes to the active local or explicit `--config` file.
- If only system config is loaded, or no config file exists yet, TLC writes the new value to the
  OS user config path instead of `/etc/tlc/config.yaml`.

Defaults:
- **Todo file**: `$XDG_DATA_HOME/tlc/todo.txt`
- **Database**: `$XDG_DATA_HOME/tlc/db.sqlite`
- **Logs**: `$XDG_DATA_HOME/tlc/tlc.log`
- **Archive threshold**: 168h (7 days)

## 📚 Documentation

**New to TLC?** Start with [Getting Started Guide](docs/GETTING-STARTED.md) for a progressive learning path from basic tasks to advanced workflows.

Detailed specifications can be found in the `docs/` directory:

**Features & Guides:**
- [Flows & Assignees](docs/flows-and-assignees.md) - Workflow automation and capability-based assignment
- [Development Setup](docs/development-setup.md) - Development workflow and watch modes
- [Editor Setup](docs/editor-setup.md) - IDE/editor integration
- [Docker Usage](docs/docker.md) - Container deployment

**Plans:**
- [Track Registry Design](docs/plans/2026-04-03-track-registry-design.md)
- [Task Prompt Design](docs/plans/2026-04-04-task-prompt-design.md)

**Specifications:**
- [Task CRUD Spec](docs/task-crud-spec-0.1.md)
- [Task Line Syntax Spec](docs/task-line-spec-0.1.md)
- [Task Flow Spec](docs/task-flow-spec-0.1.md)
- [Task Log Spec](docs/task-log-spec-0.1.md)
- [Sync Architecture](docs/sync-architecture-0.1.md)
- [CLI Spec](docs/tlc-cli-spec-0.1.md)
- [TUI Spec](docs/tlc-tui-spec-0.1.md)
- [Config Spec](docs/tlc-config-spec-0.1.md)
- [Config Discovery](docs/config-discovery.md)
- [Project Detection](docs/project-detection.md)

## Contributing

Contributions are welcome! Check the
[open issues](https://github.com/hop-top/tlc/issues) for things to
work on, or open a pull request with your improvements. All PRs are
appreciated -- whether it is a bug fix, new feature, or documentation
update.

## Feedback

Found a bug or have a feature request? Open an issue on the
[GitHub Issues](https://github.com/hop-top/tlc/issues) page. For
questions and general discussion, use
[GitHub Discussions](https://github.com/hop-top/tlc/discussions).

## License

[MIT](LICENSE)
