# Development Setup Guide

## Prerequisites

- Go 1.26+ (project uses 1.26.1)
- Git
- [just](https://github.com/casey/just) — command runner (replaces Make)

## Quick Setup

1. Clone and setup:
   ```bash
   git clone https://github.com/hop-top/tlc.git
   cd tlc
   ```

2. Install development tools:
   ```bash
   just tools
   ```

   This installs:
   - golangci-lint
   - air (file watcher)

3. (Optional) Install pre-commit hooks:
   ```bash
   pip install pre-commit  # If not already installed
   just pre-commit-install
   ```

4. Verify setup:
   ```bash
   just check  # Runs lint + test
   ```

## Development Workflow

### Standard Workflow

```bash
# Build
just build

# Run tests
just test

# Lint code
just lint

# Format code
just fmt

# All checks (fmt + lint + test)
just dev
```

### Watch Mode (Auto-detection)

Watch mode provides immediate feedback by automatically running tasks when you save files:

```bash
# Watch and rebuild on changes
just watch        # or: just w

# Watch and lint on changes
just watch-lint   # or: just wl

# Watch and test on changes
just watch-test   # or: just wt
```

**How it works:**
- Monitors Go source files for changes
- Debounces rapid file saves (500ms-1000ms delay)
- Provides instant feedback on errors
- Logs errors to `build-errors.log` or `lint-errors.log`
- Clear screen on each run for better visibility

**Use cases:**
- `just watch-lint`: Best for active development - catches errors as you type
- `just watch`: Use when testing the built binary
- `just watch-test`: Use during TDD workflow

### Before Committing

```bash
# Run all checks
just dev

# Or let pre-commit handle it (if installed)
git commit  # Hooks run automatically
```

## Common Tasks

| Task | Command |
|------|---------|
| Build binary | `just build` |
| Run tests | `just test` |
| Run tests (fast) | `just test-short` |
| Lint code | `just lint` |
| Lint with auto-fix | `just lint-fix` |
| Format code | `just fmt` |
| Coverage report | `just coverage` |
| Clean artifacts | `just clean` |
| Install tools | `just tools` |
| Watch & rebuild | `just watch` |
| Watch & lint | `just watch-lint` |
| Watch & test | `just watch-test` |
| All checks | `just dev` |
| CI-like checks | `just check` |

## CI Integration

All PRs automatically run:
1. golangci-lint (with full config)
2. Build verification
3. Full test suite with race detector
4. Coverage reporting

Local workflow should match CI:
```bash
just check  # Equivalent to CI checks
```

## Troubleshooting

### golangci-lint not found
```bash
just tools  # Reinstall tools
```

### Pre-commit hook failing
```bash
pre-commit run --all-files  # Test hooks
pre-commit autoupdate       # Update hook versions
```

### Watch mode not working
```bash
just tools  # Ensure air is installed
which air   # Verify installation
```

### Linting errors on first run

It's normal to see linting errors on first run as we've added comprehensive checks. Use `just lint-fix` to auto-fix simple issues like formatting and imports.

For other issues:
- Review the error message and fix manually
- Use `//nolint:linter-name` for legitimate exceptions (with explanation)
- Adjust `.golangci.yml` if rules are too strict (discuss with team first)

### Test failures

If tests fail after setting up linting:
1. The critical bug in `internal/core/mocks.go` has been fixed (missing imports)
2. Run `just test` to verify all tests pass
3. If issues persist, check for race conditions with `just test` (includes `-race` flag)

## Editor Setup

See [editor-setup.md](./editor-setup.md) for:
- VS Code configuration
- GoLand/IntelliJ setup
- Neovim configuration

## Best Practices

### Development Cycle

1. Start watch-lint in one terminal: `just watch-lint`
2. Edit code in your editor
3. See instant feedback on lint errors
4. Before commit: `just dev` (format, lint, test)
5. Commit changes

### Code Quality

- Fix lint warnings before committing
- Run tests with race detector: `just test`
- Keep test coverage high: `just coverage`
- Use `just lint-fix` for auto-fixable issues
- Review TODO/FIXME comments regularly

### Performance

- Use `just test-short` for faster feedback during development
- Full `just test` with race detector before pushing
- `just watch-lint` is faster than `just watch-test`

## Additional Resources

- [Linter Configuration](./.golangci.yml) - Comprehensive linting rules
- [Air Configuration](./.air.toml) - Build watcher settings
- [Pre-commit Hooks](./.pre-commit-config.yaml) - Git hook configuration
- [Development Conventions](./dev-workflow-conventions-0.1.md) - Git workflow and commit standards
