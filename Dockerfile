# Themis — multi-stage build.
#
# Builder: official golang:alpine matching go.mod toolchain.
# Runtime: distroless static-nonroot — no shell, no package manager,
# UID 65532. Image size target < 30 MB.
#
# Build args VERSION/COMMIT/DATE are forwarded into the same ldflags
# wiring `make build` uses, so `themis --version` is identical whether
# the binary came from `make build`, goreleaser, or `docker build`.

# syntax=docker/dockerfile:1.7

ARG GO_VERSION=1.26.4
ARG ALPINE_VERSION=3.22

FROM golang:${GO_VERSION}-alpine${ALPINE_VERSION} AS builder

# git is required by `go build` only when the build references a VCS
# (it's also nice for `git describe` if the build is driven inside the
# image). Keep the layer minimal.
RUN apk add --no-cache git ca-certificates

WORKDIR /src

# Cache module downloads in a dedicated layer.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Source comes after deps so an unchanged dep set hits the cache.
COPY cmd/    ./cmd/
COPY internal/ ./internal/
COPY actions/ ./actions/

ARG VERSION=dev
ARG COMMIT=none
ARG DATE=unknown

# CGO_ENABLED=0 + GOOS=linux + static distroless target. -trimpath strips
# /src/ from the binary for reproducibility and to keep panic stacks
# clean. -s -w drops the symbol table + DWARF — operators don't need
# them on the runtime image; if they're needed for postmortem, rebuild
# without -s -w from the same commit.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux \
    go build -trimpath \
      -ldflags "-s -w \
        -X github.com/tzone85/themis/internal/cli.Version=${VERSION} \
        -X github.com/tzone85/themis/internal/cli.Commit=${COMMIT} \
        -X github.com/tzone85/themis/internal/cli.Date=${DATE}" \
      -o /out/themis ./cmd/themis


FROM gcr.io/distroless/static-debian12:nonroot AS runtime

# Image labels are picked up by `docker inspect` and rendered on the
# GitHub Container Registry package page.
LABEL org.opencontainers.image.title="themis" \
      org.opencontainers.image.description="Compliance gateway for AI-generated code" \
      org.opencontainers.image.source="https://github.com/tzone85/themis" \
      org.opencontainers.image.licenses="Apache-2.0"

COPY --from=builder /out/themis /themis

# Distroless nonroot uses UID:GID 65532:65532. Operator-supplied volumes
# bound to /data must be readable+writable by that UID — see
# docs/ops/deployment.md.
USER nonroot:nonroot

# REST API + dashboard. Operators publish or proxy this port.
EXPOSE 8787

ENTRYPOINT ["/themis"]
