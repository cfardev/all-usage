// Package buildinfo exposes version metadata, optionally injected at build time:
//
//	go build -ldflags "-X github.com/cfardev/all-usage/internal/buildinfo.Version=v1.0.0"
package buildinfo

import "runtime/debug"

// Version is the release version. It is overridden with -ldflags at release time.
var Version = "dev"

// String returns the effective version, falling back to the module build info
// (set when installed with `go install ...@version`).
func String() string {
	if Version != "dev" {
		return Version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return Version
}

// UserAgent is the HTTP User-Agent sent with every outgoing request.
func UserAgent() string {
	return "all-usage/" + String() + " (+https://github.com/cfardev/all-usage)"
}
