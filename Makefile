.PHONY: build test cover cover-html lint vet vulncheck vulncheck-advisory ci clean

BIN := ~/.local/bin/themis
PKGS := ./...

build:
	go build -o $(BIN) ./cmd/themis

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
