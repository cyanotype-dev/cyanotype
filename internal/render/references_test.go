package render

import (
	"slices"
	"testing"
)

// TestReferencesFindsWhatRenderWrites renders every kind of reference the
// renderer emits, the product brief, topic files and the AGENTS.md import,
// and expects References to find each one.
func TestReferencesFindsWhatRenderWrites(t *testing.T) {
	r := resolveSpec(t, "version: 1\ntargets: [claude, agents-md]\nproject:\n  name: tiny\n  goal: A tiny project.\n  brief: docs/PRODUCT.md\n"+
		"selections: [go, plan-before-code, conventional-commits]\noutput:\n  split: always\n")
	files, _, err := Render(r, nil)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, f := range files {
		if f.Header {
			got = append(got, References(f.Content)...)
		}
	}
	slices.Sort(got)
	want := []string{"AGENTS.md", "docs/PRODUCT.md", "docs/agents/git.md", "docs/agents/style.md", "docs/agents/workflow.md"}
	if !slices.Equal(got, want) {
		t.Errorf("References() over the rendered files = %q, want %q", got, want)
	}
}

func TestReferences(t *testing.T) {
	tests := []struct {
		name, content string
		want          []string
	}{
		{
			// The pointer internal/adopt writes for a moved file.
			name: "moved file",
			content: "<!-- cyanotype:custom -->\r\n- Before starting a task, read docs/agents/original-claude.md, " +
				"which holds the rules written for this project before cyanotype.\r\n<!-- /cyanotype:custom -->\r\n",
			want: []string{"docs/agents/original-claude.md"},
		},
		{
			name:    "repeats and the import",
			content: "@AGENTS.md\n- Before adding a feature, read docs/PRODUCT.md.\n- Also read docs/PRODUCT.md.\n",
			want:    []string{"AGENTS.md", "docs/PRODUCT.md"},
		},
		{
			name:    "words that are not paths",
			content: "- Always read carefully.\n- Then read CONTRIBUTING, twice.\n@ someone\nNot a bullet, read docs/x.md.\n",
		},
		{
			name:    "paths outside the project",
			content: "- Before starting, read ../secrets.md.\n@/etc/passwd\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := References([]byte(tt.content)); !slices.Equal(got, tt.want) {
				t.Errorf("References() = %q, want %q", got, tt.want)
			}
		})
	}
}
