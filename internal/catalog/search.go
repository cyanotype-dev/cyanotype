package catalog

import (
	"slices"
	"strings"
)

// Query says which entries Search returns. Its zero value matches every
// entry.
type Query struct {
	// Terms must each appear, ignoring case, in the entry's id, name,
	// summary or category.
	Terms []string
	// Category, when set, is the only category returned.
	Category string
	// Stack, when not empty, keeps only the entries whose requirements it
	// meets, as RequirementsMet says: the entries that fit a project using
	// those ids.
	Stack []string
}

// Search returns the entries that match q, sorted by id. It serves
// cyanotype catalog search and the MCP search_catalog tool, so agents find
// entries by name, category or stack the same way in both.
func (c *Catalog) Search(q Query) []Entry {
	var out []Entry
	for _, e := range c.Entries() {
		if q.Category != "" && e.Category != q.Category {
			continue
		}
		if len(q.Stack) > 0 && !e.RequirementsMet(q.Stack) {
			continue
		}
		if matchesTerms(e, q.Terms) {
			out = append(out, e)
		}
	}
	return out
}

// matchesTerms reports whether every term appears in e's id, name, summary
// or category, ignoring case.
func matchesTerms(e Entry, terms []string) bool {
	text := strings.ToLower(strings.Join([]string{e.ID, e.Name, e.Summary, e.Category}, "\n"))
	for _, term := range terms {
		if !strings.Contains(text, strings.ToLower(term)) {
			return false
		}
	}
	return true
}

// RequirementsMet reports whether stack holds every entry e requires and
// one alternative of each of its requires_one_of groups. An entry with no
// requirements fits any stack.
func (e Entry) RequirementsMet(stack []string) bool {
	for _, id := range e.Requires {
		if !slices.Contains(stack, id) {
			return false
		}
	}
	for _, group := range e.RequiresOneOf {
		if !slices.ContainsFunc(group, func(id string) bool { return slices.Contains(stack, id) }) {
			return false
		}
	}
	return true
}
