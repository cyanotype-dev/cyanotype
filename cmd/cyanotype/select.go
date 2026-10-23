package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cyanotype-dev/cyanotype/internal/contract"
	"github.com/cyanotype-dev/cyanotype/internal/generate"
	"github.com/cyanotype-dev/cyanotype/internal/ops"
)

// selectOptions holds the flags of the select command.
type selectOptions struct {
	dir    string
	add    []string
	remove []string
	dryRun bool
	yes    bool
	json   bool
}

// newSelectCmd builds the select command, the CLI side of the MCP
// set_selections tool. detect tells it about the terminal when it runs.
func newSelectCmd(detect func() console) *cobra.Command {
	var opts selectOptions
	cmd := &cobra.Command{
		Use:   "select",
		Short: "Add or remove catalog entries in cyanotype.yaml",
		Long: "Select adds catalog entries to the selections of cyanotype.yaml, after the\n" +
			"ones it has, and removes others, keeping the file's comments. It shows the\n" +
			"change as a diff and asks before writing. A change that would leave the spec\n" +
			"with errors, such as a missing requirement, is refused, and a locked entry is\n" +
			"never removed. It changes only cyanotype.yaml: run cyanotype generate to\n" +
			"update the generated files.\n\n" +
			"Exit codes: 0 written, or nothing to change, 1 error, 2 wrong usage,\n" +
			"3 changes found but not written (--dry-run, declined, or no terminal to ask on).\n" +
			"With --json it prints the result as JSON and nothing else, never asks,\n" +
			"and writes only with --yes.",
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s := &selection{opts: opts, cmd: cmd, console: detect()}
			return s.run()
		},
	}
	cmd.Flags().StringVar(&opts.dir, "dir", ".", "project folder holding cyanotype.yaml")
	cmd.Flags().StringSliceVar(&opts.add, "add", nil, "entry ids to select, separated by commas")
	cmd.Flags().StringSliceVar(&opts.remove, "remove", nil, "entry ids to stop selecting, separated by commas")
	cmd.Flags().BoolVar(&opts.dryRun, "dry-run", false, "show the change and write nothing")
	cmd.Flags().BoolVarP(&opts.yes, "yes", "y", false, "write the change without asking")
	cmd.Flags().BoolVar(&opts.json, "json", false, jsonFlagHelp)
	return cmd
}

// selection is one run of the select command.
type selection struct {
	opts    selectOptions
	cmd     *cobra.Command
	console console
}

func (s *selection) run() error {
	out := s.cmd.OutOrStdout()
	if err := s.checkUsage(); err != nil {
		if s.opts.json {
			return printJSON(out, &contract.Select{Diagnostics: usageDiagnostics(err)}, codeUsage)
		}
		return usageError(err)
	}
	c, err := loadBundled()
	if err != nil {
		return err
	}
	root, err := openProject(s.opts.dir)
	if err != nil {
		return err
	}
	defer root.Close()

	result, change := ops.Select(c, root, s.opts.add, s.opts.remove)
	if s.opts.json {
		code := s.outcome(result, change)
		if code == codeOK && change != nil {
			result = ops.WriteSelect(root, change, result)
			code = outputCode(result.Errors)
		}
		return printJSON(out, result, code)
	}

	specPath := filepath.Join(s.opts.dir, generate.SpecFile)
	errOut := s.cmd.ErrOrStderr()
	d := specDiagnostics{errOut: errOut, color: s.console.colorErr, specPath: specPath}
	if n := d.printOutput(root, s.opts.dir, result.Diagnostics); n > 0 {
		fmt.Fprintf(errOut, "%s; nothing was written.\n", plural(n, "error"))
		return &exitError{code: codeError}
	}
	if change == nil {
		fmt.Fprintf(out, "%s already has those selections; nothing to write.\n", specPath)
		return nil
	}
	printDiff(out, s.console.colorOut, result.SpecDiff)
	fmt.Fprintln(out, "\n"+changeSummary(result))

	switch {
	case s.opts.dryRun:
		fmt.Fprintln(out, "Dry run: nothing was written.")
		return &exitError{code: codeNotWritten}
	case s.opts.yes:
	case !s.console.interactive:
		fmt.Fprintf(errOut, "Nothing was written: stdin is not a terminal, so cyanotype cannot ask for confirmation.\n"+
			"Run the same command with --yes to write %s.\n", specPath)
		return &exitError{code: codeNotWritten}
	case !confirm(s.cmd.InOrStdin(), errOut, "Write "+specPath+"?"):
		fmt.Fprintln(errOut, "Nothing was written.")
		return &exitError{code: codeNotWritten}
	}
	result = ops.WriteSelect(root, change, result)
	if n := d.printOutput(root, s.opts.dir, contract.Diagnostics{Errors: result.Errors}); n > 0 {
		return &exitError{code: codeError}
	}
	fmt.Fprintf(out, "Wrote %s. Run cyanotype generate to update the generated files.\n", specPath)
	return nil
}

// checkUsage rejects flags that ask for nothing or for two things at once.
func (s *selection) checkUsage() error {
	switch {
	case len(s.opts.add) == 0 && len(s.opts.remove) == 0:
		return errors.New("nothing to change: name entries with --add or --remove")
	case s.opts.dryRun && s.opts.yes:
		return errors.New("--dry-run and --yes cannot be used together")
	}
	return nil
}

// outcome returns the exit code of a JSON run before anything is written:
// 1 for errors, 0 when nothing changes or --yes will write, 3 otherwise.
func (s *selection) outcome(result *contract.Select, change *ops.SpecChange) int {
	switch {
	case len(result.Errors) > 0:
		if result.Errors[0].Code == contract.CodeUsage {
			return codeUsage
		}
		return codeError
	case change == nil, s.opts.yes:
		return codeOK
	}
	return codeNotWritten
}

// changeSummary says what a change of selections does: "Adds pnpm and
// node; removes npm."
func changeSummary(r *contract.Select) string {
	var parts []string
	if len(r.Added) > 0 {
		parts = append(parts, "adds "+andList(r.Added))
	}
	if len(r.Removed) > 0 {
		parts = append(parts, "removes "+andList(r.Removed))
	}
	summary := strings.Join(parts, "; ") + "."
	return strings.ToUpper(summary[:1]) + summary[1:]
}
