package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	bundled "github.com/cyanotype-dev/cyanotype/catalog"
	"github.com/cyanotype-dev/cyanotype/internal/catalog"
	"github.com/cyanotype-dev/cyanotype/internal/check"
	"github.com/cyanotype-dev/cyanotype/internal/contract"
	"github.com/cyanotype-dev/cyanotype/internal/generate"
	"github.com/cyanotype-dev/cyanotype/internal/ops"
)

// checkOptions holds the flags of the check command.
type checkOptions struct {
	dir    string
	strict bool
	json   bool
}

// newCheckCmd builds the check command. detect tells it about the terminal
// when it runs, for colors only: check never reads stdin.
func newCheckCmd(detect func() console) *cobra.Command {
	var opts checkOptions
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Report drift between the project and cyanotype.yaml",
		Long: "Check compares the project with cyanotype.yaml and the files generate would\n" +
			"write, and reports what drifted: generated files edited by hand or out of\n" +
			"date, files git tracks that the generated .gitignore lines keep out, the\n" +
			"stack detected in the repository, an older catalog pin, long files and\n" +
			"pointers to missing files. It never writes and never reads stdin, so it\n" +
			"runs in a pre-commit hook and in CI.\n\n" +
			"Exit codes: 0 no errors (warnings allowed, unless --strict), 1 the check\n" +
			"could not run, 2 wrong usage, 3 drift found. With --json the result is\n" +
			"JSON on stdout and nothing else, with the same exit codes.",
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := &checking{
				opts:     opts,
				out:      cmd.OutOrStdout(),
				errOut:   cmd.ErrOrStderr(),
				console:  detect(),
				specPath: filepath.Join(opts.dir, generate.SpecFile),
			}
			if opts.json {
				out, code := c.runJSON()
				return printJSON(c.out, out, code)
			}
			return c.run()
		},
	}
	cmd.Flags().StringVar(&opts.dir, "dir", ".", "project folder holding cyanotype.yaml")
	cmd.Flags().BoolVar(&opts.strict, "strict", false, "fail on warnings too, with exit code 3")
	cmd.Flags().BoolVar(&opts.json, "json", false, jsonFlagHelp)
	return cmd
}

// checking is one run of the check command.
type checking struct {
	opts        checkOptions
	out, errOut io.Writer
	console     console
	specPath    string
}

func (c *checking) run() error {
	root, err := os.OpenRoot(c.opts.dir)
	if err != nil {
		return fmt.Errorf("open the project folder: %w", err)
	}
	defer root.Close()

	diag := specDiagnostics{errOut: c.errOut, color: c.console.colorErr, specPath: c.specPath}
	f, err := generate.LoadSpec(root)
	if errors.Is(err, generate.ErrNoSpec) {
		return fmt.Errorf("%w; run cyanotype in the folder that holds it, or pass --dir", err)
	}
	if err != nil {
		return diag.report(root, err, "nothing was checked")
	}
	cat, err := catalog.LoadRoot(bundled.FS())
	if err != nil {
		return fmt.Errorf("the bundled catalog is invalid, which is a bug in cyanotype: %w", err)
	}

	report, err := check.Run(cat, f, root.FS(), c.opts.dir, check.ExecGit{})
	if err != nil {
		return diag.report(root, err, "nothing was checked")
	}
	for _, fd := range report.Findings {
		c.printFinding(fd)
	}
	for _, s := range report.Skipped {
		fmt.Fprintf(c.out, "Skipped %s.\n", s)
	}

	errs, warns := report.Count(check.Error), report.Count(check.Warning)
	fmt.Fprintln(c.out, checkSummary(errs, warns, c.opts.strict))
	if errs > 0 || (c.opts.strict && warns > 0) {
		return &exitError{code: codeDrift}
	}
	return nil
}

// runJSON is run for --json: it returns the output instead of printing
// it, and the exit code run would end with.
func (c *checking) runJSON() (*contract.Check, int) {
	root, err := openProject(c.opts.dir)
	if err != nil {
		return &contract.Check{Diagnostics: contract.Diagnostics{Errors: contract.Errors(err)}}, codeError
	}
	defer root.Close()
	cat, err := loadBundled()
	if err != nil {
		return &contract.Check{Diagnostics: contract.Diagnostics{Errors: contract.Errors(err)}}, codeError
	}
	out := ops.Check(cat, root, c.opts.strict)
	return out, checkCode(out)
}

// checkCode is the exit code of a check output.
func checkCode(out *contract.Check) int {
	switch {
	case len(out.Errors) > 0:
		return codeError
	case out.Summary.Drift:
		return codeDrift
	}
	return codeOK
}

// printFinding prints one finding to stdout as "path[:line]: severity:
// message", with paths as the user knows them, so terminals link to them.
func (c *checking) printFinding(fd check.Finding) {
	loc := filepath.Join(c.opts.dir, filepath.FromSlash(fd.Path))
	if fd.Path == generate.SpecFile {
		loc = c.specPath
	}
	if fd.Line > 0 {
		loc += fmt.Sprintf(":%d", fd.Line)
	}
	color := red
	if fd.Severity == check.Warning {
		color = yellow
	}
	fmt.Fprintf(c.out, "%s: %s: %s\n", loc, paint(c.console.colorOut, color, fd.Severity.String()), fd.Msg)
}

// checkSummary counts the findings: "No drift found.", "1 error.", "2
// errors and 1 warning.". With warnings only, it says whether they fail
// the check.
func checkSummary(errs, warns int, strict bool) string {
	switch {
	case errs == 0 && warns == 0:
		return "No drift found."
	case errs == 0 && strict:
		return plural(warns, "warning") + "; --strict fails on warnings."
	case errs == 0:
		return plural(warns, "warning") + "; no errors."
	case warns == 0:
		return plural(errs, "error") + "."
	}
	return plural(errs, "error") + " and " + plural(warns, "warning") + "."
}
