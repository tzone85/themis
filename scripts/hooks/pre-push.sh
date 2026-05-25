#!/usr/bin/env bash
# scripts/hooks/pre-push.sh
#
# Themis pre-push hook for local enforcement.
# Install:
#   cp scripts/hooks/pre-push.sh .git/hooks/pre-push
#   chmod +x .git/hooks/pre-push
#
# Environment:
#   THEMIS_BIN              — path to the themis binary (default: `themis` on PATH)
#   THEMIS_BASE             — base state directory (default: $HOME/.themis)
#   THEMIS_TENANT_ID        — tenant id (default: `local`)
#   THEMIS_POLICY           — path to policy YAML (default: $PWD/themis.yaml)
#   THEMIS_PR_ID            — PR identifier (default: derived from current branch name)
#   THEMIS_BASE_REF         — base ref to diff against (default: origin/main)
#   THEMIS_FAIL_ON_RA       — set to "true" to fail on REQUIRE_APPROVAL (default: false)
#
# Exit codes mirror the action: 0=ALLOW, 0=REQUIRE_APPROVAL (unless THEMIS_FAIL_ON_RA),
# 1=DENY, 2=configuration error, 3=server/pipeline error.

set -euo pipefail

THEMIS_BIN="${THEMIS_BIN:-themis}"
THEMIS_BASE="${THEMIS_BASE:-${HOME}/.themis}"
THEMIS_TENANT_ID="${THEMIS_TENANT_ID:-local}"
THEMIS_POLICY="${THEMIS_POLICY:-${PWD}/themis.yaml}"
THEMIS_BASE_REF="${THEMIS_BASE_REF:-origin/main}"
THEMIS_FAIL_ON_RA="${THEMIS_FAIL_ON_RA:-false}"

if ! command -v "${THEMIS_BIN}" >/dev/null 2>&1; then
  echo "themis pre-push: ${THEMIS_BIN} not on PATH; set THEMIS_BIN to the binary location" >&2
  exit 2
fi

if [[ ! -f "${THEMIS_POLICY}" ]]; then
  echo "themis pre-push: policy file not found at ${THEMIS_POLICY}; set THEMIS_POLICY or create themis.yaml" >&2
  exit 2
fi

# Default PR id is the current branch name; replace slashes so the file path
# stays inside one directory.
if [[ -z "${THEMIS_PR_ID:-}" ]]; then
  BRANCH=$(git rev-parse --abbrev-ref HEAD)
  THEMIS_PR_ID="local:${BRANCH//\//_}"
fi

echo "themis pre-push: tenant=${THEMIS_TENANT_ID} pr-id=${THEMIS_PR_ID} base=${THEMIS_BASE_REF}"

# Lazy-init the tenant state on first use so the hook works out-of-the-box.
if [[ ! -d "${THEMIS_BASE}/tenants/${THEMIS_TENANT_ID}" ]]; then
  "${THEMIS_BIN}" tenant init --id "${THEMIS_TENANT_ID}" --base "${THEMIS_BASE}"
fi

# Catalogue snapshot is optional; if THEMIS_CATALOGUE is set, sync it.
if [[ -n "${THEMIS_CATALOGUE:-}" && -d "${THEMIS_CATALOGUE}" ]]; then
  "${THEMIS_BIN}" catalogue sync \
    --id "${THEMIS_TENANT_ID}" --base "${THEMIS_BASE}" \
    --source "${THEMIS_CATALOGUE}"
fi

WORKDIR=$(git rev-parse --show-toplevel)
AICHANGE_DIR=$(mktemp -d)
trap 'rm -rf "${AICHANGE_DIR}"' EXIT

"${THEMIS_BIN}" ingest \
  --id "${THEMIS_TENANT_ID}" --base "${THEMIS_BASE}" \
  --adapter git_heuristic \
  --pr-id "${THEMIS_PR_ID}" \
  --workdir "${WORKDIR}" \
  --base-ref "${THEMIS_BASE_REF}" \
  --out "${AICHANGE_DIR}/aichange.json" >/dev/null

DECISION_JSON=$(mktemp)
"${THEMIS_BIN}" decide \
  --id "${THEMIS_TENANT_ID}" --base "${THEMIS_BASE}" \
  --aichange "${AICHANGE_DIR}/aichange.json" \
  --policy "${THEMIS_POLICY}" \
  --workdir "${WORKDIR}" \
  > "${DECISION_JSON}"

VERDICT=$(grep -E '"verdict"' "${DECISION_JSON}" | head -1 | sed -E 's/.*"verdict":[[:space:]]*"([^"]+)".*/\1/')
REASON=$(grep -E '"reason"' "${DECISION_JSON}" | head -1 | sed -E 's/.*"reason":[[:space:]]*"([^"]*)".*/\1/')

echo "themis pre-push: verdict=${VERDICT}${REASON:+ (${REASON})}"
rm -f "${DECISION_JSON}"

case "${VERDICT}" in
  DENY)
    exit 1
    ;;
  REQUIRE_APPROVAL)
    if [[ "${THEMIS_FAIL_ON_RA}" == "true" ]]; then
      exit 1
    fi
    exit 0
    ;;
  ALLOW)
    exit 0
    ;;
  *)
    echo "themis pre-push: unknown verdict (${VERDICT}); failing closed" >&2
    exit 3
    ;;
esac
