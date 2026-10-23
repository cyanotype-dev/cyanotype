package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/cyanotype-dev/cyanotype/internal/contract"
	"github.com/cyanotype-dev/cyanotype/internal/generate"
	"github.com/cyanotype-dev/cyanotype/internal/spec"
)

// specDiagnostics prints problems in a spec to stderr, in the
// "file:line: error: field: message" form editors and terminals link to.
// generate and check report a spec that does not resolve the same way.
type specDiagnostics struct {
	errOut io.Writer
	// color reports that errOut shows colors.
	color bool
	// specPath is the spec as the user knows it, such as
	// "demo/cyanotype.yaml".
	specPath string
}

// report prints every problem err holds, then how many there were and
// outcome, such as "nothing was written", and returns an exitError that
// ends the command with codeError. Problems in the spec name their line;
// YAML syntax errors, whose line yaml.v3 may report one off, say "near
// line" and show the lines around it.
func (d specDiagnostics) report(root *os.Root, err error, outcome string) error {
	errs := []error{err}
	var joined interface{ Unwrap() []error }
	if errors.As(err, &joined) {
		errs = joined.Unwrap()
	}
	label := paint(d.color, red, "error")
	for _, e := range errs {
		var ve *spec.ValidationError
		switch {
		case !errors.As(e, &ve):
			fmt.Fprintf(d.errOut, "%s: %v\n", label, e)
		case ve.Syntax && ve.Line > 0:
			msg := fmt.Sprintf("invalid YAML near line %d: %s", ve.Line, strings.TrimPrefix(ve.Msg, "invalid YAML: "))
			d.print(label, 0, "", msg)
			d.excerpt(root, ve.Line)
		default:
			d.print(label, ve.Line, ve.Field, ve.Msg)
		}
	}
	fmt.Fprintf(d.errOut, "%s; %s.\n", plural(len(errs), "error"), outcome)
	return &exitError{code: codeError}
}

// printOutput prints the warnings and errors of an output of
// internal/contract as report prints the errors the core returns: spec
// problems at their line, with the lines around an approximate one, other
// files under the project folder dir. It returns how many errors there
// were, so the caller can count them in its own words.
func (d specDiagnostics) printOutput(root *os.Root, dir string, diags contract.Diagnostics) int {
	for _, w := range diags.Warnings {
		d.printDiagnostic(root, dir, paint(d.color, yellow, "warning"), w)
	}
	for _, e := range diags.Errors {
		d.printDiagnostic(root, dir, paint(d.color, red, "error"), e)
	}
	return len(diags.Errors)
}

// printDiagnostic prints one diagnostic with its label.
func (d specDiagnostics) printDiagnostic(root *os.Root, dir, label string, diag contract.Diagnostic) {
	switch {
	case diag.File == "":
		fmt.Fprintf(d.errOut, "%s: %s\n", label, diag.Message)
	case diag.File != generate.SpecFile:
		loc := filepath.Join(dir, filepath.FromSlash(diag.File))
		if diag.Line > 0 {
			loc += fmt.Sprintf(":%d", diag.Line)
		}
		fmt.Fprintf(d.errOut, "%s: %s: %s\n", loc, label, diag.Message)
	case diag.LineApproximate:
		msg := fmt.Sprintf("invalid YAML near line %d: %s", diag.Line, strings.TrimPrefix(diag.Message, "invalid YAML: "))
		d.print(label, 0, "", msg)
		if root != nil {
			d.excerpt(root, diag.Line)
		}
	default:
		d.print(label, diag.Line, diag.Field, diag.Message)
	}
}

// print prints one problem in the spec.
func (d specDiagnostics) print(label string, line int, field, msg string) {
	loc := d.specPath
	if line > 0 {
		loc += fmt.Sprintf(":%d", line)
	}
	if field != "" {
		msg = field + ": " + msg
	}
	fmt.Fprintf(d.errOut, "%s: %s: %s\n", loc, label, msg)
}

// excerpt prints the lines of the spec around line, numbered.
func (d specDiagnostics) excerpt(root *os.Root, line int) {
	data, err := root.ReadFile(generate.SpecFile)
	if err != nil {
		return // the error itself has been printed; the excerpt is a bonus
	}
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	for n := max(line-1, 1); n <= min(line+1, len(lines)); n++ {
		fmt.Fprintf(d.errOut, "  %4d | %s\n", n, lines[n-1])
	}
}
