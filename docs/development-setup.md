# Development Setup Guide

## Prerequisites

- Go 1.25+ (project uses 1.25.5)
- Git
- Make (standard on macOS/Linux)

## Quick Setup

1. Clone and setup:
   ```bash
   git clone https://github.com/google/oss-tlc-cli.git
   cd oss-tlc-cli
   ```

2. Install development tools:
   ```bash
   make tools
   ```

   This installs:
   - golangci-lint v1.61.0
   - air (file watcher)

3. (Optional) Install pre-commit hooks:
   ```bash
   pip install pre-commit  # If not already installed
   make pre-commit-install
   ```

4. Verify setup:
   ```bash
   make verify  # Runs lint + test
   ```

## Development Workflow

### Standard Workflow

```bash
# Build
make build

# Run tests
make test

# Lint code
make lint

# Format code
make fmt

# All checks (fmt + lint + test)
make dev
```

### Watch Mode (Auto-detection)

Watch mode provides immediate feedback by automatically running tasks when you save files:

```bash
# Watch and rebuild on changes
make watch        # or: make w

# Watch and lint on changes
make watch-lint   # or: make wl

# Watch and test on changes
make watch-test   # or: make wt
```

**How it works:**
- Monitors Go source files for changes
- Debounces rapid file saves (500ms-1000ms delay)
- Provides instant feedback on errors
- Logs errors to `build-errors.log` or `lint-errors.log`
- Clear screen on each run for better visibility

**Use cases:**
- `make watch-lint`: Best for active development - catches errors as you type
- `make watch`: Use when testing the built binary
- `make watch-test`: Use during TDD workflow

### Before Committing

```bash
# Run all checks
make dev

# Or let pre-commit handle it (if installed)
git commit  # Hooks run automatically
```

## Common Tasks

| Task | Command |
|------|---------|
| Build binary | `make build` |
| Run tests | `make test` |
| Run tests (fast) | `make test-short` |
| Lint code | `make lint` |
| Lint with auto-fix | `make lint-fix` |
| Format code | `make fmt` |
| Coverage report | `make coverage` |
| Clean artifacts | `make clean` |
| Install tools | `make tools` |
| Watch & rebuild | `make watch` |
| Watch & lint | `make watch-lint` |
| Watch & test | `make watch-test` |
| All checks | `make dev` |
| CI-like checks | `make verify` |

## CI Integration

All PRs automatically run:
1. golangci-lint (with full config)
2. Build verification
3. Full test suite with race detector
4. Coverage reporting

Local workflow should match CI:
```bash
make verify  # Equivalent to CI checks
```

## Troubleshooting

### golangci-lint not found
```bash
make tools  # Reinstall tools
```

### Pre-commit hook failing
```bash
pre-commit run --all-files  # Test hooks
pre-commit autoupdate       # Update hook versions
```

### Watch mode not working
```bash
make tools  # Ensure air is installed
which air   # Verify installation
```

### Linting errors on first run

It's normal to see linting errors on first run as we've added comprehensive checks. Use `make lint-fix` to auto-fix simple issues like formatting and imports.

For other issues:
- Review the error message and fix manually
- Use `//nolint:linter-name` for legitimate exceptions (with explanation)
- Adjust `.golangci.yml` if rules are too strict (discuss with team first)

### Test failures

If tests fail after setting up linting:
1. The critical bug in `internal/core/mocks.go` has been fixed (missing imports)
2. Run `make test` to verify all tests pass
3. If issues persist, check for race conditions with `make test` (includes `-race` flag)

## Editor Setup

See [editor-setup.md](./editor-setup.md) for:
- VS Code configuration
- GoLand/IntelliJ setup
- Neovim configuration

## Best Practices

### Development Cycle

1. Start watch-lint in one terminal: `make watch-lint`
2. Edit code in your editor
3. See instant feedback on lint errors
4. Before commit: `make dev` (format, lint, test)
5. Commit changes

### Code Quality

- Fix lint warnings before committing
- Run tests with race detector: `make test`
- Keep test coverage high: `make coverage`
- Use `make lint-fix` for auto-fixable issues
- Review TODO/FIXME comments regularly

### Performance

- Use `make test-short` for faster feedback during development
- Full `make test` with race detector before pushing
- `make watch-lint` is faster than `make watch-test`

## Additional Resources

- [Linter Configuration](./.golangci.yml) - Comprehensive linting rules
- [Air Configuration](./.air.toml) - Build watcher settings
- [Pre-commit Hooks](./.pre-commit-config.yaml) - Git hook configuration
- [Development Conventions](./dev-workflow-conventions-0.1.md) - Git workflow and commit standards
