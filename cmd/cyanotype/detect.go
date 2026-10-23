package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/cyanotype-dev/cyanotype/internal/contract"
	"github.com/cyanotype-dev/cyanotype/internal/detect"
	"github.com/cyanotype-dev/cyanotype/internal/ops"
)

// newDetectCmd builds the detect command, the CLI side of the MCP
// detect_stack tool. detectConsole tells it about the terminal, for colors
// only.
func newDetectCmd(detectConsole func() console) *cobra.Command {
	var opts projectOptions
	cmd := &cobra.Command{
		Use:   "detect",
		Short: "Show the catalog entries the project's files suggest",
		Long: "Detect reads the manifests and files at the root of the project folder, such\n" +
			"as package.json, go.mod and lockfiles, and lists the catalog entries they\n" +
			"suggest, with what revealed each one. It needs no cyanotype.yaml and writes\n" +
			"nothing.\n\n" +
			"Exit codes: 0 done, even when nothing is detected, 1 the folder cannot be\n" +
			"read, 2 wrong usage.",
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
			if opts.json {
				return printJSON(cmd.OutOrStdout(), ops.Detect(c, root), codeOK)
			}
			con := detectConsole()
			found := detect.Detect(c, root.FS())
			d := specDiagnostics{errOut: cmd.ErrOrStderr(), color: con.colorErr}
			d.printOutput(nil, opts.dir, contract.NewDetect(c, found).Diagnostics)
			if found.Empty() {
				fmt.Fprintln(cmd.OutOrStdout(), "Nothing detected: no file at the root of the project suggests a catalog entry.")
				return nil
			}
			printDetected(cmd.OutOrStdout(), con.colorOut, opts.dir, c, found)
			return nil
		},
	}
	addProjectFlags(cmd, &opts)
	cmd.Flags().Lookup("dir").Usage = "project folder to inspect"
	return cmd
}
