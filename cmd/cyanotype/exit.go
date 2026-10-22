package main

import (
	"errors"
	"fmt"
)

// Exit codes of cyanotype, documented in docs/DESIGN.md so scripts can rely
// on them.
const (
	// codeOK: the command did what it was asked, or there was nothing to do.
	codeOK = 0
	// codeError: a problem in the spec, the catalog or the files, or an I/O
	// error.
	codeError = 1
	// codeUsage: wrong flags or arguments.
	codeUsage = 2
	// codeNotWritten: changes were found but not written, because of
	// --dry-run, a declined confirmation, or no terminal to ask on. Scripts
	// can run generate --dry-run to check that generated files are current.
	codeNotWritten = 3
	// codeDrift: cyanotype check found drift, errors or, with --strict,
	// warnings. It shares 3 with codeNotWritten, as both mean that running
	// generate, or fixing what the output names, is left to do.
	codeDrift = 3
)

// exitError carries the exit code a command ends with. err is nil when the
// command has already told the user what happened.
type exitError struct {
	code int
	err  error
}

func (e *exitError) Error() string {
	if e.err == nil {
		return fmt.Sprintf("exit code %d", e.code)
	}
	return e.err.Error()
}

func (e *exitError) Unwrap() error {
	return e.err
}

// usageError marks err as a mistake in flags or arguments.
func usageError(err error) error {
	return &exitError{code: codeUsage, err: err}
}

// exitCode returns the process exit code for the error a command returned,
// and whether main still has to print the error.
func exitCode(err error) (code int, report bool) {
	if err == nil {
		return codeOK, false
	}
	var e *exitError
	if errors.As(err, &e) {
		return e.code, e.err != nil
	}
	return codeError, true
}
