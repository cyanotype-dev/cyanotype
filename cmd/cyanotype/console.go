package main

import (
	"os"

	"golang.org/x/term"
)

// console describes the terminal the command runs in. It is detected once,
// when a command runs, and tests replace it with their own.
type console struct {
	// interactive reports that stdin is a terminal, so a command can ask
	// for confirmation. When it is false, nothing may read stdin: a script
	// or CI job would wait forever for an answer nobody gives.
	interactive bool
	// colorOut and colorErr report that stdout and stderr show ANSI colors.
	colorOut, colorErr bool
}

// detectConsole inspects the process's stdin, stdout and stderr.
func detectConsole() console {
	return console{
		interactive: isTerminal(os.Stdin),
		colorOut:    colorEnabled(isTerminal(os.Stdout), os.Getenv) && enableANSI(os.Stdout),
		colorErr:    colorEnabled(isTerminal(os.Stderr), os.Getenv) && enableANSI(os.Stderr),
	}
}

// isTerminal reports whether f is a terminal. x/term asks the operating
// system, so a pipe, a file or /dev/null is never mistaken for one.
func isTerminal(f *os.File) bool {
	return term.IsTerminal(int(f.Fd())) //nolint:gosec // descriptors and Windows handles fit in an int
}

// colorEnabled decides whether output to a stream may use colors: only on
// a terminal, never when the NO_COLOR environment variable is set to
// anything but "" (see no-color.org), and never on a terminal that declares
// itself dumb. getenv is os.Getenv outside tests.
func colorEnabled(terminal bool, getenv func(string) string) bool {
	return terminal && getenv("NO_COLOR") == "" && getenv("TERM") != "dumb"
}
