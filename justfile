# justfile for tlc

set shell := ["bash", "-euo", "pipefail", "-c"]

binary_name := "tlc"
bin_dir := "bin"
coverage_dir := "coverage"
tmp_dir := "tmp"
main_path := "cmd/tlc/main.go"
shims_bin_dir := "internal/flowtest/shims/bin"
shims := "claude codex gemini copilot opencode fabric llm npm npx uv pip composer docker docker-compose gh git passthrough"

# Show available recipes
default:
    @echo "TLC Development Recipes"
    @echo ""
    @just --list

# Install the binary to $GOPATH/bin
install:
    @echo "Installing {{binary_name}}..."
    @go install {{main_path}}
    @echo "✓ Installed to $(go env GOPATH)/bin/{{binary_name}}"

# Build the binary and plugins to bin/
build: build-plugins
    @echo "Building {{binary_name}}..."
    @mkdir -p {{bin_dir}}
    @go build -v -o {{bin_dir}}/{{binary_name}} {{main_path}}
    @echo "✓ Built {{bin_dir}}/{{binary_name}}"

# Build all plugin binaries
build-plugins:
    #!/usr/bin/env bash
    set -euo pipefail
    for p in $(find plugins -name 'main.go' 2>/dev/null || true); do
        dir=$(dirname "$p")
        name=$(basename "$dir")
        echo "Building plugin $name..."
        mkdir -p "$dir/bin"
        go build -buildvcs=false -o "$dir/bin/$name" "./$dir/"
        echo "✓ Built $dir/bin/$name"
    done

# Compile flowtest shim binaries into internal/flowtest/shims/bin/
build-shims:
    #!/usr/bin/env bash
    set -euo pipefail
    echo "Building flowtest shims..."
    mkdir -p {{shims_bin_dir}}
    for shim in {{shims}}; do
        go build -tags shimbin -buildvcs=false \
            -o {{shims_bin_dir}}/$shim ./internal/flowtest/shims/$shim/ || exit 1
        echo "  built $shim"
    done
    go build -tags shimbin -buildvcs=false \
        -o {{shims_bin_dir}}/tlc-shim-catchall ./internal/flowtest/shims/catchall/
    echo "  built tlc-shim-catchall"
    echo "✓ Shims built to {{shims_bin_dir}}/"

# Run all tests (CLI suite runs under TrueColor profile via TestMain)
test:
    @echo "Running tests..."
    @go test -v -race ./...
    @echo "✓ All tests passed"

# Run tests without race detector (faster)
test-short:
    @echo "Running tests (short mode)..."
    @go test -v -short ./...

# Run golangci-lint
lint:
    @echo "Running linters..."
    @golangci-lint run --config .golangci.yml
    @echo "✓ Linting passed"

# Run golangci-lint with auto-fix
lint-fix:
    @echo "Running linters with auto-fix..."
    @golangci-lint run --config .golangci.yml --fix
    @echo "✓ Linting and fixes applied"

# Format all Go files
fmt:
    #!/usr/bin/env bash
    set -euo pipefail
    echo "Formatting Go files..."
    go_files=$(find . -name '*.go' \
        -not -path "./vendor/*" \
        -not -path "./.worktrees/*" \
        -not -path "./{{tmp_dir}}/*")
    gofmt -s -w $go_files
    goimports -w $go_files
    echo "✓ Formatting complete"

# Generate test coverage report
coverage:
    @echo "Generating coverage report..."
    @./scripts/coverage.sh
    @echo "✓ Coverage report: {{coverage_dir}}/all.html"

# Clean build artifacts
clean:
    #!/usr/bin/env bash
    set -euo pipefail
    echo "Cleaning..."
    rm -rf {{bin_dir}} {{coverage_dir}} {{tmp_dir}}
    rm -rf plugins/*/bin
    find . -name "*.test" -type f -delete
    find . -name "*.out" -type f -delete
    echo "✓ Clean complete"

# Watch for changes and rebuild (requires air)
watch:
    #!/usr/bin/env bash
    set -euo pipefail
    if ! command -v air >/dev/null 2>&1; then
        echo "⚠ 'air' not found. Install with: just tools"
        exit 1
    fi
    echo "Starting file watcher (build mode)..."
    air -c .air.toml

# Watch for changes and run linter (requires air)
watch-lint:
    #!/usr/bin/env bash
    set -euo pipefail
    if ! command -v air >/dev/null 2>&1; then
        echo "⚠ 'air' not found. Install with: just tools"
        exit 1
    fi
    echo "Starting file watcher (lint mode)..."
    air -c .air.lint.toml

# Watch for changes and run tests (requires air)
watch-test:
    #!/usr/bin/env bash
    set -euo pipefail
    if ! command -v air >/dev/null 2>&1; then
        echo "⚠ 'air' not found. Install with: just tools"
        exit 1
    fi
    echo "Starting file watcher (test mode)..."
    air -c .air.toml -- -c 'go test -v -short ./...'

# Run fmt, lint, and test (pre-commit workflow)
dev: fmt lint test
    @echo "✓ Development checks passed"

# Install development tools (golangci-lint, air)
tools:
    #!/usr/bin/env bash
    set -euo pipefail
    echo "Installing development tools..."
    echo "→ Installing golangci-lint..."
    if ! command -v golangci-lint >/dev/null 2>&1; then
        curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh \
            | sh -s -- -b "$(go env GOPATH)/bin" latest
    else
        echo "  ✓ golangci-lint already installed"
    fi
    echo "→ Installing air..."
    if ! command -v air >/dev/null 2>&1; then
        go install github.com/air-verse/air@latest
    else
        echo "  ✓ air already installed"
    fi
    echo "✓ All tools installed"

# Install pre-commit hooks
pre-commit-install:
    #!/usr/bin/env bash
    set -euo pipefail
    if ! command -v pre-commit >/dev/null 2>&1; then
        echo "⚠ 'pre-commit' not found. Install with: pip install pre-commit"
        exit 1
    fi
    echo "Installing pre-commit hooks..."
    pre-commit install
    echo "✓ Pre-commit hooks installed"

# Run linting and tests (CI-like checks)
verify: lint test
    @echo "✓ All verification checks passed"

# Alias: same as verify
check: verify

# Validate tlc command examples in docs/AGENTS.md
validate-docs:
    @echo "Validating doc command examples..."
    @./scripts/validate-doc-commands.sh
    @echo "✓ Doc command validation passed"

# Check documentation internal links using lychee
docs-links:
    #!/usr/bin/env bash
    set -euo pipefail
    if ! command -v lychee >/dev/null 2>&1; then
        echo "⚠ 'lychee' not found. Install with: cargo install lychee"
        exit 1
    fi
    echo "Checking documentation links..."
    lychee --config .lychee.toml docs/ 2>&1 | {
        if grep -q "🚫"; then
            echo "⚠ Documentation link issues found (see above)"
            exit 1
        else
            echo "✓ All documentation links valid"
        fi
    }

# Check documentation internal links (offline mode)
docs-links-offline:
    #!/usr/bin/env bash
    set -euo pipefail
    if ! command -v lychee >/dev/null 2>&1; then
        echo "⚠ 'lychee' not found. Install with: cargo install lychee"
        exit 1
    fi
    echo "Checking documentation links (offline mode)..."
    lychee --offline --config .lychee.toml docs/ || {
        echo "⚠ Some internal documentation links are broken"
        exit 1
    }
    echo "✓ All internal documentation links valid"

# Aliases
alias w := watch
alias wl := watch-lint
alias wt := watch-test
