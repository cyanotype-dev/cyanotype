package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	bundled "github.com/cyanotype-dev/cyanotype/catalog"
	"github.com/cyanotype-dev/cyanotype/internal/catalog"
	"github.com/cyanotype-dev/cyanotype/internal/generate"
	"github.com/cyanotype-dev/cyanotype/internal/resolve"
	"github.com/cyanotype-dev/cyanotype/internal/spec"
)

// generateOptions holds the flags of the generate command.
type generateOptions struct {
	dir    string
	dryRun bool
	yes    bool
}

// newGenerateCmd builds the generate command. detect tells it about the
// terminal when it runs.
func newGenerateCmd(detect func() console) *cobra.Command {
	var opts generateOptions
	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate instruction files and constraints from cyanotype.yaml",
		Long: "Generate reads cyanotype.yaml, resolves it against the bundled catalog and\n" +
			"shows the files it would write as a diff. It asks before writing, and never\n" +
			"deletes files.\n\n" +
			"Exit codes: 0 written or already up to date, 1 error, 2 wrong usage,\n" +
			"3 changes found but not written (--dry-run, declined, or no terminal to ask on).",
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if opts.dryRun && opts.yes {
				return usageError(errors.New("--dry-run and --yes cannot be used together"))
			}
			g := &generation{
				opts:     opts,
				in:       cmd.InOrStdin(),
				out:      cmd.OutOrStdout(),
				errOut:   cmd.ErrOrStderr(),
				console:  detect(),
				dirFlag:  cmd.Flags().Changed("dir"),
				specPath: filepath.Join(opts.dir, generate.SpecFile),
			}
			return g.run()
		},
	}
	cmd.Flags().StringVar(&opts.dir, "dir", ".", "project folder holding cyanotype.yaml")
	cmd.Flags().BoolVar(&opts.dryRun, "dry-run", false, "show the changes and write nothing")
	cmd.Flags().BoolVarP(&opts.yes, "yes", "y", false, "write the changes without asking")
	return cmd
}

// generation is one run of the generate command.
type generation struct {
	opts        generateOptions
	in          io.Reader
	out, errOut io.Writer
	console     console
	// dirFlag reports that --dir was given, so suggested commands repeat it.
	dirFlag bool
	// specPath is the spec as the user knows it, for messages.
	specPath string
}

func (g *generation) run() error {
	root, err := os.OpenRoot(g.opts.dir)
	if err != nil {
		return fmt.Errorf("open the project folder: %w", err)
	}
	defer root.Close()

	f, err := generate.LoadSpec(root)
	if errors.Is(err, generate.ErrNoSpec) {
		return fmt.Errorf("%w; run cyanotype in the folder that holds it, or pass --dir", err)
	}
	if err != nil {
		return g.reportErrors(root, err)
	}
	c, err := catalog.LoadRoot(bundled.FS())
	if err != nil {
		return fmt.Errorf("the bundled catalog is invalid, which is a bug in cyanotype: %w", err)
	}

	changes, warnings, err := generate.Preview(c, f, root.FS())
	g.printWarnings(warnings)
	if err != nil {
		return g.reportErrors(root, err)
	}

	pending := 0
	for _, ch := range changes {
		if ch.Status != generate.Unchanged {
			g.printDiff(ch.Diff())
			pending++
		}
	}
	g.printSummary(changes)
	if pending == 0 {
		fmt.Fprintln(g.out, "Everything is up to date; nothing to write.")
		return nil
	}

	switch {
	case g.opts.dryRun:
		fmt.Fprintln(g.out, "Dry run: nothing was written.")
		return &exitError{code: codeNotWritten}
	case g.opts.yes:
	case !g.console.interactive:
		// Never read a stdin that is not a terminal: a script or CI job
		// would wait forever for an answer.
		fmt.Fprintf(g.errOut, "Nothing was written: stdin is not a terminal, so cyanotype cannot ask for confirmation.\n"+
			"Run \"%s\" to write these files.\n", g.command("--yes"))
		return &exitError{code: codeNotWritten}
	case !g.confirm():
		fmt.Fprintln(g.errOut, "Nothing was written.")
		return &exitError{code: codeNotWritten}
	}

	written, err := generate.Write(root, changes)
	if err != nil {
		if len(written) > 0 {
			fmt.Fprintf(g.errOut, "Wrote only %s before failing.\n", strings.Join(written, ", "))
		}
		return err
	}
	fmt.Fprintf(g.out, "Wrote %s.\n", plural(len(written), "file"))
	return nil
}

// confirm asks whether to write and reads one line of answer. Only "y" and
// "yes" write; anything else, or the end of input, means no.
func (g *generation) confirm() bool {
	fmt.Fprint(g.errOut, "Write these files? [y/N] ")
	answer, err := bufio.NewReader(g.in).ReadString('\n')
	if err != nil && answer == "" {
		fmt.Fprintln(g.errOut)
		return false
	}
	answer = strings.TrimSpace(answer)
	return strings.EqualFold(answer, "y") || strings.EqualFold(answer, "yes")
}

// command spells out a generate command with extra flags, repeating --dir
// when the user gave it.
func (g *generation) command(flags ...string) string {
	parts := []string{"cyanotype", "generate"}
	if g.dirFlag {
		dir := g.opts.dir
		if strings.ContainsAny(dir, " \t") {
			dir = `'` + dir + `'`
		}
		parts = append(parts, "--dir", dir)
	}
	return strings.Join(append(parts, flags...), " ")
}

// printWarnings prints warnings to stderr, one per line, in the
// "file:line: warning: field: message" form editors and terminals link to
// the spec.
func (g *generation) printWarnings(warnings []resolve.Warning) {
	for _, w := range warnings {
		g.printDiagnostic(g.paint(g.console.colorErr, yellow, "warning"), w.Line, w.Field, w.Msg)
	}
}

// reportErrors prints every problem err holds, then how many there were,
// and returns an exitError that ends the command with codeError.
// Problems in the spec name their line; YAML syntax errors, whose line
// yaml.v3 may report one off, say "near line" and show the lines around it.
func (g *generation) reportErrors(root *os.Root, err error) error {
	errs := []error{err}
	var joined interface{ Unwrap() []error }
	if errors.As(err, &joined) {
		errs = joined.Unwrap()
	}
	label := g.paint(g.console.colorErr, red, "error")
	for _, e := range errs {
		var ve *spec.ValidationError
		switch {
		case !errors.As(e, &ve):
			fmt.Fprintf(g.errOut, "%s: %v\n", label, e)
		case ve.Syntax && ve.Line > 0:
			msg := fmt.Sprintf("invalid YAML near line %d: %s", ve.Line, strings.TrimPrefix(ve.Msg, "invalid YAML: "))
			g.printDiagnostic(label, 0, "", msg)
			g.printExcerpt(root, ve.Line)
		default:
			g.printDiagnostic(label, ve.Line, ve.Field, ve.Msg)
		}
	}
	fmt.Fprintf(g.errOut, "%s; nothing was written.\n", plural(len(errs), "error"))
	return &exitError{code: codeError}
}

// printDiagnostic prints one problem in the spec.
func (g *generation) printDiagnostic(label string, line int, field, msg string) {
	loc := g.specPath
	if line > 0 {
		loc += fmt.Sprintf(":%d", line)
	}
	if field != "" {
		msg = field + ": " + msg
	}
	fmt.Fprintf(g.errOut, "%s: %s: %s\n", loc, label, msg)
}

// printExcerpt prints the lines of the spec around line, numbered.
func (g *generation) printExcerpt(root *os.Root, line int) {
	data, err := root.ReadFile(generate.SpecFile)
	if err != nil {
		return // the error itself has been printed; the excerpt is a bonus
	}
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	for n := max(line-1, 1); n <= min(line+1, len(lines)); n++ {
		fmt.Fprintf(g.errOut, "  %4d | %s\n", n, lines[n-1])
	}
}

// printDiff prints a unified diff to stdout, colored by line kind.
func (g *generation) printDiff(d string) {
	for line := range strings.Lines(d) {
		color := ""
		switch {
		case strings.HasPrefix(line, "--- "), strings.HasPrefix(line, "+++ "):
			color = bold
		case strings.HasPrefix(line, "@@"):
			color = cyan
		case strings.HasPrefix(line, "+"):
			color = green
		case strings.HasPrefix(line, "-"):
			color = red
		}
		text := strings.TrimSuffix(line, "\n")
		fmt.Fprintln(g.out, g.paint(g.console.colorOut, color, text))
	}
}

// printSummary lists every generated file with its status, then counts
// them.
func (g *generation) printSummary(changes []generate.Change) {
	fmt.Fprintln(g.out)
	counts := map[generate.Status]int{}
	for _, ch := range changes {
		counts[ch.Status]++
		color := map[generate.Status]string{generate.Created: green, generate.Modified: yellow, generate.Unchanged: dim}[ch.Status]
		label := g.paint(g.console.colorOut, color, fmt.Sprintf("%-10s", ch.Status))
		note := ""
		if ch.CRLF {
			note = " (line endings change from CRLF to LF)"
		}
		fmt.Fprintf(g.out, "  %s %s%s\n", label, ch.Path, note)
	}

	toWrite := counts[generate.Created] + counts[generate.Modified]
	if toWrite == 0 {
		return
	}
	summary := fmt.Sprintf("%s to write (%d created, %d modified)",
		plural(toWrite, "file"), counts[generate.Created], counts[generate.Modified])
	if n := counts[generate.Unchanged]; n > 0 {
		summary += fmt.Sprintf(", %d unchanged", n)
	}
	fmt.Fprintln(g.out, summary+".")
}

// ANSI escape sequences for the colors the output uses.
const (
	reset  = "\x1b[0m"
	bold   = "\x1b[1m"
	dim    = "\x1b[2m"
	red    = "\x1b[31m"
	green  = "\x1b[32m"
	yellow = "\x1b[33m"
	cyan   = "\x1b[36m"
)

// paint wraps s in color when the stream shows colors.
func (g *generation) paint(enabled bool, color, s string) string {
	if !enabled || color == "" {
		return s
	}
	return color + s + reset
}

// plural formats a count with its noun: "1 file", "2 files".
func plural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
