// Command cyanotype generates instruction files for coding agents.
package main

import (
	"os"
)

// version is set at build time with:
//
//	go build -ldflags "-X main.version=v0.1.0" ./cmd/cyanotype
var version = "dev"

func main() {
	if err := newRootCmd(version).Execute(); err != nil {
		os.Exit(1)
	}
}
