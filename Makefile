# Makefile for oss-tlc-cli

.PHONY: help install build test lint fmt coverage clean watch watch-lint watch-test dev tools pre-commit-install

# Colors for output
COLOR_RESET=\033[0m
COLOR_BOLD=\033[1m
COLOR_GREEN=\033[32m
COLOR_YELLOW=\033[33m
COLOR_BLUE=\033[34m

# Variables
BINARY_NAME=tlc
BIN_DIR=bin
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

build: ## Build the binary to bin/
	@echo "$(COLOR_BLUE)Building $(BINARY_NAME)...$(COLOR_RESET)"
	@mkdir -p $(BIN_DIR)
	@go build -v -o $(BIN_DIR)/$(BINARY_NAME) $(MAIN_PATH)
	@echo "$(COLOR_GREEN)✓ Built $(BIN_DIR)/$(BINARY_NAME)$(COLOR_RESET)"

test: ## Run all tests
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

coverage: ## Generate test coverage report
	@echo "$(COLOR_BLUE)Generating coverage report...$(COLOR_RESET)"
	@./scripts/coverage.sh
	@echo "$(COLOR_GREEN)✓ Coverage report: $(COVERAGE_DIR)/all.html$(COLOR_RESET)"

clean: ## Clean build artifacts
	@echo "$(COLOR_BLUE)Cleaning...$(COLOR_RESET)"
	@rm -rf $(BIN_DIR) $(COVERAGE_DIR) $(TMP_DIR)
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

dev: fmt lint test ## Run fmt, lint, and test (pre-commit workflow)
	@echo "$(COLOR_GREEN)✓ Development checks passed$(COLOR_RESET)"

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

# Development shorthand aliases
.PHONY: w wl wt
w: watch       ## Alias for watch
wl: watch-lint ## Alias for watch-lint
wt: watch-test ## Alias for watch-test
