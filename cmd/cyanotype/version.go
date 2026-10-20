package main

import (
	"fmt"
	"runtime/debug"

	"github.com/spf13/cobra"
)

// resolveVersion picks the version to report. A version set with -ldflags
// wins; otherwise it falls back to the module version Go records in the
// binary, so "go install .../cmd/cyanotype@v0.1.0" reports v0.1.0 rather
// than "dev". Builds from a checkout record "(devel)", which is no version.
func resolveVersion(ldflags string, info *debug.BuildInfo, ok bool) string {
	if ldflags != "dev" || !ok || info == nil {
		return ldflags
	}
	if v := info.Main.Version; v != "" && v != "(devel)" {
		return v
	}
	return ldflags
}

func newVersionCmd(version string) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the cyanotype version",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "cyanotype %s\n", version)
			return err
		},
	}
}
