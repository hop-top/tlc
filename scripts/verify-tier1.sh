#!/usr/bin/env bash
#
# verify-tier1.sh — guardrail that asserts the dogfood-grade workflow
# never asks for Tier 2 / Tier 3 grader output.
#
# Tier 3 traces (assertion-level diffs, prompt text, cassette excerpts)
# MUST stay private. The private grader repo hard-codes `--tier=1`; this
# script is the public-side belt-and-suspenders: it greps the supplied
# workflow file for any `--tier=2` / `--tier=3` reference and fails CI if
# found. actionlint can't express this rule natively, so we ship a shell
# guard instead.
#
# Usage:
#   scripts/verify-tier1.sh <workflow-file> [<workflow-file>...]
#
# Default target if no args: .github/workflows/dogfood-grade.yml.
set -euo pipefail

die() { printf 'verify-tier1: %s\n' "$*" >&2; exit 1; }

targets=("$@")
if [ ${#targets[@]} -eq 0 ]; then
  targets=(".github/workflows/dogfood-grade.yml")
fi

bad=0
for f in "${targets[@]}"; do
  [ -f "$f" ] || die "missing file: $f"
  # Strip YAML comments (everything from `#` to EOL, but only when `#`
  # follows whitespace or starts the line, so URL fragments don't break
  # this) before grepping. We only want to flag real `--tier=2` /
  # `--tier=3` references in workflow code, not in comments documenting
  # the rule.
  stripped=$(sed -E 's/(^|[[:space:]])#.*$//' "$f")
  # Match --tier=2 / --tier=3 / --tier 2 / --tier 3, with optional quoting.
  # The trailing (not-a-digit|end) guard prevents false positives on values
  # like --tier=23 (not a real tier, but must not trip the guard).
  if printf '%s\n' "$stripped" | grep -E -n -- "--tier[= ]['\"]?[23]([^0-9]|$)"; then
    echo "  in: $f" >&2
    bad=1
  fi
done

if [ "$bad" -ne 0 ]; then
  die "Tier 2 / Tier 3 flags are forbidden in public workflows (private-content firewall)"
fi

echo "verify-tier1: ok — no Tier 2 / Tier 3 references in ${targets[*]}"
