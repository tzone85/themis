#!/usr/bin/env bash
set -euo pipefail

# Reads coverage.out (go cover profile) and coverage.thresholds.yaml.
# Fails if global coverage or any per-package coverage is below threshold.

COVERAGE_FILE="${COVERAGE_FILE:-coverage.out}"
THRESHOLDS="${THRESHOLDS:-coverage.thresholds.yaml}"

if [[ ! -f "$COVERAGE_FILE" ]]; then
  echo "coverage profile $COVERAGE_FILE not found; run 'make cover' first" >&2
  exit 2
fi

# Global coverage from `go tool cover -func` final line.
GLOBAL_PCT=$(go tool cover -func="$COVERAGE_FILE" | awk '/^total:/ {gsub("%",""); print $3}')

# Per-package coverage from `go tool cover -func`.
PER_PKG=$(go tool cover -func="$COVERAGE_FILE" \
  | grep -v '^total:' || true \
  | awk '{gsub("%","",$3); pkg=$1; sub(/\/[^\/]+$/, "", pkg); cov[pkg]+=$3; cnt[pkg]++} END {for (k in cov) printf "%s %.1f\n", k, cov[k]/cnt[k]}')

# Global gate.
GLOBAL_TARGET=$(awk '/^global:/ {print $2}' "$THRESHOLDS")
echo "[cover-check] global: ${GLOBAL_PCT}% (target: ${GLOBAL_TARGET}%)"
if (( $(echo "$GLOBAL_PCT < $GLOBAL_TARGET" | bc -l) )); then
  echo "[cover-check] FAIL: global coverage ${GLOBAL_PCT}% below ${GLOBAL_TARGET}%"
  exit 1
fi

# Per-package gate.
FAIL=0
if [[ -n "$PER_PKG" ]]; then
  while read -r pkg pct; do
    TARGET=$(awk -v p="$pkg" '$1 == p":" {gsub(":",""); print $2}' "$THRESHOLDS")
    if [[ -z "${TARGET:-}" ]]; then TARGET="$GLOBAL_TARGET"; fi
    echo "[cover-check] $pkg: ${pct}% (target: ${TARGET}%)"
    if (( $(echo "$pct < $TARGET" | bc -l) )); then
      echo "[cover-check] FAIL: $pkg ${pct}% below ${TARGET}%"
      FAIL=1
    fi
  done <<< "$PER_PKG"
fi

if [[ "$FAIL" == "1" ]]; then exit 1; fi
echo "[cover-check] PASS"
