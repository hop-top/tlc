#!/usr/bin/env bash
# Fail while a test package is merely SLOW, not once it has already
# blown the timeout.
#
# internal/cli is one serialized package — every CLI test takes a single
# global mutex, because they mutate cobra flag variables and viper state
# and so cannot run in parallel — and a growing share of them shell out
# to a real built binary. Runtime therefore only ever goes up, one test
# at a time, and no single commit looks like the culprit.
#
# When it crossed go's 10m per-package default the failure mode was
# `panic: test timed out`, with a goroutine dump naming whichever test
# happened to be holding the lock. That test is innocent, so the dump
# sends the reader after the wrong code; it cost a real diagnosis
# already. Raising -timeout buys headroom but hides the trend, so this
# guard watches the trend instead: the budget below is well under the
# timeout, and crossing it is a normal red build with a clear message
# rather than a hang to be decoded.
#
# Raising a budget is a legitimate fix when the suite has honestly grown
# — do it deliberately, in a commit that says why, rather than by
# nudging -timeout until the symptom stops.

set -euo pipefail

log="${1:?usage: check-suite-runtime.sh <go-test-output>}"

if [[ ! -f "$log" ]]; then
    echo "check-suite-runtime: no test log at $log" >&2
    exit 1
fi

# "<package> <seconds>" per line. A plain list rather than an
# associative array: macOS still ships bash 3.2, where `declare -A` does
# not exist and the keys get evaluated as arithmetic, so the guard
# silently measures nothing.
#
# Keep each budget comfortably under the -timeout in ci.yml so this
# guard fires first and reports a number.
budgets='
hop.top/tlc/internal/cli 900
hop.top/tlc/internal/storage 300
'

status=0
measured=0

while read -r pkg max; do
    [[ -z "$pkg" ]] && continue
    # `ok  <pkg>  123.45s` — cached results print "(cached)" and carry no
    # duration, which is not a measurement and must not read as 0s.
    line="$(grep -E "^(ok|FAIL)[[:space:]]+${pkg}[[:space:]]" "$log" || true)"
    if [[ -z "$line" ]]; then
        continue
    fi
    secs="$(printf '%s\n' "$line" | grep -oE '[0-9]+\.[0-9]+s' | head -1 | tr -d 's' || true)"
    if [[ -z "$secs" ]]; then
        continue
    fi

    measured=$((measured + 1))
    printf '%-34s %8.1fs  (budget %ss)\n' "$pkg" "$secs" "$max"

    if (( $(printf '%.0f' "$secs") > max )); then
        status=1
        cat >&2 <<EOF

check-suite-runtime: ${pkg} took ${secs}s, over its ${max}s budget.

  This package is serialized behind one global mutex, so its runtime is
  the sum of its tests and only grows. Left alone it reaches the
  -timeout in ci.yml and fails as a hang whose goroutine dump names an
  innocent test.

  Fix the growth, or raise the budget in scripts/check-suite-runtime.sh
  deliberately and say why in the commit. Do not raise -timeout to
  silence this.
EOF
    fi
done <<EOF
$budgets
EOF

if (( measured == 0 )); then
    echo "check-suite-runtime: no timed results for the watched packages" >&2
    echo "  (a fully cached run measures nothing; re-run with -count=1)" >&2
fi

exit "$status"
