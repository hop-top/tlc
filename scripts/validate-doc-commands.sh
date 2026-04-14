#!/usr/bin/env bash
# validate-doc-commands.sh — verify tlc command examples in docs are syntactically valid
# Runs each example in --help mode; exits non-zero if any unknown command/flag found.
# Usage: ./scripts/validate-doc-commands.sh
# Refs: tlc/T-0067

set -euo pipefail

export PATH="/opt/homebrew/bin:$HOME/.local/bin:$PATH"
TLC="${TLC:-tlc}"
PASS=0; FAIL=0
ERRORS=()

check() {
  local label="$1"; shift
  local cmd=("$@")
  # Probe with --help; unknown subcommands/flags print to stderr and exit non-zero
  if "${cmd[@]}" --help >/dev/null 2>&1; then
    echo "  OK  $label"
    ((PASS++)) || true
  else
    echo " FAIL $label"
    ERRORS+=("$label: ${cmd[*]}")
    ((FAIL++)) || true
  fi
}

# check_flag: verify a flag exists in --help output (for flags that require a value)
check_flag() {
  local label="$1"; shift
  local subcmd=("$@")
  # Extract just the flag name (last arg) — bash 3.2 compat (no negative indices)
  local flag="${subcmd[${#subcmd[@]}-1]}"
  local parent=("${subcmd[@]:0:${#subcmd[@]}-1}")
  if "${parent[@]}" --help 2>&1 | grep -qF -- "$flag"; then
    echo "  OK  $label"
    ((PASS++)) || true
  else
    echo " FAIL $label"
    ERRORS+=("$label: flag $flag not found in ${parent[*]} --help")
    ((FAIL++)) || true
  fi
}

echo "=== tlc doc command validation ==="
echo

echo "-- top-level subcommands --"
check "tlc task"         $TLC task
check "tlc flow"         $TLC flow
check "tlc assignee"     $TLC assignee
check "tlc log"          $TLC log
check "tlc auth"         $TLC auth
check "tlc config"       $TLC config
check "tlc doctor"       $TLC doctor
check "tlc init"         $TLC init
check "tlc label"        $TLC label
check "tlc project"      $TLC project
check "tlc sync"         $TLC sync
check "tlc tag"          $TLC tag
check "tlc tui"          $TLC tui
check "tlc upgrade"      $TLC upgrade
check "tlc uri"          $TLC uri
check "tlc version"      $TLC version
check "tlc workflow"     $TLC workflow

echo
echo "-- task subcommands --"
check "task assign"      $TLC task assign
check "task claim"       $TLC task claim
check "task complete"    $TLC task complete
check "task create"      $TLC task create
check "task delete"      $TLC task delete
check "task list"        $TLC task list
check "task stale"       $TLC task stale
check "task reopen"      $TLC task reopen
check "task show"        $TLC task show
check "task unassign"    $TLC task unassign
check "task unclaim"     $TLC task unclaim
check "task update"      $TLC task update

echo
echo "-- flow subcommands --"
check "flow import"      $TLC flow import
check "flow invoke"      $TLC flow invoke
check "flow list"        $TLC flow list
check "flow run"         $TLC flow run
check "flow status"      $TLC flow status
check "flow test"        $TLC flow test

echo
echo "-- flow test flags --"
check_flag "flow test --record"        $TLC flow test --record
check_flag "flow test --passthrough"   $TLC flow test --passthrough
check_flag "flow test --keep-sandbox"  $TLC flow test --keep-sandbox
check_flag "flow test --steps"         $TLC flow test --steps

echo
echo "-- key flag checks --"
check "task complete --no-verify"  $TLC task complete --no-verify
check "task delete --yes"          $TLC task delete --yes
check "task list --mine"           $TLC task list --mine
check "task list --summary"        $TLC task list --summary
check "task list --counters"       $TLC task list --counters
check_flag "task list --workspace"      $TLC task list --workspace
check_flag "task list --space"          $TLC task list --space
check      "task list --stale"          $TLC task list --stale
check      "task list --blocked"        $TLC task list --blocked
check_flag "task list --priority"       $TLC task list --priority
check_flag "task list --blocked-by"     $TLC task list --blocked-by
check      "task stale --run-hooks"     $TLC task stale --run-hooks
check      "task update --force"        $TLC task update --force
check_flag "task update --add-tag"           $TLC task update --add-tag
check_flag "task update --remove-tag"        $TLC task update --remove-tag
check_flag "task update --add-blocked-by"    $TLC task update --add-blocked-by
check_flag "task update --remove-blocked-by" $TLC task update --remove-blocked-by
check_flag "task update --blocked"           $TLC task update --blocked
check      "task update --unblock"           $TLC task update --unblock
check_flag "task update --timeout"           $TLC task update --timeout
check_flag "task create --timeout"           $TLC task create --timeout

echo
echo "-- track subcommands --"
check "track create"     $TLC track create
check "track list"       $TLC track list
check "track show"       $TLC track show
check "track update"     $TLC track update
check "track archive"    $TLC track archive
check "track abandon"    $TLC track abandon
check "track delete"     $TLC track delete
check "track summary"    $TLC track summary
check "track exec"       $TLC track exec

echo
echo "-- agent subcommands --"
check "tlc agent"        $TLC agent
check "agent run"        $TLC agent run
check "agent watch"      $TLC agent watch
check "agent status"     $TLC agent status
check "agent cancel"     $TLC agent cancel
check "agent list"       $TLC agent list

echo
echo "-- task exec --"
check "task exec"        $TLC task exec

echo
echo "-- agent/exec flag checks --"
check_flag "agent run --agent"          $TLC agent run --agent
check_flag "agent run --async"          $TLC agent run --async
check_flag "agent run --local"          $TLC agent run --local
check_flag "agent run --trust-project"  $TLC agent run --trust-project
check_flag "task exec --agent"          $TLC task exec --agent
check_flag "task exec --local"          $TLC task exec --local
check_flag "track exec --agent"         $TLC track exec --agent
check_flag "track exec --local"         $TLC track exec --local

echo
echo "-- schema command --"
if $TLC schema >/dev/null 2>&1; then
  echo "  OK  tlc schema"
  ((PASS++)) || true
else
  echo " FAIL tlc schema"
  ERRORS+=("tlc schema")
  ((FAIL++)) || true
fi

echo
echo "=== Results: $PASS passed, $FAIL failed ==="
if [ ${#ERRORS[@]} -gt 0 ]; then
  echo
  echo "Failed commands:"
  for e in "${ERRORS[@]}"; do
    echo "  - $e"
  done
  exit 1
fi
