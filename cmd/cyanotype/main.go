// Command cyanotype generates instruction files for coding agents.
package main

import (
	"fmt"
	"os"
	"runtime/debug"
)

// version is set at build time with:
//
//	go build -ldflags "-X main.version=v0.1.0" ./cmd/cyanotype
//
// Release builds set it this way; see .goreleaser.yaml.
var version = "dev"

func main() {
	info, ok := debug.ReadBuildInfo()
	err := newRootCmd(resolveVersion(version, info, ok)).Execute()
	code, report := exitCode(err)
	if report {
		fmt.Fprintln(os.Stderr, "Error:", err)
	}
	os.Exit(code)
}
