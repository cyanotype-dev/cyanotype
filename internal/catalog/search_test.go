package catalog

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestSearch(t *testing.T) {
	c, err := LoadRoot(os.DirFS(filepath.Join("..", "..", "catalog")))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		q    Query
		want []string
	}{
		{"a term in the id", Query{Terms: []string{"pnpm"}}, []string{"pnpm"}},
		{"terms ignore case and all must match", Query{Terms: []string{"PACKAGE", "node"}}, []string{"npm", "pnpm"}},
		{"a term in the name", Query{Terms: []string{"Conventional"}}, []string{"conventional-commits"}},
		{"a category", Query{Category: "database"}, []string{"postgres", "sqlite", "supabase"}},
		{"a category and a term", Query{Category: "lint", Terms: []string{"go"}}, []string{"golangci-lint"}},
		{"a stack keeps what fits it", Query{Category: "testing", Stack: []string{"python"}}, []string{"pytest"}},
		{"requires_one_of needs one alternative", Query{Category: "ui-components", Stack: []string{"tailwind", "vite-react"}}, []string{"shadcn-ui"}},
		{"an unmet requires_one_of", Query{Category: "ui-components", Stack: []string{"tailwind"}}, nil},
		{"nothing matches", Query{Terms: []string{"kubernetes"}}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for _, e := range c.Search(tt.q) {
				got = append(got, e.ID)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("Search(%+v) = %q, want %q", tt.q, got, tt.want)
			}
		})
	}
	if got, all := len(c.Search(Query{})), len(c.Entries()); got != all {
		t.Errorf("an empty query returned %d entries, want all %d", got, all)
	}
}

func TestRequirementsMet(t *testing.T) {
	e := Entry{Requires: []string{"tailwind"}, RequiresOneOf: [][]string{{"nextjs", "vite-react"}}}
	tests := []struct {
		stack []string
		want  bool
	}{
		{[]string{"tailwind", "nextjs"}, true},
		{[]string{"tailwind", "vite-react"}, true},
		{[]string{"tailwind"}, false},
		{[]string{"nextjs"}, false},
		{nil, false},
	}
	for _, tt := range tests {
		if got := e.RequirementsMet(tt.stack); got != tt.want {
			t.Errorf("RequirementsMet(%q) = %v, want %v", tt.stack, got, tt.want)
		}
	}
	if !(Entry{}).RequirementsMet(nil) {
		t.Error("an entry without requirements does not fit an empty stack")
	}
}
