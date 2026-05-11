# Makefile for oss-tlc-cli

.PHONY: help install build build-plugins build-shims test test-short lint lint-fix fmt fmt-check vet tidy tidy-check coverage clean watch watch-lint watch-test dev check tools pre-commit-install verify validate-docs docs-links docs-links-offline prebuild smoke-chdir pre-merge

# Colors for output
COLOR_RESET=\033[0m
COLOR_BOLD=\033[1m
COLOR_GREEN=\033[32m
COLOR_YELLOW=\033[33m
COLOR_BLUE=\033[34m

# Variables
BINARY_NAME=tlc
BIN_DIR=bin
PLUGIN_DIRS=$(wildcard plugins/*/main.go)
COVERAGE_DIR=coverage
TMP_DIR=tmp
MAIN_PATH=cmd/tlc/main.go
GO_FILES=$(shell find . -name '*.go' -not -path "./vendor/*" -not -path "./.worktrees/*" -not -path "./$(TMP_DIR)/*")

# Default target
help: ## Show this help message
	@echo "$(COLOR_BOLD)TLC Development Makefile$(COLOR_RESET)"
	@echo ""
	@echo "$(COLOR_BLUE)Available targets:$(COLOR_RESET)"
	@awk 'BEGIN {FS = ":.*##"; printf "\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  $(COLOR_GREEN)%-20s$(COLOR_RESET) %s\n", $$1, $$2 }' $(MAKEFILE_LIST)
	@echo ""

install: ## Install the binary to $(GOPATH)/bin
	@echo "$(COLOR_BLUE)Installing $(BINARY_NAME)...$(COLOR_RESET)"
	@go install $(MAIN_PATH)
	@echo "$(COLOR_GREEN)✓ Installed to $(shell go env GOPATH)/bin/$(BINARY_NAME)$(COLOR_RESET)"

build: build-plugins ## Build the binary and plugins to bin/
	@echo "$(COLOR_BLUE)Building $(BINARY_NAME)...$(COLOR_RESET)"
	@mkdir -p $(BIN_DIR)
	@go build -v -o $(BIN_DIR)/$(BINARY_NAME) $(MAIN_PATH)
	@echo "$(COLOR_GREEN)✓ Built $(BIN_DIR)/$(BINARY_NAME)$(COLOR_RESET)"

build-plugins: ## Build all plugin binaries
	@if [ -n "$(PLUGIN_DIRS)" ]; then \
		for p in $(PLUGIN_DIRS); do \
			dir=$$(dirname $$p); \
			name=$$(basename $$dir); \
			echo "$(COLOR_BLUE)Building plugin $$name...$(COLOR_RESET)"; \
			mkdir -p $$dir/bin; \
			go build -buildvcs=false -o $$dir/bin/$$name ./$$dir/; \
			echo "$(COLOR_GREEN)✓ Built $$dir/bin/$$name$(COLOR_RESET)"; \
		done; \
	fi

SHIMS_BIN_DIR = internal/flowtest/shims/bin
SHIMS = claude codex gemini copilot opencode fabric llm \
        npm npx uv pip composer \
        docker docker-compose \
        gh git \
        passthrough

build-shims: ## Compile flowtest shim binaries into internal/flowtest/shims/bin/
	@echo "$(COLOR_BLUE)Building flowtest shims...$(COLOR_RESET)"
	@mkdir -p $(SHIMS_BIN_DIR)
	@for shim in $(SHIMS); do \
		go build -tags shimbin -buildvcs=false -o $(SHIMS_BIN_DIR)/$$shim ./internal/flowtest/shims/$$shim/ || exit 1; \
		echo "  built $$shim"; \
	done
	@go build -tags shimbin -buildvcs=false -o $(SHIMS_BIN_DIR)/tlc-shim-catchall ./internal/flowtest/shims/catchall/
	@echo "  built tlc-shim-catchall"
	@echo "$(COLOR_GREEN)✓ Shims built to $(SHIMS_BIN_DIR)/$(COLOR_RESET)"

test: ## Run all tests (CLI suite runs under TrueColor profile via TestMain)
	@echo "$(COLOR_BLUE)Running tests...$(COLOR_RESET)"
	@go test -v -race ./...
	@echo "$(COLOR_GREEN)✓ All tests passed$(COLOR_RESET)"

test-short: ## Run tests without race detector (faster)
	@echo "$(COLOR_BLUE)Running tests (short mode)...$(COLOR_RESET)"
	@go test -v -short ./...

lint: ## Run golangci-lint
	@echo "$(COLOR_BLUE)Running linters...$(COLOR_RESET)"
	@golangci-lint run --config .golangci.yml
	@echo "$(COLOR_GREEN)✓ Linting passed$(COLOR_RESET)"

lint-fix: ## Run golangci-lint with auto-fix
	@echo "$(COLOR_BLUE)Running linters with auto-fix...$(COLOR_RESET)"
	@golangci-lint run --config .golangci.yml --fix
	@echo "$(COLOR_GREEN)✓ Linting and fixes applied$(COLOR_RESET)"

fmt: ## Format all Go files
	@echo "$(COLOR_BLUE)Formatting Go files...$(COLOR_RESET)"
	@gofmt -s -w $(GO_FILES)
	@goimports -w $(GO_FILES)
	@echo "$(COLOR_GREEN)✓ Formatting complete$(COLOR_RESET)"

vet: ## Run go vet
	@echo "$(COLOR_BLUE)Running go vet...$(COLOR_RESET)"
	@go vet ./...
	@echo "$(COLOR_GREEN)✓ Vet passed$(COLOR_RESET)"

tidy: ## Tidy and verify go modules
	@echo "$(COLOR_BLUE)Tidying go modules...$(COLOR_RESET)"
	@go mod tidy && go mod verify
	@echo "$(COLOR_GREEN)✓ Modules tidy$(COLOR_RESET)"

fmt-check: ## Check formatting (non-mutating; fails if files need formatting)
	@echo "$(COLOR_BLUE)Checking Go file formatting...$(COLOR_RESET)"
	@bad=$$(gofmt -l $(GO_FILES)); if [ -n "$$bad" ]; then echo "$$bad"; echo "$(COLOR_YELLOW)⚠ Run 'make fmt' to fix$(COLOR_RESET)"; exit 1; fi
	@bad=$$(goimports -l $(GO_FILES)); if [ -n "$$bad" ]; then echo "$$bad"; echo "$(COLOR_YELLOW)⚠ Run 'make fmt' to fix$(COLOR_RESET)"; exit 1; fi
	@echo "$(COLOR_GREEN)✓ Formatting check passed$(COLOR_RESET)"

tidy-check: ## Verify go.mod/go.sum are tidy (non-mutating; fails if dirty)
	@echo "$(COLOR_BLUE)Checking go module tidiness...$(COLOR_RESET)"
	@go mod tidy
	@git diff --exit-code -- go.mod go.sum || { echo "$(COLOR_YELLOW)⚠ go.mod/go.sum not tidy; run 'make tidy'$(COLOR_RESET)"; exit 1; }
	@echo "$(COLOR_GREEN)✓ Modules tidy check passed$(COLOR_RESET)"

coverage: ## Generate test coverage report
	@echo "$(COLOR_BLUE)Generating coverage report...$(COLOR_RESET)"
	@./scripts/coverage.sh
	@echo "$(COLOR_GREEN)✓ Coverage report: $(COVERAGE_DIR)/all.html$(COLOR_RESET)"

clean: ## Clean build artifacts
	@echo "$(COLOR_BLUE)Cleaning...$(COLOR_RESET)"
	@rm -rf $(BIN_DIR) $(COVERAGE_DIR) $(TMP_DIR)
	@rm -rf plugins/*/bin
	@find . -name "*.test" -type f -delete
	@find . -name "*.out" -type f -delete
	@echo "$(COLOR_GREEN)✓ Clean complete$(COLOR_RESET)"

watch: ## Watch for changes and rebuild (requires air)
	@echo "$(COLOR_BLUE)Starting file watcher (build mode)...$(COLOR_RESET)"
	@if ! command -v air >/dev/null 2>&1; then \
		echo "$(COLOR_YELLOW)⚠ 'air' not found. Install with: make tools$(COLOR_RESET)"; \
		exit 1; \
	fi
	@air -c .air.toml

watch-lint: ## Watch for changes and run linter (requires air)
	@echo "$(COLOR_BLUE)Starting file watcher (lint mode)...$(COLOR_RESET)"
	@if ! command -v air >/dev/null 2>&1; then \
		echo "$(COLOR_YELLOW)⚠ 'air' not found. Install with: make tools$(COLOR_RESET)"; \
		exit 1; \
	fi
	@air -c .air.lint.toml

watch-test: ## Watch for changes and run tests
	@echo "$(COLOR_BLUE)Starting file watcher (test mode)...$(COLOR_RESET)"
	@if ! command -v air >/dev/null 2>&1; then \
		echo "$(COLOR_YELLOW)⚠ 'air' not found. Install with: make tools$(COLOR_RESET)"; \
		exit 1; \
	fi
	@air -c .air.toml -- -c 'go test -v -short ./...'

dev: fmt vet lint tidy test ## Run fmt, vet, lint, tidy, and test (pre-commit workflow)
	@echo "$(COLOR_GREEN)✓ Development checks passed$(COLOR_RESET)"

check: fmt-check vet lint tidy-check test ## Full pre-build gate (non-mutating: fmt-check, vet, lint, tidy-check, test)
	@echo "$(COLOR_GREEN)✓ All checks passed$(COLOR_RESET)"

tools: ## Install development tools (golangci-lint, air)
	@echo "$(COLOR_BLUE)Installing development tools...$(COLOR_RESET)"
	@echo "$(COLOR_YELLOW)→ Installing golangci-lint...$(COLOR_RESET)"
	@if ! command -v golangci-lint >/dev/null 2>&1; then \
		curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(shell go env GOPATH)/bin latest; \
	else \
		echo "  $(COLOR_GREEN)✓ golangci-lint already installed$(COLOR_RESET)"; \
	fi
	@echo "$(COLOR_YELLOW)→ Installing air...$(COLOR_RESET)"
	@if ! command -v air >/dev/null 2>&1; then \
		go install github.com/air-verse/air@latest; \
	else \
		echo "  $(COLOR_GREEN)✓ air already installed$(COLOR_RESET)"; \
	fi
	@echo "$(COLOR_GREEN)✓ All tools installed$(COLOR_RESET)"

pre-commit-install: ## Install pre-commit hooks
	@echo "$(COLOR_BLUE)Installing pre-commit hooks...$(COLOR_RESET)"
	@if ! command -v pre-commit >/dev/null 2>&1; then \
		echo "$(COLOR_YELLOW)⚠ 'pre-commit' not found. Install with: pip install pre-commit$(COLOR_RESET)"; \
		exit 1; \
	fi
	@pre-commit install
	@echo "$(COLOR_GREEN)✓ Pre-commit hooks installed$(COLOR_RESET)"

verify: lint test ## Run linting and tests (CI-like checks)
	@echo "$(COLOR_GREEN)✓ All verification checks passed$(COLOR_RESET)"

smoke-chdir: ## Run the -C/--chdir end-to-end smoke (builds tlc, exercises two projects)
	@echo "$(COLOR_BLUE)Running -C/--chdir smoke...$(COLOR_RESET)"
	@go test -run TestChdir_Smoke_E2E_SwitchesProject -count=1 ./cmd/tlc/
	@echo "$(COLOR_GREEN)✓ chdir smoke passed$(COLOR_RESET)"

# Pre-merge gate enforces the three checks every PR to main must pass:
#   1. build       — `go build ./...` (buildvcs disabled for worktree compatibility)
#   2. static vet  — `go vet ./...` (canonical static check used by `make check`;
#                    golangci-lint exists separately but currently has heavy
#                    pre-existing lint debt + a v2.12 config-schema regression
#                    in the shared CI lint workflow — out of scope here)
#   3. e2e smoke   — `-C/--chdir` smoke test exercising the recently landed
#                    chdir pre-parse (2b9335e / 814c397)
pre-merge: ## Pre-merge gate: build, vet, and -C smoke (every PR to main must pass)
	@echo "$(COLOR_BOLD)Running pre-merge gate...$(COLOR_RESET)"
	@echo ""
	@echo "$(COLOR_BLUE)[1/3] Build...$(COLOR_RESET)"
	@GOFLAGS=-buildvcs=false go build ./...
	@echo "$(COLOR_GREEN)✓ Build passed$(COLOR_RESET)"
	@echo ""
	@echo "$(COLOR_BLUE)[2/3] Vet...$(COLOR_RESET)"
	@GOFLAGS=-buildvcs=false go vet ./...
	@echo "$(COLOR_GREEN)✓ Vet passed$(COLOR_RESET)"
	@echo ""
	@echo "$(COLOR_BLUE)[3/3] -C/--chdir smoke...$(COLOR_RESET)"
	@GOFLAGS=-buildvcs=false $(MAKE) --no-print-directory smoke-chdir
	@echo ""
	@echo "$(COLOR_GREEN)$(COLOR_BOLD)✓ Pre-merge gate passed$(COLOR_RESET)"

validate-docs: ## Validate tlc command examples in docs/AGENTS.md (Refs: tlc/T-0067)
	@echo "$(COLOR_BLUE)Validating doc command examples...$(COLOR_RESET)"
	@./scripts/validate-doc-commands.sh
	@echo "$(COLOR_GREEN)✓ Doc command validation passed$(COLOR_RESET)"

docs-links: ## Check documentation internal links using lychee
	@echo "$(COLOR_BLUE)Checking documentation links...$(COLOR_RESET)"
	@if ! command -v lychee >/dev/null 2>&1; then \
		echo "$(COLOR_YELLOW)⚠ 'lychee' not found. Install with: cargo install lychee$(COLOR_RESET)"; \
		exit 1; \
	fi
	@lychee --config .lychee.toml docs/ 2>&1 | { \
		if grep -q "🚫"; then \
			echo "$(COLOR_YELLOW)⚠ Documentation link issues found (see above)$(COLOR_RESET)"; \
			exit 1; \
		else \
			echo "$(COLOR_GREEN)✓ All documentation links valid$(COLOR_RESET)"; \
		fi; \
	}

docs-links-offline: ## Check documentation internal links (offline mode - excludes external URLs)
	@echo "$(COLOR_BLUE)Checking documentation links (offline mode)...$(COLOR_RESET)"
	@if ! command -v lychee >/dev/null 2>&1; then \
		echo "$(COLOR_YELLOW)⚠ 'lychee' not found. Install with: cargo install lychee$(COLOR_RESET)"; \
		exit 1; \
	fi
	@lychee --offline --config .lychee.toml docs/ || { \
		echo "$(COLOR_YELLOW)⚠ Some internal documentation links are broken$(COLOR_RESET)"; \
		exit 1; \
	}
	@echo "$(COLOR_GREEN)✓ All internal documentation links valid$(COLOR_RESET)"

prebuild: ## Run all Go quality gates before building (fmt, vet, lint, test, mod tidy)
	@echo "$(COLOR_BOLD)Running prebuild quality gates...$(COLOR_RESET)"
	@echo ""
	@echo "$(COLOR_BLUE)[1/5] Formatting...$(COLOR_RESET)"
	@gofmt -s -w $(GO_FILES)
	@echo "$(COLOR_GREEN)✓ Format complete$(COLOR_RESET)"
	@echo ""
	@echo "$(COLOR_BLUE)[2/5] Vetting...$(COLOR_RESET)"
	@go vet ./...
	@echo "$(COLOR_GREEN)✓ Vet passed$(COLOR_RESET)"
	@echo ""
	@echo "$(COLOR_BLUE)[3/5] Linting...$(COLOR_RESET)"
	@golangci-lint run --config .golangci.yml
	@echo "$(COLOR_GREEN)✓ Lint passed$(COLOR_RESET)"
	@echo ""
	@echo "$(COLOR_BLUE)[4/5] Testing...$(COLOR_RESET)"
	@go test -v -race -coverprofile=coverage.out ./...
	@echo "$(COLOR_GREEN)✓ Tests passed$(COLOR_RESET)"
	@echo ""
	@echo "$(COLOR_BLUE)[5/5] Tidying modules...$(COLOR_RESET)"
	@go mod tidy && go mod verify
	@echo "$(COLOR_GREEN)✓ Modules verified$(COLOR_RESET)"
	@echo ""
	@echo "$(COLOR_GREEN)$(COLOR_BOLD)All prebuild quality gates passed$(COLOR_RESET)"

# Development shorthand aliases
.PHONY: w wl wt
w: watch       ## Alias for watch
wl: watch-lint ## Alias for watch-lint
wt: watch-test ## Alias for watch-test
