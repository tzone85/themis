.PHONY: build test cover cover-html lint vet vulncheck vulncheck-advisory ci clean

BIN := ~/.local/bin/themis
PKGS := ./...

# Build identity — injected into internal/cli at link time so `themis
# --version` carries semver, commit, and build date. A clean `go build`
# (no ldflags) still works; the defaults in internal/cli/root.go take
# over and the binary identifies itself as "dev (commit none, …)".
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X github.com/tzone85/themis/internal/cli.Version=$(VERSION) \
           -X github.com/tzone85/themis/internal/cli.Commit=$(COMMIT) \
           -X github.com/tzone85/themis/internal/cli.Date=$(DATE)

build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/themis

test:
	go test -race -count=1 $(PKGS)

cover:
	go test -race -count=1 -coverprofile=coverage.out -covermode=atomic $(PKGS)
	go tool cover -func=coverage.out | tail -1

cover-html: cover
	go tool cover -html=coverage.out -o coverage.html

lint:
	golangci-lint run

vet:
	go vet $(PKGS)

vulncheck:
	go install golang.org/x/vuln/cmd/govulncheck@latest
	govulncheck $(PKGS)

# vulncheck-advisory runs the same scan but does not fail the build —
# used in `ci` so a stdlib-vuln advisory (which can only be cleared by
# bumping the Go toolchain on the host) doesn't block local development.
# Run `make vulncheck` standalone to gate releases.
vulncheck-advisory:
	@go install golang.org/x/vuln/cmd/govulncheck@latest
	@govulncheck $(PKGS) || echo "[vulncheck] ADVISORY: vulnerabilities reported (see above); upgrade the Go toolchain to clear stdlib findings."

ci: vet lint test cover vulncheck-advisory
	bash scripts/cover_check.sh

clean:
	rm -f coverage.out coverage.html
