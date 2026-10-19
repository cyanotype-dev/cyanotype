package spec

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// designExample returns the first YAML block of the Project spec section of
// docs/DESIGN.md. Reading it from the doc itself, instead of keeping a copy
// in testdata, keeps the documented example valid.
func designExample(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "docs", "DESIGN.md"))
	if err != nil {
		t.Fatal(err)
	}
	doc := strings.ReplaceAll(string(data), "\r\n", "\n")

	_, section, ok := strings.Cut(doc, "\n## Project spec\n")
	if !ok {
		t.Fatal(`docs/DESIGN.md has no "## Project spec" section`)
	}
	_, block, ok := strings.Cut(section, "```yaml\n")
	if !ok {
		t.Fatal("the Project spec section of docs/DESIGN.md has no yaml block")
	}
	block, _, ok = strings.Cut(block, "```\n")
	if !ok {
		t.Fatal("the yaml block in the Project spec section of docs/DESIGN.md is not closed")
	}
	return block
}

// TestDesignExample loads the spec example from docs/DESIGN.md, checks every
// field, and checks that the example is in canonical layout.
func TestDesignExample(t *testing.T) {
	example := designExample(t)
	f, err := Load(strings.NewReader(example))
	if err != nil {
		t.Fatalf("the DESIGN.md example does not load:\n%v", err)
	}

	want := Spec{
		Version:        1,
		CatalogVersion: "2026.10",
		Targets:        []Target{TargetClaude, TargetAgentsMD, TargetCursor},
		Mode:           ModeAdvanced,
		Project: Project{
			Name:  "recipe-box",
			Goal:  "Web app where families save, organize and share recipes.",
			Users: "Non-technical family members, mostly on mobile.",
			CoreFeatures: []string{
				"Save recipes with photos and ingredients",
				"Share a recipe with family or publicly",
			},
			OutOfScope: []string{"Meal planning and shopping lists (later)"},
			Brief:      "docs/PRODUCT.md",
		},
		Languages: Languages{
			Code: "english", Comments: "english", Docs: "english",
			Commits: "english", AgentReplies: "italian",
		},
		Selections: []string{
			"typescript", "nextjs", "node", "pnpm", "postgres", "vitest", "i18n-next-intl",
			"conventional-commits", "no-test-tampering", "no-destructive-commands",
		},
		Locked: []string{"no-test-tampering", "no-destructive-commands"},
		I18n: &I18n{
			DefaultLocale:  "en",
			Locales:        []string{"en", "it", "es"},
			TranslationsBy: TranslationsAgentDraft,
		},
		Answers: map[string]string{"domain_notes": "Private recipes never appear in search results."},
		CustomRules: []CustomRule{{
			Rule: "Store all quantities in metric units; convert only for display.",
			Why:  "Keeps scaling and conversion logic in one place.",
		}},
		Output: Output{Split: SplitAuto, MaxLines: 100},
	}
	if !reflect.DeepEqual(f.Spec, want) {
		t.Errorf("Spec =\n%+v\nwant\n%+v", f.Spec, want)
	}

	if got := save(t, example, nil); got != example {
		t.Errorf("the DESIGN.md example is not in canonical layout; Save() writes:\n%s", got)
	}
}
