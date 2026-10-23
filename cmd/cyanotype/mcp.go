package main

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	bundled "github.com/cyanotype-dev/cyanotype/catalog"
	"github.com/cyanotype-dev/cyanotype/internal/mcpserver"
	"github.com/cyanotype-dev/cyanotype/internal/preset"
)

// newMCPCmd builds the mcp command, which runs the MCP server for a coding
// agent. version is reported to the client.
func newMCPCmd(version string) *cobra.Command {
	var allow []string
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Run the MCP server for coding agents, on stdin and stdout",
		Long: "Mcp runs cyanotype's MCP server: a coding agent that starts it gets tools to\n" +
			"search the catalog, read and validate cyanotype.yaml, detect the stack,\n" +
			"preview and generate files, check for drift and change selections. Each tool\n" +
			"returns what the matching command prints with --json. No tool writes unless\n" +
			"the agent passes write: true.\n\n" +
			"The tools may use only the folders given with --allow, and folders inside\n" +
			"them; without --allow, only the folder the server starts in. Stdout carries\n" +
			"the protocol and nothing else; the server logs to stderr. It runs until the\n" +
			"client closes stdin.",
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runMCP(cmd, version, allow)
		},
	}
	cmd.Flags().StringArrayVar(&allow, "allow", nil, "a folder the tools may use, with the folders inside it; repeat it for more (default: the current folder)")
	return cmd
}

// runMCP serves the MCP protocol on the command's stdin and stdout until
// the client disconnects.
func runMCP(cmd *cobra.Command, version string, allow []string) error {
	start, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("find the current folder: %w", err)
	}
	folders, err := mcpserver.OpenFolders(start, allow)
	if err != nil {
		return usageError(fmt.Errorf("--allow: %w", err))
	}
	defer folders.Close()
	c, err := loadBundled()
	if err != nil {
		return err
	}
	presets, err := preset.LoadRoot(bundled.FS(), c)
	if err != nil {
		return fmt.Errorf("the bundled presets are invalid, which is a bug in cyanotype: %w", err)
	}

	log := slog.New(slog.NewTextHandler(cmd.ErrOrStderr(), nil))
	srv, err := mcpserver.New(c, presets, mcpserver.Options{Folders: folders, Version: version, Logger: log})
	if err != nil {
		return err
	}
	log.Info("cyanotype mcp started", "version", version, "allowed", folders.Names())
	transport := &mcp.IOTransport{Reader: io.NopCloser(cmd.InOrStdin()), Writer: nopWriteCloser{cmd.OutOrStdout()}}
	err = srv.Run(cmd.Context(), transport)
	log.Info("cyanotype mcp stopped")
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, mcp.ErrConnectionClosed) {
		return fmt.Errorf("mcp server: %w", err)
	}
	return nil
}

// nopWriteCloser leaves stdout open when the transport closes: the
// process owns it.
type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }
