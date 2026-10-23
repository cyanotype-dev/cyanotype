package contract

import "github.com/cyanotype-dev/cyanotype/internal/catalog"

// Catalog is the output of cyanotype catalog search --json and of the MCP
// search_catalog tool: the catalog entries that match a query.
type Catalog struct {
	Head
	// CatalogVersion is the version of the catalog searched, the bundled
	// one, such as "2026.10.1".
	CatalogVersion string `json:"catalog_version,omitzero"`
	// Entries match the query, sorted by id; empty when none does.
	Entries []Entry `json:"entries,omitzero"`
	Diagnostics
}

// Entry is one catalog entry as agents need it to choose: what it is, what
// it requires, and the rule it adds. Its real constraints and detection
// rules are left out.
type Entry struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Category string `json:"category"`
	// Select is "radio", at most one entry of the category, or "checkbox".
	Select  string `json:"select"`
	Summary string `json:"summary"`
	URL     string `json:"url,omitempty"`
	Topic   string `json:"topic,omitempty"`
	Level   string `json:"level,omitempty"`
	// Requires, RequiresOneOf and Conflicts hold entry ids, as in the
	// catalog.
	Requires      []string   `json:"requires,omitempty"`
	RequiresOneOf [][]string `json:"requires_one_of,omitempty"`
	Conflicts     []string   `json:"conflicts,omitempty"`
	// Rule and Why are the text the entry adds to generated files, with
	// placeholders such as {languages} unfilled.
	Rule string `json:"rule"`
	Why  string `json:"why"`
}

// NewCatalog returns the output for the entries of a search in the catalog
// of the given version.
func NewCatalog(version string, entries []catalog.Entry) *Catalog {
	out := &Catalog{CatalogVersion: version, Entries: make([]Entry, 0, len(entries))}
	for _, e := range entries {
		out.Entries = append(out.Entries, Entry{
			ID: e.ID, Name: e.Name, Category: e.Category, Select: string(e.Select), Summary: e.Summary,
			URL: e.URL, Topic: e.Topic, Level: string(e.Level),
			Requires: e.Requires, RequiresOneOf: e.RequiresOneOf, Conflicts: e.Conflicts,
			Rule: e.Snippet.Rule, Why: e.Snippet.Why,
		})
	}
	return out
}
