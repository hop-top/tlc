# Flag Audit: Hop Tools CLI Alignment

Author: $USER
Date: 2026-04-13

## Global Flags Comparison

| Flag | tlc | ctxt | xray | rsx | kit/cli contract |
|------|-----|------|------|-----|------------------|
| `--help` / `-h` | yes | yes | yes | yes | yes (flag only) |
| `--version` / `-v` | yes | yes (`-v`) | yes (`-V`) | yes (`-v`) | yes (`-v`) |
| `--verbose` / `-V` | yes (`-V`) | yes (`-V`) | yes (`-v`) | no | tlc-specific |
| `--quiet` | yes | no | no | no | kit provides |
| `--no-color` | yes | no | no | no | kit provides |
| `--no-hints` | yes | no | no | no | kit provides |
| `--format` / `-f` | yes (`-f`) | `--output` | no | `--json`/`--markdown` | kit: `--format` |
| `--config` / `-c` | yes (`-c`) | `--config` | config file | no | tlc-specific |
| `--json` | yes (NL) | no | no | yes | n/a |
| `--dry-run` | yes (NL) | no | no | no | n/a |
| `--execute` / `-x` | yes (NL) | no | no | no | n/a |

NL = natural language prompt flags only.

## tlc Detailed Flag Inventory

### Root Persistent Flags (via kit/cli + tlc additions)

| Flag | Short | Default | Source | Notes |
|------|-------|---------|--------|-------|
| `--config` | `-c` | `""` | tlc | config file path |
| `--verbose` | `-V` | `false` | tlc | verbose logging |
| `--format` | `-f` | `table` | kit/output | output format |
| `--quiet` | | `false` | kit/cli | suppress non-essential output |
| `--no-color` | | `false` | kit/cli | disable ANSI colour |
| `--no-hints` | | `false` | kit/output | suppress hints |
| `--version` | `-v` | | fang | version output |
| `--help` | `-h` | | cobra | help output |

### Root RunE Flags (NL prompt)

| Flag | Short | Default | Notes |
|------|-------|---------|-------|
| `--dry-run` | | `false` | show resolved cmds |
| `--execute` | `-x` | `false` | execute without confirm |
| `--json` | | `false` | JSON output for NL |

### task Subcommand Flags

| Subcommand | Flag | Short | Default |
|------------|------|-------|---------|
| task (persistent) | `--no-prompt` | | `false` |
| task list | `--status` | `-s` | `[]` |
| task list | `--assigned-to` | `-a` | `""` |
| task list | `--tag` | | `[]` |
| task list | `--mine` | | `false` |
| task list | `--archived` | | `false` |
| task list | `--all-projects` | | `false` |
| task list | `--sort-by` | | `created_at` |
| task list | `--sort-direction` | | `desc` |
| task list | `--limit` | `-n` | `100` |
| task list | `--offset` | | `0` |
| task list | `--summary` | | `false` |
| task list | `--counters` | | `false` |
| task list | `--workspace` | | `""` |
| task list | `--space` | | `""` |
| task list | `--profile` | | `""` |
| task list | `--squad` | | `""` |
| task list | `--stale` | | `false` |
| task list | `--blocked` | | `false` |
| task list | `--priority` | | `[]` |
| task list | `--blocked-by` | | `[]` |
| task list | `--track` | | `""` |
| task create | `--tag` | | `[]` |
| task create | `--assigned-to` | | `""` |
| task create | `--description` | `-d` | `""` |
| task create | `--effort` | | `""` |
| task create | `--priority` | | `""` |
| task create | `--reference` | | `""` |
| task create | `--blocked-by` | | `[]` |
| task create | `--track` | | `""` |
| task create | `--stale-timeout` | | `""` |
| task update | `--title` | | `""` |
| task update | `--description` | `-d` | `""` |
| task update | `--status` | | `""` |
| task update | `--assigned-to` | | `""` |
| task update | `--effort` | | `""` |
| task update | `--priority` | | `""` |
| task update | `--force` | | `false` |
| task update | `--add-blocked-by` | | `[]` |
| task update | `--remove-blocked-by` | | `[]` |
| task update | `--clear-blocked-by` | | `false` |
| task update | `--add-tag` | | `[]` |
| task update | `--remove-tag` | | `[]` |
| task update | `--blocked` | | `""` |
| task update | `--unblock` | | `false` |
| task update | `--stale-timeout` | | `""` |
| task update | `--track` | | `""` |
| task delete | `--yes` | `-y` | `false` |
| task claim | `--note` | | `""` |
| task unclaim | `--note` | | `""` |
| task assign | `--note` | | `""` |
| task complete | `--note` | | `""` |
| task complete | `--no-verify` | | `false` |
| task reopen | `--note` | | `""` |
| task unassign | `--note` | | `""` |

### track Subcommand Flags

| Subcommand | Flag | Short | Default |
|------------|------|-------|---------|
| track list | `--status` | | `[]` |
| track list | `--state` | | `""` |
| track list | `--type` | | `""` |
| track list | `--limit` | `-n` | `100` |
| track list | `--offset` | | `0` |
| track list | `--all-projects` | | `false` |

### log Subcommand Flags

| Subcommand | Flag | Short | Default |
|------------|------|-------|---------|
| log | `--task-id` | | `""` |
| log | `--action` | | `""` |
| log | `--by` | | `""` |
| log | `--since` | | `""` |
| log | `--until` | | `""` |
| log | `--limit` | `-n` | `100` |
| log | `--offset` | | `0` |
| log | `--sort` | | `desc` |
| log | `--all` | | `false` |

## Conflicts and Issues

### `-v` / `-V` Version vs Verbose

- **kit contract**: `-v` = `--version` (via fang)
- **tlc**: `-V` = `--verbose` (uppercase to avoid clash) -- **correct**
- **xray**: `-V` = `--version`, `-v` = `--verbose` -- **reversed**
- **ctxt**: `-v` = `--version`, `-V` = `--verbose` -- **matches kit**
- **rsx**: `-v` = `--version` only -- **matches kit**

tlc and ctxt are aligned. xray is non-Go (commander.js), different
ecosystem.

### `--format` naming

- **kit contract**: `--format` (table, json, yaml)
- **tlc**: `--format` with `-f` shorthand -- **aligned**
- **ctxt**: `--output` instead of `--format` -- **drift**
- **rsx**: `--json`/`--markdown` booleans -- **different paradigm**
- **xray**: no structured output -- **n/a**

tlc is aligned with kit. ctxt/rsx predate kit adoption.

### help subcommand

- **kit contract**: no help subcommand; -h/--help only
- **tlc**: has `help` subcommand with `help llm` child -- **violation**
- Action: T-0508 to remove; `help llm` needs alternate home

### completion subcommand

- **kit contract**: disabled
- **tlc**: disabled via kit/cli -- **aligned**
- **ctxt**: present -- **drift** (pre-kit)

## Summary

tlc global flags are well-aligned with kit/cli contract. Key actions:

1. Remove `help` subcommand (T-0508); relocate `help llm` to root
2. No flag rename needed; shortnames are stable
3. `--format` + `-f` already wired through kit/output
4. `--quiet`, `--no-color`, `--no-hints` already wired through kit/cli
