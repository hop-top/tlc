# Editor Setup Guide

This guide covers IDE/editor configuration for optimal Go development with linting and auto-detection.

## VS Code

### Automatic Setup

VS Code settings are already configured in `.vscode/settings.json` and will be applied automatically when you open the project.

### Manual Verification

1. Install the Go extension:
   - Open VS Code
   - Press `Cmd+Shift+P` (Mac) or `Ctrl+Shift+P` (Windows/Linux)
   - Type "Extensions: Show Recommended Extensions"
   - Install "Go" by Go Team at Google

2. Verify settings:
   - Open Settings (JSON): `Cmd+Shift+P` → "Preferences: Open Settings (JSON)"
   - Workspace settings from `.vscode/settings.json` should be active

### Features Enabled

- **Linting on save**: Uses golangci-lint with project config
- **Format on save**: Uses goimports with local package sorting
- **Organize imports**: Automatically on save
- **Test integration**: Run tests with `-v -race -short` flags
- **Coverage decoration**: Shows coverage in gutter
- **Enhanced gopls**: Static analysis with unusedparams, shadow, nilness checks

### Keyboard Shortcuts

- `Cmd+S` / `Ctrl+S`: Save, format, organize imports
- `Cmd+Shift+T` / `Ctrl+Shift+T`: Run tests at cursor
- `Cmd+Shift+B` / `Ctrl+Shift+B`: Build
- `F12`: Go to definition
- `Shift+F12`: Find all references

---

## GoLand / IntelliJ IDEA

### File Watchers

1. Open Preferences → Tools → File Watchers
2. Click "+" → golangci-lint
3. Configure:
   - **Program**: `golangci-lint`
   - **Arguments**: `run --config $ProjectFileDir$/.golangci.yml $FilePath$`
   - **Output paths**: `$ProjectFileDir$`
   - **Trigger**: On file save
4. Click OK

### Actions on Save

1. Open Preferences → Tools → Actions on Save
2. Enable:
   - ✓ Reformat code
   - ✓ Optimize imports
   - ✓ Run code cleanup

### Code Style

1. Open Preferences → Editor → Code Style → Go
2. **Imports tab**:
   - Sorting type: `goimports`
   - Local package path: `github.com/google/oss-tlc-cli`
3. **Other tab**:
   - Line length: 120

### Inspections

1. Open Preferences → Editor → Inspections
2. Enable:
   - ✓ Go → Probable bugs (all)
   - ✓ Go → Code style issues (all)
   - ✓ Go → Performance issues (all)

### External Tools (Optional)

Add Make targets as external tools:

1. Open Preferences → Tools → External Tools → Add
2. Create tools for:
   - **Make Lint**: `make lint`
   - **Make Test**: `make test`
   - **Make Dev**: `make dev`

---

## Neovim

### Using nvim-lspconfig

Add to your Neovim configuration:

```lua
-- Assuming you have nvim-lspconfig installed
require('lspconfig').gopls.setup {
  settings = {
    gopls = {
      analyses = {
        unusedparams = true,
        shadow = true,
        nilness = true,
      },
      staticcheck = true,
      gofumpt = true,
      hints = {
        assignVariableTypes = true,
        compositeLiteralFields = true,
        constantValues = true,
        functionTypeParameters = true,
        parameterNames = true,
        rangeVariableTypes = true,
      },
    },
  },
}
```

### Using null-ls for Linting

```lua
local null_ls = require('null-ls')
null_ls.setup {
  sources = {
    -- Diagnostics (linting)
    null_ls.builtins.diagnostics.golangci_lint.with({
      args = {
        "run",
        "--config",
        ".golangci.yml",
        "--out-format",
        "json",
        "$FILENAME"
      }
    }),
    -- Formatting
    null_ls.builtins.formatting.goimports.with({
      extra_args = {"-local", "github.com/google/oss-tlc-cli"}
    }),
  },
}
```

### Auto-format on Save

```lua
-- Format on save
vim.api.nvim_create_autocmd("BufWritePre", {
  pattern = "*.go",
  callback = function()
    vim.lsp.buf.format({ async = false })
  end,
})
```

### Key Mappings (Example)

```lua
local opts = { noremap=true, silent=true }
vim.keymap.set('n', '<leader>l', ':!make lint<CR>', opts)
vim.keymap.set('n', '<leader>t', ':!make test<CR>', opts)
vim.keymap.set('n', '<leader>d', ':!make dev<CR>', opts)
vim.keymap.set('n', '<leader>b', ':!make build<CR>', opts)
```

---

## Vim

### ALE (Asynchronous Lint Engine)

Add to your `.vimrc`:

```vim
" ALE configuration
let g:ale_linters = {
\   'go': ['gopls', 'golangci-lint'],
\}

let g:ale_fixers = {
\   'go': ['goimports', 'gofmt'],
\}

let g:ale_go_golangci_lint_options = '--config .golangci.yml --fast'
let g:ale_go_goimports_options = '-local github.com/google/oss-tlc-cli'

" Fix files on save
let g:ale_fix_on_save = 1

" Lint on text change
let g:ale_lint_on_text_changed = 'normal'
let g:ale_lint_on_insert_leave = 1
```

### vim-go

Add to your `.vimrc`:

```vim
" vim-go configuration
let g:go_fmt_command = "goimports"
let g:go_fmt_options = {
\   'goimports': '-local github.com/google/oss-tlc-cli',
\}

" Use golangci-lint
let g:go_metalinter_command = "golangci-lint"
let g:go_metalinter_autosave = 1
let g:go_metalinter_autosave_enabled = ['golangci-lint']

" Lint on save
let g:go_metalinter_enabled = ['golangci-lint']
let g:go_metalinter_deadline = "5s"

" Additional features
let g:go_auto_type_info = 1
let g:go_def_mode = 'gopls'
let g:go_info_mode = 'gopls'
```

---

## Emacs

### Using lsp-mode with gopls

Add to your Emacs configuration:

```elisp
;; LSP mode with gopls
(use-package lsp-mode
  :ensure t
  :commands (lsp lsp-deferred)
  :hook (go-mode . lsp-deferred)
  :config
  (setq lsp-gopls-analyses '((unusedparams . t)
                              (shadow . t)
                              (nilness . t)))
  (setq lsp-gopls-staticcheck t)
  (setq lsp-gopls-use-placeholders t))

;; Format on save
(add-hook 'go-mode-hook
          (lambda ()
            (add-hook 'before-save-hook 'lsp-format-buffer nil t)
            (add-hook 'before-save-hook 'lsp-organize-imports nil t)))

;; Set goimports options
(setq gofmt-command "goimports")
(setq gofmt-args '("-local" "github.com/google/oss-tlc-cli"))
```

### Using flycheck with golangci-lint

```elisp
;; Flycheck with golangci-lint
(use-package flycheck
  :ensure t
  :config
  (flycheck-define-checker golangci-lint
    "A Go linter using golangci-lint."
    :command ("golangci-lint" "run"
              "--config" ".golangci.yml"
              "--out-format" "line-number"
              source)
    :error-patterns
    ((error line-start (file-name) ":" line ":" column ": " (message) line-end))
    :modes go-mode)
  (add-to-list 'flycheck-checkers 'golangci-lint))
```

---

## Sublime Text

### LSP-gopls Setup

1. Install Package Control
2. Install packages:
   - `LSP`
   - `LSP-gopls`
   - `GoSublime` (optional)

3. Create/edit `Packages/User/LSP.sublime-settings`:

```json
{
  "clients": {
    "gopls": {
      "enabled": true,
      "settings": {
        "gopls.analyses": {
          "unusedparams": true,
          "shadow": true,
          "nilness": true
        },
        "gopls.staticcheck": true,
        "gopls.gofumpt": true
      }
    }
  }
}
```

4. Create/edit `Packages/User/Go.sublime-settings`:

```json
{
  "gofmt_cmd": "goimports",
  "gofmt_args": ["-local", "github.com/google/oss-tlc-cli"],
  "fmt_on_save": true
}
```

---

## General Editor-Agnostic Tips

### Environment Setup

Ensure these tools are in your `$PATH`:
```bash
# Check installations
which golangci-lint  # Should show: $GOPATH/bin/golangci-lint
which air            # Should show: $GOPATH/bin/air
which goimports      # Should show: $GOPATH/bin/goimports

# If missing, run:
make tools
```

### EditorConfig

This project includes `.editorconfig` support. Most editors respect these settings automatically. If not, install an EditorConfig plugin for your editor.

### Watch Mode Alternative

If your editor doesn't have good file watching, run `make watch-lint` in a separate terminal for live feedback.

### Terminal Integration

For editors with terminal integration:
- Configure terminal to run `make watch-lint`
- Split pane: editor on left, watch output on right
- Instant feedback on all saves

---

## Troubleshooting

### Linter not running

1. Verify golangci-lint is installed: `which golangci-lint`
2. Check config exists: `ls -la .golangci.yml`
3. Run manually: `make lint`
4. Check editor logs for errors

### Format on save not working

1. Verify goimports is installed: `which goimports`
2. Check editor settings for format-on-save
3. Try manual format: `make fmt`

### gopls errors

1. Update gopls: `go install golang.org/x/tools/gopls@latest`
2. Restart editor
3. Clear gopls cache: `rm -rf ~/.cache/gopls`

### Performance issues

If gopls is slow:
1. Exclude large directories in editor settings (vendor, node_modules)
2. Use `.vscode/settings.json` exclusions (already configured)
3. Consider reducing gopls analysis depth

---

## Recommended Workflow

1. **Editor with linting**: Use VS Code, GoLand, or Neovim with gopls
2. **Watch mode**: Run `make watch-lint` in terminal for additional feedback
3. **Pre-commit**: Use `make dev` before committing
4. **CI verification**: Push and verify CI passes

This setup provides:
- Immediate feedback (< 1 second in editor)
- File watcher feedback (< 2 seconds via `make watch-lint`)
- Pre-commit verification (`make dev`)
- CI validation (GitHub Actions)
