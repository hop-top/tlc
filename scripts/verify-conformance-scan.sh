#!/usr/bin/env bash
#
# verify-conformance-scan.sh — fail when a conformance scan read nothing.
#
# A scanner that reads zero files exits 0 and reports a clean pass, which
# is indistinguishable from a real pass unless someone reads the counts.
# This guard reads the counts and refuses an empty scan.
#
# Accepts, in any mix:
#   - a single-leaf report (`kit conformance verify-no-leak --format=json`
#     output: an object carrying `scanned_files`);
#   - the aggregate report written by the 12fcc action (12fc-report.json:
#     an object with a `leaves` array, each leaf carrying `payload`).
#
# Leaf output can carry text ahead of the JSON object (a stderr warning
# merged by the action's `2>&1`); the first object found is used.
#
# A leaf whose scan count cannot be determined fails too: a count that
# cannot be read proves nothing was scanned.
#
# Usage:
#   scripts/verify-conformance-scan.sh [--min N] <report.json> [<report.json>...]
#
# Exit: 0 every leaf scanned >= N files (default 1); 1 a leaf scanned fewer
# or its count is unreadable; 2 usage error.
set -euo pipefail

min=1
files=()
while [ $# -gt 0 ]; do
  case "$1" in
    --min)
      [ $# -ge 2 ] || { echo "verify-conformance-scan: --min needs a value" >&2; exit 2; }
      min="$2"; shift 2 ;;
    -h|--help)
      sed -n '2,/^set -euo/p' "$0" | sed '$d' | sed 's/^# \{0,1\}//'
      exit 0 ;;
    -*)
      echo "verify-conformance-scan: unknown flag: $1" >&2; exit 2 ;;
    *)
      files+=("$1"); shift ;;
  esac
done

case "$min" in
  ''|*[!0-9]*) echo "verify-conformance-scan: --min must be a non-negative integer" >&2; exit 2 ;;
esac
[ ${#files[@]} -gt 0 ] || { echo "verify-conformance-scan: no report file given" >&2; exit 2; }

# -I keeps python from importing modules out of the caller's cwd. A
# version-manager python3 shim may still read the cwd's tool config, so
# run this from a trusted checkout.
exec python3 -I - "$min" "${files[@]}" <<'PY'
import json
import sys

min_files = int(sys.argv[1])
paths = sys.argv[2:]
decoder = json.JSONDecoder()


def first_object(text):
    """Return the first JSON object found in text, or None."""
    i = text.find("{")
    while i != -1:
        try:
            obj, _ = decoder.raw_decode(text[i:])
        except ValueError:
            i = text.find("{", i + 1)
            continue
        if isinstance(obj, dict):
            return obj
        i = text.find("{", i + 1)
    return None


def count_of(doc):
    n = doc.get("scanned_files")
    return n if isinstance(n, int) and not isinstance(n, bool) else None


# (label, scanned count or None)
rows = []
for path in paths:
    try:
        with open(path, encoding="utf-8") as fh:
            text = fh.read()
    except OSError as exc:
        rows.append((f"{path}", None, f"unreadable: {exc.strerror}"))
        continue
    doc = first_object(text)
    if doc is None:
        rows.append((path, None, "no JSON object found"))
        continue
    if isinstance(doc.get("leaves"), list):
        if not doc["leaves"]:
            rows.append((path, None, "aggregate report lists no leaves"))
        for leaf in doc["leaves"]:
            name = leaf.get("name", "?") if isinstance(leaf, dict) else "?"
            payload = leaf.get("payload") if isinstance(leaf, dict) else None
            inner = None
            if isinstance(payload, dict):
                inner = payload
                if "scanned_files" not in payload and isinstance(payload.get("raw"), str):
                    inner = first_object(payload["raw"])
            n = count_of(inner) if isinstance(inner, dict) else None
            why = "" if n is not None else "scanned_files missing from leaf output"
            rows.append((f"{path}: {name}", n, why))
    else:
        n = count_of(doc)
        tool = doc.get("tool", "?")
        why = "" if n is not None else "scanned_files missing"
        rows.append((f"{path}: {tool}", n, why))

bad = False
for label, n, why in rows:
    if n is None:
        print(f"FAIL {label}: {why}; cannot show anything was scanned")
        bad = True
    elif n < min_files:
        print(f"FAIL {label}: scanned_files={n} (need >= {min_files}); an empty scan is not a pass")
        bad = True
    else:
        print(f"ok   {label}: scanned_files={n}")

sys.exit(1 if bad else 0)
PY
