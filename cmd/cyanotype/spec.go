package main

import (
	"bytes"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/cyanotype-dev/cyanotype/internal/contract"
	"github.com/cyanotype-dev/cyanotype/internal/generate"
	"github.com/cyanotype-dev/cyanotype/internal/ops"
	"github.com/cyanotype-dev/cyanotype/internal/spec"
)

// projectOptions holds the flags of commands that only read a project.
type projectOptions struct {
	dir  string
	json bool
}

// addProjectFlags adds --dir and --json to cmd.
func addProjectFlags(cmd *cobra.Command, opts *projectOptions) {
	cmd.Flags().StringVar(&opts.dir, "dir", ".", "project folder holding cyanotype.yaml")
	cmd.Flags().BoolVar(&opts.json, "json", false, jsonFlagHelp)
}

// newSpecCmd builds the spec command, the CLI side of the MCP get_spec
// tool. detect tells it about the terminal, for colors only.
func newSpecCmd(detect func() console) *cobra.Command {
	var opts projectOptions
	cmd := &cobra.Command{
		Use:   "spec",
		Short: "Show cyanotype.yaml with its defaults applied",
		Long: "Spec prints the project's cyanotype.yaml with every default applied, as\n" +
			"generate reads it. cyanotype.local.yaml is not supported yet, so this is the\n" +
			"team spec only. With --json it prints the spec as written and with defaults.\n\n" +
			"Exit codes: 0 done, 1 a problem in the spec, 2 wrong usage.",
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			root, err := openProject(opts.dir)
			if err != nil {
				return err
			}
			defer root.Close()
			out := ops.Spec(root)
			if opts.json {
				return printJSON(cmd.OutOrStdout(), out, outputCode(out.Errors))
			}
			d := specDiagnostics{errOut: cmd.ErrOrStderr(), color: detect().colorErr, specPath: filepath.Join(opts.dir, generate.SpecFile)}
			if n := d.printOutput(root, opts.dir, out.Diagnostics); n > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "%s.\n", plural(n, "error"))
				return &exitError{code: codeError}
			}
			var buf bytes.Buffer
			if err := spec.New(*out.Effective).Save(&buf); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "# %s with its defaults applied; %s.\n", generate.SpecFile, contract.TeamOnlyNote)
			_, err = cmd.OutOrStdout().Write(buf.Bytes())
			return err
		},
	}
	addProjectFlags(cmd, &opts)
	return cmd
}

// outputCode returns the exit code of an output with these errors: 1 when
// there is any, 0 otherwise.
func outputCode(errs []contract.Diagnostic) int {
	if len(errs) > 0 {
		return codeError
	}
	return codeOK
}
