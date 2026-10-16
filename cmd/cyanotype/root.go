package main

import (
	"github.com/spf13/cobra"
)

// newRootCmd builds the command tree. The version is passed in rather than
// read from the package variable so tests can inject their own.
func newRootCmd(version string) *cobra.Command {
	root := &cobra.Command{
		Use:   "cyanotype",
		Short: "Generate instruction files for coding agents",
		Long: "Cyanotype generates instruction files for coding agents (CLAUDE.md, AGENTS.md,\n" +
			"Cursor and Copilot rules) from a catalog of choices, and backs critical rules\n" +
			"with real constraints.",
		SilenceUsage: true,
		// Shell completion is not part of Phase 0.
		CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
	}

	root.AddCommand(newVersionCmd(version))

	return root
}
