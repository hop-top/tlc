# TLC — Task Line CLI

TLC is a high-performance, multi-agent task orchestration tool designed for developers and AI agents. It uses a human-friendly "Task Line Syntax" (TLS) to manage tasks in a flat text file while maintaining a synchronized SQLite database for advanced querying and state management.

## 🚀 Key Features

- **Hybrid Storage Model**: Edit tasks directly in a human-friendly `todo.txt` (Task Line Syntax) or use the synchronized SQLite database for high-performance querying.
- **Complex Orchestration (Task Flows)**: Define declarative workflows with support for sequential and parallel execution, conditional branching, synchronization joins, and automated retries.
- **Multi-Agent Collaboration**: Safe coordination between humans and AI agents using task claiming, responsibility transfer, delegation protocols, and time-bounded ownership leases (TTL).
- **Deterministic Task Execution**: Robust execution contract with stdout/stderr capture, error normalization, and configurable timeouts.
- **Audit-Ready Logging**: A canonical, reverse-chronological `CHANGELOG` capturing every state transition, collaboration action, and execution attempt.
- **Advanced Query Engine**: Power-user filtering with intuitive shorthands (`@me`, `#tag`), logical operators (AND/OR/NOT), and metadata-aware searching.
- **External System Sync**: Bidirectional integration with GitHub Issues, Jira, and Linear while maintaining a strict boundary for internal agent tasks.
- **Auto-Archiving**: Automatically clean up your workspace by archiving completed tasks after a configurable duration (default: 7 days), keeping your active list focused and high-performance.
- **Modern TUI & CLI**: A keyboard-driven Terminal User Interface built with Bubble Tea, featuring a Kanban board, dashboard, and real-time flow monitoring.
- **XDG Specification Compliance**: Zero-config persistence following standard OS paths for data, logs, and configuration.

## 🛠 Installation

### Quick Install (Build from source)
```bash
curl -fsSL https://raw.githubusercontent.com/IdeaCraftersLabs/oss-tlc-cli/main/install.sh | bash
```

### Build manually
```bash
# Clone the repository
git clone https://github.com/IdeaCraftersLabs/oss-tlc-cli.git
cd oss-tlc-cli

# Build the binary
mkdir -p bin
go build -o bin/tlc cmd/tlc/main.go
```

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

### Managing Tasks
List all tasks:
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

Create a new task:
```bash
./bin/tlc task create "Design API schema" --assigned-to engineer-1 --tag infra
```

Update a task status:
```bash
./bin/tlc task update T-0042 --status DONE
```

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

## 📝 Task Line Syntax (TLS)
Tasks are stored in a canonical single-line format:
`[status] <ID> <Title> @assignee #tag prio:<P> domain:<D> ref:<R>`

Example:
`[~] T-0002 Fix token refresh race @engineer-3 #auth #bug ref:docs/rfc/012.md`

## ⚙️ Configuration
TLC follows a hierarchical configuration:
1. Environment variables (`TLC_*`)
2. Local config (`.tlc/config.yaml` or custom `--config`)
3. User config (`$XDG_CONFIG_HOME/tlc/config.yaml`)
4. System defaults

Defaults:
- **Todo file**: `$XDG_DATA_HOME/tlc/todo.txt`
- **Database**: `$XDG_DATA_HOME/tlc/db.sqlite`
- **Logs**: `$XDG_DATA_HOME/tlc/tlc.log`
- **Archive threshold**: 168h (7 days)

## 📚 Documentation
Detailed specifications can be found in the `docs/` directory:
- [Task CRUD Spec](docs/task-crud-spec-0.1.md)
- [Task Line Syntax Spec](docs/task-line-spec-0.1.md)
- [Task Log Spec](docs/task-log-spec-0.1.md)
- [Sync Architecture](docs/sync-architecture-0.1.md)
