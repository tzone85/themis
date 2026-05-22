.PHONY: build test cover cover-html lint vet vulncheck ci clean

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

ci: vet lint test cover vulncheck
	bash scripts/cover_check.sh

clean:
	rm -f coverage.out coverage.html
