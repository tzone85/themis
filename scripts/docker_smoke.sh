#!/usr/bin/env bash
#
# docker_smoke.sh — verify the production container image works end-to-end.
#
# Phases:
#   1. docker build
#   2. image size assertion (< MAX_MB)
#   3. --version smoke
#   4. `tenant init` against a host-mounted tmpdir as nonroot UID 65532
#      (catches volume-permission regressions)
#
# Usage:
#   bash scripts/docker_smoke.sh                 # uses HEAD metadata
#   IMAGE=themis:local MAX_MB=30 bash scripts/docker_smoke.sh
#
# Exit 0 on full success, non-zero on any phase failure.

set -euo pipefail

IMAGE="${IMAGE:-themis:smoke}"
MAX_MB="${MAX_MB:-30}"

VERSION="$(git describe --tags --always --dirty 2>/dev/null || echo dev)"
COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo none)"
DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

step() { printf '\n=== %s ===\n' "$*"; }

# 1. Build.
step "build $IMAGE"
docker build \
  --build-arg VERSION="$VERSION" \
  --build-arg COMMIT="$COMMIT" \
  --build-arg DATE="$DATE" \
  -t "$IMAGE" .

# 2. Image size.
step "image size <= ${MAX_MB} MB"
SIZE_BYTES="$(docker image inspect "$IMAGE" --format '{{.Size}}')"
SIZE_MB=$(( SIZE_BYTES / 1024 / 1024 ))
echo "image size: ${SIZE_MB} MB"
if [ "$SIZE_MB" -gt "$MAX_MB" ]; then
  echo "FAIL: image is ${SIZE_MB} MB, exceeds ${MAX_MB} MB budget" >&2
  exit 1
fi

# 3. --version smoke.
step "themis --version"
VERSION_OUT="$(docker run --rm "$IMAGE" --version)"
echo "$VERSION_OUT"
case "$VERSION_OUT" in
  *"$VERSION"*) ;;
  *) echo "FAIL: --version output missing $VERSION" >&2; exit 1;;
esac

# 4. tenant init against host-mounted volume as nonroot.
step "tenant init under nonroot UID 65532 + host volume"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
# Permit nonroot UID 65532 to write — mirrors operator runbook.
chmod 777 "$TMP"
docker run --rm -v "$TMP":/data "$IMAGE" \
  tenant init --id smoke --base /data
test -d "$TMP/tenants/smoke" || {
  echo "FAIL: tenants/smoke not created under $TMP" >&2
  ls -la "$TMP" >&2
  exit 1
}

echo
echo "OK: docker smoke passed for $IMAGE (${SIZE_MB} MB)"
