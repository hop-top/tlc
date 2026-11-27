#!/usr/bin/env bash
#
# verify-conformance-scan.test.sh — sample-input checks for the empty-scan
# guard. Each case writes a report, runs the guard, and compares the exit
# code. No network, no kit binary needed.
#
# Usage: scripts/verify-conformance-scan.test.sh
set -euo pipefail

here=$(cd "$(dirname "$0")" && pwd)
guard="$here/verify-conformance-scan.sh"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

fail=0
# expect <want-exit> <name> <guard args...>
expect() {
  local want=$1 name=$2 got=0
  shift 2
  "$guard" "$@" >"$tmp/out" 2>&1 || got=$?
  if [ "$got" -eq "$want" ]; then
    printf 'ok   %s (exit %s)\n' "$name" "$got"
  else
    printf 'FAIL %s: want exit %s, got %s\n' "$name" "$want" "$got"
    sed 's/^/     /' "$tmp/out"
    fail=1
  fi
}

# Single-leaf kit output.
cat >"$tmp/leaf-real.json" <<'J'
{"tool":"verify-no-leak","rules_version":"x","scanned_files":568,"findings":[]}
J
cat >"$tmp/leaf-zero.json" <<'J'
{"tool":"verify-no-leak","rules_version":"x","scanned_files":0,"findings":[],"skipped":[{"file":"/w/cmd","reason":"unsupported extension"}]}
J
# Warning text ahead of the object, as the action's 2>&1 produces.
cat >"$tmp/leaf-warn-zero.json" <<'J'
verify-no-leak: warning: 0 files scanned — no scannable files under --paths
{"tool":"verify-no-leak","scanned_files":0,"findings":[]}
J
printf 'not json at all\n' >"$tmp/garbage.json"
printf '{"tool":"verify-no-leak","findings":[]}\n' >"$tmp/no-count.json"

# Aggregate report written by the action.
cat >"$tmp/agg-real.json" <<'J'
{"leaves":[{"name":"verify-no-leak","exitCode":0,"status":"pass","payload":{"tool":"verify-no-leak","scanned_files":568,"findings":[]}}]}
J
cat >"$tmp/agg-zero.json" <<'J'
{"leaves":[
 {"name":"verify-no-leak","exitCode":0,"status":"pass","payload":{"tool":"verify-no-leak","scanned_files":0,"findings":[],"skipped":[{"file":"/w/cmd","reason":"unsupported extension"}]}},
 {"name":"verify-stories","exitCode":0,"status":"pass","payload":{"tool":"verify-stories","scanned_files":0,"summary":{"errors":0,"warnings":0},"findings":[]}}]}
J
# One leaf real, one empty: still a failure.
cat >"$tmp/agg-mixed.json" <<'J'
{"leaves":[
 {"name":"verify-no-leak","payload":{"scanned_files":568}},
 {"name":"verify-stories","payload":{"scanned_files":0}}]}
J
# Leaf output the action could not parse as JSON alone: payload.raw.
cat >"$tmp/agg-raw-real.json" <<'J'
{"leaves":[{"name":"verify-no-leak","payload":{"raw":"warning: x\n{\"scanned_files\": 7, \"findings\": []}"}}]}
J
cat >"$tmp/agg-raw-zero.json" <<'J'
{"leaves":[{"name":"verify-no-leak","payload":{"raw":"warning: 0 files\n{\"scanned_files\": 0, \"findings\": []}"}}]}
J
cat >"$tmp/agg-raw-unreadable.json" <<'J'
{"leaves":[{"name":"verify-no-leak","payload":{"raw":"panic: boom"}}]}
J
printf '{"leaves":[]}\n' >"$tmp/agg-empty.json"

expect 0 "leaf with scanned files passes"          "$tmp/leaf-real.json"
expect 1 "leaf with zero files fails"              "$tmp/leaf-zero.json"
expect 1 "warning-prefixed zero-file leaf fails"   "$tmp/leaf-warn-zero.json"
expect 1 "non-JSON input fails"                    "$tmp/garbage.json"
expect 1 "missing scanned_files fails"             "$tmp/no-count.json"
expect 1 "missing file fails"                      "$tmp/does-not-exist.json"
expect 0 "aggregate with scanned files passes"     "$tmp/agg-real.json"
expect 1 "aggregate with zero-file leaves fails"   "$tmp/agg-zero.json"
expect 1 "aggregate with one empty leaf fails"     "$tmp/agg-mixed.json"
expect 0 "aggregate raw payload with files passes" "$tmp/agg-raw-real.json"
expect 1 "aggregate raw payload, zero files fails" "$tmp/agg-raw-zero.json"
expect 1 "aggregate raw payload, no count fails"   "$tmp/agg-raw-unreadable.json"
expect 1 "aggregate with no leaves fails"          "$tmp/agg-empty.json"
expect 0 "several reports, all real"               "$tmp/leaf-real.json" "$tmp/agg-real.json"
expect 1 "several reports, one empty"              "$tmp/leaf-real.json" "$tmp/agg-zero.json"
expect 1 "--min above the count fails"             --min 1000 "$tmp/leaf-real.json"
expect 0 "--min 0 accepts an empty scan"           --min 0 "$tmp/leaf-zero.json"
expect 2 "no file is a usage error"
expect 2 "bad --min is a usage error"              --min x "$tmp/leaf-real.json"
expect 2 "unknown flag is a usage error"           --bogus "$tmp/leaf-real.json"

exit "$fail"
