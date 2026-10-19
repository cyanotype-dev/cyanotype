package main

import (
	"github.com/spf13/cobra"
)

// newRootCmd builds the command tree. The version is passed in rather than
// read from the package variable so tests can inject their own.
func newRootCmd(version string) *cobra.Command {
	return newRootCmdWithConsole(version, detectConsole)
}

// newRootCmdWithConsole builds the command tree with detect telling commands
// about the terminal, so tests can pretend to be interactive or not.
func newRootCmdWithConsole(version string, detect func() console) *cobra.Command {
	root := &cobra.Command{
		Use:   "cyanotype",
		Short: "Generate instruction files for coding agents",
		Long: "Cyanotype generates instruction files for coding agents (CLAUDE.md, AGENTS.md,\n" +
			"Cursor and Copilot rules) from a catalog of choices, and backs critical rules\n" +
			"with real constraints.",
		SilenceUsage: true,
		// main prints errors, so it can skip those a command already showed
		// and pick the exit code.
		SilenceErrors: true,
		// Shell completion is not part of Phase 0.
		CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
	}
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return usageError(err)
	})

	root.AddCommand(newVersionCmd(version))
	root.AddCommand(newInitCmd(detect))
	root.AddCommand(newGenerateCmd(detect))

	return root
}

// noArgs rejects positional arguments as a usage error.
func noArgs(cmd *cobra.Command, args []string) error {
	if err := cobra.NoArgs(cmd, args); err != nil {
		return usageError(err)
	}
	return nil
}
