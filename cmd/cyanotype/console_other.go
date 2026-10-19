//go:build !windows

package main

import "os"

// enableANSI reports whether f can show ANSI escape sequences. Terminals
// outside Windows always can, so there is nothing to turn on.
func enableANSI(*os.File) bool {
	return true
}
