// Command cyanotype generates instruction files for coding agents.
package main

import (
	"fmt"
	"os"
)

// version is set at build time with:
//
//	go build -ldflags "-X main.version=v0.1.0" ./cmd/cyanotype
var version = "dev"

func main() {
	err := newRootCmd(version).Execute()
	code, report := exitCode(err)
	if report {
		fmt.Fprintln(os.Stderr, "Error:", err)
	}
	os.Exit(code)
}
