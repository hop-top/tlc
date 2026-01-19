# Research Report: Go CLI Landscape 2026

**Date:** January 17, 2026
**Subject:** Analysis of Top Go CLI Repositories, Structures, and Frameworks

## 🏆 Top Go CLI Repositories & Tools
Analysis of high-star and influential Go repositories reveals several "gold standard" implementations:

1.  **Orchestration & Infrastructure:** `kubernetes/kubernetes`, `docker/cli`, `helm/helm`.
2.  **Productivity & Search:** `junegunn/fzf` (fuzzy finder), `jesseduffield/lazygit` (git UI).
3.  **Developer Experience:** `cli/cli` (GitHub CLI), `gohugoio/hugo` (Static Site Generator).
4.  **Modern TUI (Text User Interface):** `charmbracelet/glow` (Markdown), `aquasecurity/trivy` (Security scanner).

## 🏗️ Popular Project Structures

### 1. The "Standard" Layout (Enterprise/Large Scale)
Most dominant in established projects like Kubernetes and Docker.
*   **`cmd/`**: Contains main entry points. Each subdirectory is a binary.
*   **`internal/`**: Private application code. Enforced by the Go compiler to prevent external imports.
*   **`pkg/`**: Public library code. *Note: Usage is decreasing in favor of `internal/` unless explicit library export is intended.*

### 2. The "Flat" Layout (Micro-CLIs)
Common for single-purpose tools.
*   **Root `main.go`**: Entry point in the root directory.
*   **Sibling Packages**: Logic partitioned into small, focused packages like `commands/` or `config/`.

### 3. The "Domain-Driven" Layout (Feature-Oriented)
Emerging in complex CLI apps that mimic service architectures.
*   Packages organized by feature (e.g., `internal/auth/`, `internal/sync/`) rather than technical layer.

## 🛠️ Dominant Frameworks & Libraries

### 1. Command Frameworks
*   **`spf13/cobra`**: The industry standard. Used by Kubernetes, Hugo, and GitHub CLI. Best for complex, nested subcommands.
*   **`urfave/cli`**: A minimalist and expressive alternative, popular for smaller tools.
*   **`alecthomas/kong`**: Type-safe parsing using struct tags. Increasing adoption for its conciseness.

### 2. Configuration & State
*   **`spf13/viper`**: The "classic" for complex config management (YAML, ENV, etc.).
*   **`knadh/koanf`**: A lighter, faster alternative to Viper with fewer dependencies.

### 3. User Interface (UI/TUI)
*   **`charmbracelet/bubbletea`**: The revolutionary "The Elm Architecture" (TEA) framework for Go. Enables rich, interactive TUIs.
*   **`charmbracelet/lipgloss`**: CSS-like styling for terminal text.
*   **`AlecAivazis/survey`**: Standard for interactive prompts (Select, Confirm, Input).

### 4. Observability
*   **`schollz/progressbar`**: Thread-safe progress indicators.
*   **`rs/zerolog`**: High-performance structured logging.

## 📝 Strategic Recommendations
1.  **Standardize on Cobra + Viper** for robust, multi-command CLIs.
2.  **Adopt Bubbletea** for high-touch, interactive user experiences.
3.  **Default to `internal/`** for code organization to maintain a clean public API boundary.
