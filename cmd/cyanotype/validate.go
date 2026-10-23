package main

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/cyanotype-dev/cyanotype/internal/generate"
	"github.com/cyanotype-dev/cyanotype/internal/ops"
)

// newValidateCmd builds the validate command, the CLI side of the MCP
// validate tool. detect tells it about the terminal, for colors only.
func newValidateCmd(detect func() console) *cobra.Command {
	var opts projectOptions
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Check cyanotype.yaml against the catalog and count generated lines",
		Long: "Validate resolves cyanotype.yaml against the bundled catalog, as generate\n" +
			"does, and reports conflicts, missing requirements and other problems with\n" +
			"their field and line, then how many lines each generated Markdown file\n" +
			"would have. It compares nothing with the project and writes nothing; use\n" +
			"cyanotype check for drift.\n\n" +
			"Exit codes: 0 valid (warnings allowed), 1 not valid, or no spec to read,\n" +
			"2 wrong usage.",
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, err := loadBundled()
			if err != nil {
				return err
			}
			root, err := openProject(opts.dir)
			if err != nil {
				return err
			}
			defer root.Close()
			out := ops.Validate(c, root)
			if opts.json {
				return printJSON(cmd.OutOrStdout(), out, outputCode(out.Errors))
			}
			specPath := filepath.Join(opts.dir, generate.SpecFile)
			d := specDiagnostics{errOut: cmd.ErrOrStderr(), color: detect().colorErr, specPath: specPath}
			if n := d.printOutput(root, opts.dir, out.Diagnostics); n > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "%s; %s is not valid.\n", plural(n, "error"), specPath)
				return &exitError{code: codeError}
			}
			w := cmd.OutOrStdout()
			for _, f := range out.Files {
				fmt.Fprintf(w, "  %s: %s\n", f.Path, plural(f.Lines, "line"))
			}
			fmt.Fprintf(w, "%s is valid; output.max_lines is %d.\n", specPath, out.MaxLines)
			return nil
		},
	}
	addProjectFlags(cmd, &opts)
	return cmd
}
