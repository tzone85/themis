#!/usr/bin/env bash
# themis-check.sh
#
# Invoked by the GitHub Action wrapper (actions/themis-check/action.yml).
# Computes a git-diff-based AIChange against the PR base, POSTs it to a
# running Themis REST server, surfaces the verdict, and exits non-zero on
# DENY (and on REQUIRE_APPROVAL if THEMIS_FAIL_ON_REQUIRE_APPROVAL=true).
#
# The script intentionally uses only POSIX shell + curl + jq + git so it
# runs on any GitHub Actions runner image without extra dependencies.

set -euo pipefail

: "${THEMIS_BASE_URL:?THEMIS_BASE_URL is required}"
: "${THEMIS_TOKEN:?THEMIS_TOKEN is required}"
: "${THEMIS_TENANT_ID:?THEMIS_TENANT_ID is required}"
: "${THEMIS_POLICY_PATH:?THEMIS_POLICY_PATH is required}"

PR_ID="${THEMIS_PR_ID:-}"
if [[ -z "${PR_ID}" ]]; then
  PR_ID="gh:${GH_REPOSITORY:-unknown/unknown}#${GH_PR_NUMBER:-0}"
fi

BASE_SHA="${GH_BASE_SHA:-HEAD~1}"
HEAD_SHA="${GH_HEAD_SHA:-HEAD}"
ACTOR="${THEMIS_ACTOR:-}"
FAIL_ON_RA="${THEMIS_FAIL_ON_REQUIRE_APPROVAL:-false}"

if ! command -v jq >/dev/null 2>&1; then
  echo "themis-check: jq is required on the runner" >&2
  exit 2
fi
if ! command -v curl >/dev/null 2>&1; then
  echo "themis-check: curl is required on the runner" >&2
  exit 2
fi

# Build an AIChange JSON from the diff. Each entry includes a SHA-256 of the
# file content at HEAD (or empty for deletions) and at BASE (or empty for
# additions). We avoid `themis` itself here so the action stays a tiny shim.

if [[ -z "${ACTOR}" ]]; then
  AUTHOR_EMAIL="$(git log -1 --format='%ae' "${HEAD_SHA}")"
  ACTOR="human:${AUTHOR_EMAIL}"
fi

# Emit a single FileTouch object for each name-status entry.
TOUCHES_JSON=$(git diff --name-status --no-renames "${BASE_SHA}..${HEAD_SHA}" \
  | awk -v base="${BASE_SHA}" -v head="${HEAD_SHA}" '
      function sha256(ref, path,   cmd, out) {
        cmd = "git show " ref ":" path " 2>/dev/null | shasum -a 256 | awk \"{print \\$1}\""
        cmd | getline out
        close(cmd)
        return out
      }
      BEGIN { printf "[" }
      {
        kind = ""
        if      ($1 == "A") kind = "ADDED"
        else if ($1 == "M") kind = "MODIFIED"
        else if ($1 == "D") kind = "DELETED"
        else next
        path = $2
        before = (kind == "ADDED") ? "" : sha256(base, path)
        after  = (kind == "DELETED") ? "" : sha256(head, path)
        if (NR > 1) printf ","
        printf "{\"path\":\"%s\",\"change_kind\":\"%s\",\"before_hash\":\"%s\",\"after_hash\":\"%s\"}", path, kind, before, after
      }
      END { print "]" }')

if [[ "${TOUCHES_JSON}" == "[]" ]]; then
  echo "themis-check: no diff between ${BASE_SHA}..${HEAD_SHA}; allowing trivially." >&2
  echo "verdict=ALLOW" >> "${GITHUB_OUTPUT:-/dev/null}"
  echo "decision-json={\"verdict\":\"ALLOW\",\"reason\":\"empty diff\"}" >> "${GITHUB_OUTPUT:-/dev/null}"
  exit 0
fi

POLICY_BODY=$(cat "${THEMIS_POLICY_PATH}")

REQUEST_BODY=$(jq -n \
  --arg pr "${PR_ID}" \
  --arg actor "${ACTOR}" \
  --argjson touches "${TOUCHES_JSON}" \
  --arg policy "${POLICY_BODY}" '{
    ai_change: { pr_id: $pr, actor: $actor, touched_files: $touches },
    policy_yaml: $policy
  }')

RESPONSE=$(curl -sS -X POST \
  -H "Authorization: Bearer ${THEMIS_TOKEN}" \
  -H "Content-Type: application/json" \
  --data-raw "${REQUEST_BODY}" \
  "${THEMIS_BASE_URL%/}/v1/tenants/${THEMIS_TENANT_ID}/decide")

VERDICT=$(echo "${RESPONSE}" | jq -r '.decision.verdict // empty')
REASON=$(echo "${RESPONSE}" | jq -r '.decision.reason // ""')

if [[ -z "${VERDICT}" ]]; then
  echo "themis-check: server returned no verdict; full response below:" >&2
  echo "${RESPONSE}" >&2
  exit 3
fi

echo "themis-check: verdict=${VERDICT}${REASON:+ (${REASON})}"

if [[ -n "${GITHUB_OUTPUT:-}" ]]; then
  {
    echo "verdict=${VERDICT}"
    echo "decision-json<<__THEMIS_EOF__"
    echo "${RESPONSE}"
    echo "__THEMIS_EOF__"
  } >> "${GITHUB_OUTPUT}"
fi

case "${VERDICT}" in
  DENY)
    exit 1
    ;;
  REQUIRE_APPROVAL)
    if [[ "${FAIL_ON_RA}" == "true" ]]; then
      exit 1
    fi
    exit 0
    ;;
  ALLOW)
    exit 0
    ;;
  *)
    echo "themis-check: unknown verdict ${VERDICT}" >&2
    exit 3
    ;;
esac
