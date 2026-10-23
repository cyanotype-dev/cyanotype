package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	bundled "github.com/cyanotype-dev/cyanotype/catalog"
	"github.com/cyanotype-dev/cyanotype/internal/catalog"
	"github.com/cyanotype-dev/cyanotype/internal/contract"
	"github.com/cyanotype-dev/cyanotype/internal/ops"
)

// newCatalogCmd builds the catalog command, which groups commands about
// the bundled catalog. Alone it shows its help.
func newCatalogCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "catalog",
		Short: "Find entries in the bundled catalog",
		Args:  unknownCommand,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newCatalogSearchCmd())
	return cmd
}

// catalogSearchOptions holds the flags of catalog search.
type catalogSearchOptions struct {
	category string
	stack    []string
	json     bool
}

// newCatalogSearchCmd builds catalog search, the CLI side of the MCP
// search_catalog tool.
func newCatalogSearchCmd() *cobra.Command {
	var opts catalogSearchOptions
	cmd := &cobra.Command{
		Use:   "search [terms...]",
		Short: "Find catalog entries by name, category or stack",
		Long: "Search lists the bundled catalog's entries whose id, name, summary or\n" +
			"category holds every term, ignoring case. --category keeps one category,\n" +
			"and --stack keeps the entries whose requirements those ids meet, the ones\n" +
			"that fit a project using them. Without terms or flags it lists every entry.\n\n" +
			"Exit codes: 0 done, even when nothing matches, 2 wrong usage.",
		RunE: func(cmd *cobra.Command, terms []string) error {
			c, err := loadBundled()
			if err != nil {
				return err
			}
			out := ops.Search(c, catalog.Query{Terms: terms, Category: opts.category, Stack: opts.stack})
			if opts.json {
				return printJSON(cmd.OutOrStdout(), out, codeOK)
			}
			printEntries(cmd.OutOrStdout(), out.Entries)
			return nil
		},
	}
	cmd.Flags().StringVar(&opts.category, "category", "", "only entries of this category, such as package-manager")
	cmd.Flags().StringSliceVar(&opts.stack, "stack", nil, "only entries whose requirements these ids meet, separated by commas")
	cmd.Flags().BoolVar(&opts.json, "json", false, jsonFlagHelp)
	return cmd
}

// printEntries lists entries one per line, "id: Name (category, select).
// Summary. Requires ...", then counts them.
func printEntries(w io.Writer, entries []contract.Entry) {
	for _, e := range entries {
		fmt.Fprintf(w, "%s: %s (%s, %s). %s.%s\n", e.ID, e.Name, e.Category, e.Select, strings.TrimSuffix(e.Summary, "."), requiresNote(e))
	}
	switch len(entries) {
	case 0:
		fmt.Fprintln(w, "No entries match.")
	case 1:
		fmt.Fprintln(w, "1 entry.")
	default:
		fmt.Fprintf(w, "%d entries.\n", len(entries))
	}
}

// requiresNote says what an entry requires: " Requires node.", or "".
func requiresNote(e contract.Entry) string {
	var parts []string
	if len(e.Requires) > 0 {
		parts = append(parts, andList(e.Requires))
	}
	for _, group := range e.RequiresOneOf {
		parts = append(parts, "one of "+orList(group, "or"))
	}
	if len(parts) == 0 {
		return ""
	}
	return " Requires " + strings.Join(parts, ", and ") + "."
}

// loadBundled loads the catalog embedded in the binary.
func loadBundled() (*catalog.Catalog, error) {
	c, err := catalog.LoadRoot(bundled.FS())
	if err != nil {
		return nil, fmt.Errorf("the bundled catalog is invalid, which is a bug in cyanotype: %w", err)
	}
	return c, nil
}

// openProject opens the project folder dir as a root that every read and
// write stays inside.
func openProject(dir string) (*os.Root, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("open the project folder: %w", err)
	}
	return root, nil
}
