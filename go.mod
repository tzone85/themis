module github.com/tzone85/themis

go 1.26.3

// Toolchain pinned higher than the `go` directive so go select the patched
// stdlib (1.26.4) at build time even when the host's setup-go has only the
// 1.26.3 manifest entry. Cleared 2 stdlib vulns (GO-2026-5037/5039).
toolchain go1.26.4

require (
	github.com/spf13/cobra v1.10.2
	gopkg.in/yaml.v3 v3.0.1
	modernc.org/sqlite v1.50.1
	pgregory.net/rapid v1.2.0
)

require (
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/spf13/pflag v1.0.9 // indirect
	golang.org/x/sys v0.44.0 // indirect
	modernc.org/libc v1.72.3 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
)
