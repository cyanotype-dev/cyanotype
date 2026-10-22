package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

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
		// Without Args of its own, cobra reports an unknown subcommand as a
		// plain error while it looks the command up, which would exit 1.
		// With Args set it calls unknownCommand instead, but only for a
		// runnable command, hence RunE, which shows the help as before.
		Args: unknownCommand,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
		// Cobra applies this default only on its own lookup path, which
		// unknownCommand replaces, so it is set here.
		SuggestionsMinimumDistance: 2,
	}
	root.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		// In "cyanotype genrate --dir x" the root parses the flags of a
		// subcommand it does not have; the mistyped command is the real
		// mistake, so report it rather than the flag.
		if !cmd.HasParent() && cmd.Flags().NArg() > 0 {
			return unknownCommand(cmd, cmd.Flags().Args())
		}
		return usageError(err)
	})

	root.AddCommand(newVersionCmd(version))
	root.AddCommand(newInitCmd(detect))
	root.AddCommand(newGenerateCmd(detect))
	root.AddCommand(newCheckCmd(detect))

	return root
}

// unknownCommand rejects any argument to the root command as a usage
// error: the root has only subcommands, so an argument is a mistyped or
// missing one. It suggests close subcommand names, as cobra would.
func unknownCommand(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return nil
	}
	msg := fmt.Sprintf("unknown command %q for %q", args[0], cmd.CommandPath())
	if suggestions := cmd.SuggestionsFor(args[0]); len(suggestions) > 0 {
		msg += "; did you mean " + orList(quoteAll(suggestions), "or") + "?"
	} else {
		msg += fmt.Sprintf("; run %q to list the commands", cmd.CommandPath()+" --help")
	}
	return usageError(errors.New(msg))
}

// noArgs rejects positional arguments as a usage error. It builds its own
// message rather than using cobra.NoArgs, whose "unknown command" text
// suggests a subcommand where the mistake is an extra argument.
func noArgs(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return nil
	}
	return usageError(fmt.Errorf("%q takes no arguments, got %s", cmd.CommandPath(), strings.Join(quoteAll(args), ", ")))
}

// quoteAll returns each string Go-quoted, for messages: ["a", "b"].
func quoteAll(items []string) []string {
	out := make([]string, len(items))
	for i, s := range items {
		out[i] = strconv.Quote(s)
	}
	return out
}
