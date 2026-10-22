package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	bundled "github.com/cyanotype-dev/cyanotype/catalog"
	"github.com/cyanotype-dev/cyanotype/internal/adopt"
	"github.com/cyanotype-dev/cyanotype/internal/catalog"
	"github.com/cyanotype-dev/cyanotype/internal/detect"
	"github.com/cyanotype-dev/cyanotype/internal/generate"
	"github.com/cyanotype-dev/cyanotype/internal/resolve"
	"github.com/cyanotype-dev/cyanotype/internal/spec"
)

// generateOptions holds the flags of the generate command.
type generateOptions struct {
	dir      string
	dryRun   bool
	yes      bool
	existing string
}

// newGenerateCmd builds the generate command. detect tells it about the
// terminal when it runs.
func newGenerateCmd(detect func() console) *cobra.Command {
	var opts generateOptions
	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate instruction files and constraints from cyanotype.yaml",
		Long: "Generate reads cyanotype.yaml, resolves it against the bundled catalog and\n" +
			"shows the files it would write as a diff. It asks before writing. The only\n" +
			"files it deletes are files it generated earlier, no longer generates, and\n" +
			"nobody changed since, as cyanotype.lock records them. An instruction file\n" +
			"written by hand, without the cyanotype header, is never replaced silently:\n" +
			"generate asks what to do with it, or takes the answer from --existing.\n\n" +
			"Exit codes: 0 written or already up to date, 1 error, 2 wrong usage,\n" +
			"3 changes found but not written (--dry-run, declined, or no terminal to ask on).",
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if opts.dryRun && opts.yes {
				return usageError(errors.New("--dry-run and --yes cannot be used together"))
			}
			if err := checkExistingFlag(opts.existing, cmd.Flags().Changed("existing")); err != nil {
				return usageError(err)
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
	cmd.Flags().StringVar(&opts.existing, "existing", "", existingFlagHelp)
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
	// choices are what to do with hand-written files, when init has
	// already asked; nil means generate decides.
	choices generate.Choices
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
	topicDir := f.Spec.WithDefaults().Output.TopicDir
	if hw := generate.HandWritten(changes); len(hw) > 0 {
		choices, err := g.chooseExisting(root, hw, topicDir)
		if err != nil {
			return err
		}
		if imports := importPaths(choices); len(imports) > 0 {
			return g.printImport(root, c, f, imports)
		}
		if choices != nil {
			changes, _, err = generate.PreviewChoices(c, f, root.FS(), choices)
			if err != nil {
				return g.reportErrors(root, err)
			}
		}
	}

	pending, waiting := 0, generate.HandWritten(changes)
	for _, ch := range changes {
		// A hand-written file still waiting for a choice gets no diff:
		// nothing will replace it.
		if ch.Status != generate.Unchanged && (!ch.HandWritten || ch.Choice != "") {
			g.printDiff(ch.Diff())
			pending++
		}
	}
	g.printSummary(changes, topicDir)
	if pending == 0 && len(waiting) == 0 {
		fmt.Fprintln(g.out, "Everything is up to date; nothing to write.")
		return nil
	}

	switch {
	case g.opts.dryRun:
		fmt.Fprintln(g.out, "Dry run: nothing was written.")
		for _, ch := range changes {
			if ch.HandWritten && ch.Choice == "" {
				fmt.Fprintf(g.out, "%s was written by hand: choose what to do with it with --existing import, keep, move or replace.\n", ch.Path)
			}
		}
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
	fmt.Fprintln(g.out, wroteSummary(changes, len(written)))
	return nil
}

// wroteSummary says what a successful Write did with the n paths it
// returned: "Wrote 3 files.", "Wrote 2 files and deleted 1." or "Deleted
// 1 file.".
func wroteSummary(changes []generate.Change, n int) string {
	deleted := 0
	for _, ch := range changes {
		if ch.Status == generate.Deleted {
			deleted++
		}
	}
	switch deleted {
	case 0:
		return fmt.Sprintf("Wrote %s.", plural(n, "file"))
	case n:
		return fmt.Sprintf("Deleted %s.", plural(n, "file"))
	}
	return fmt.Sprintf("Wrote %s and deleted %d.", plural(n-deleted, "file"), deleted)
}

// chooseExisting decides what to do with the hand-written files at paths:
// the choices init already made, --existing, or the user's answers. A dry
// run without --existing asks nothing and returns nil, leaving the files
// waiting for a choice, so it can still show everything else.
func (g *generation) chooseExisting(root *os.Root, paths []string, topicDir string) (generate.Choices, error) {
	// Choices init made apply only to files this run would replace: a
	// hand-written CLAUDE.md that is not a target stays as it is.
	chosen := generate.Choices{}
	var open []string
	for _, p := range paths {
		if c, ok := g.choices[p]; ok {
			chosen[p] = c
		} else {
			open = append(open, p)
		}
	}
	if len(open) == 0 {
		return chosen, nil
	}
	if g.opts.dryRun && g.opts.existing == "" {
		return nil, nil
	}
	x := existingChooser{flag: g.opts.existing, dir: root.Name(), topicDir: topicDir, errOut: g.errOut}
	if g.console.interactive && !g.opts.yes && !g.opts.dryRun {
		in := bufio.NewReader(g.in)
		g.in = in // confirm reads the same buffer later
		x.ask = newPrompter(in, g.errOut)
	}
	choices, err := x.choose(open)
	if err != nil {
		var ee *exitError
		if !errors.As(err, &ee) {
			fmt.Fprintf(g.errOut, "%s: %v\nNothing was written.\n", paint(g.console.colorErr, red, "error"), err)
			return nil, &exitError{code: codeError}
		}
		return nil, err
	}
	maps.Copy(chosen, choices)
	return chosen, nil
}

// printImport prints the prompt that converts the files at paths into
// the project's cyanotype.yaml, and writes nothing.
func (g *generation) printImport(root *os.Root, c *catalog.Catalog, f *spec.File, paths []string) error {
	current, err := root.ReadFile(generate.SpecFile)
	if err != nil {
		return fmt.Errorf("read %s: %w", generate.SpecFile, err)
	}
	found := detect.Detect(c, root.FS())
	stack := slices.Clone(f.Spec.Selections)
	for _, id := range found.IDs() {
		if !slices.Contains(stack, id) {
			stack = append(stack, id)
		}
	}
	prompt := adopt.Prompt(c, adopt.PromptInput{Files: paths, Spec: current, Found: found, Stack: stack})
	printImportPrompt(g.out, g.errOut, prompt, paths, g.command("--existing", "import"), g.command("--existing", "replace"))
	return nil
}

// importPaths returns the paths chosen for import, sorted.
func importPaths(choices generate.Choices) []string {
	var paths []string
	for p, c := range choices {
		if c == adopt.Import {
			paths = append(paths, p)
		}
	}
	slices.Sort(paths)
	return paths
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
	var args []string
	if g.dirFlag {
		args = append(args, "--dir", g.opts.dir)
	}
	return commandLine("generate", append(args, flags...)...)
}

// commandLine spells out a cyanotype command for the user to run, quoting
// the arguments that hold blanks.
func commandLine(sub string, args ...string) string {
	parts := []string{"cyanotype", sub}
	for _, a := range args {
		if strings.ContainsAny(a, " \t") {
			a = `'` + a + `'`
		}
		parts = append(parts, a)
	}
	return strings.Join(parts, " ")
}

// printWarnings prints warnings to stderr, one per line, in the
// "file:line: warning: field: message" form editors and terminals link to
// the spec.
func (g *generation) printWarnings(warnings []resolve.Warning) {
	for _, w := range warnings {
		g.printDiagnostic(paint(g.console.colorErr, yellow, "warning"), w.Line, w.Field, w.Msg)
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
	label := paint(g.console.colorErr, red, "error")
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
		fmt.Fprintln(g.out, paint(g.console.colorOut, color, text))
	}
}

// printSummary lists every generated file with its status, then counts
// them.
func (g *generation) printSummary(changes []generate.Change, topicDir string) {
	fmt.Fprintln(g.out)
	counts := map[generate.Status]int{}
	for _, ch := range changes {
		counts[ch.Status]++
		color := map[generate.Status]string{generate.Created: green, generate.Modified: yellow, generate.Deleted: red, generate.Unchanged: dim}[ch.Status]
		label := paint(g.console.colorOut, color, fmt.Sprintf("%-10s", ch.Status))
		note := choiceNote(ch, topicDir)
		if ch.CRLF && !ch.HandWritten {
			note = " (line endings change from CRLF to LF)"
		}
		fmt.Fprintf(g.out, "  %s %s%s\n", label, ch.Path, note)
	}

	toWrite := counts[generate.Created] + counts[generate.Modified] + counts[generate.Deleted]
	if toWrite == 0 {
		return
	}
	details := fmt.Sprintf("%d created, %d modified", counts[generate.Created], counts[generate.Modified])
	if n := counts[generate.Deleted]; n > 0 {
		details += fmt.Sprintf(", %d deleted", n)
	}
	summary := fmt.Sprintf("%s to write (%s)", plural(toWrite, "file"), details)
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
func paint(enabled bool, color, s string) string {
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
