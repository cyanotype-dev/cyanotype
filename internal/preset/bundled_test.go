package preset_test

import (
	"slices"
	"testing"

	bundled "github.com/cyanotype-dev/cyanotype/catalog"
	"github.com/cyanotype-dev/cyanotype/internal/catalog"
	"github.com/cyanotype-dev/cyanotype/internal/preset"
	"github.com/cyanotype-dev/cyanotype/internal/render"
	"github.com/cyanotype-dev/cyanotype/internal/resolve"
	"github.com/cyanotype-dev/cyanotype/internal/spec"
)

// loadBundled loads the catalog and presets embedded in the binary.
func loadBundled(t *testing.T) (*catalog.Catalog, []preset.Preset) {
	t.Helper()
	c, err := catalog.LoadRoot(bundled.FS())
	if err != nil {
		t.Fatalf("the bundled catalog is invalid:\n%v", err)
	}
	presets, err := preset.LoadRoot(bundled.FS(), c)
	if err != nil {
		t.Fatalf("the bundled presets are invalid:\n%v", err)
	}
	return c, presets
}

// TestBundledPresets checks that the catalog ships the starter presets of
// docs/DESIGN.md, and at least one preset for every project type simple
// mode offers.
func TestBundledPresets(t *testing.T) {
	_, presets := loadBundled(t)

	var ids []string
	types := map[preset.ProjectType]bool{}
	for _, p := range presets {
		ids = append(ids, p.ID)
		types[p.ProjectType] = true
	}
	want := []string{"expo-app", "fastapi-postgres", "go-api", "go-cli", "nextjs-supabase", "python-script", "vite-react-app"}
	if !slices.Equal(ids, want) {
		t.Errorf("bundled presets = %q, want %q", ids, want)
	}
	for _, pt := range preset.ProjectTypes() {
		if !types[pt] {
			t.Errorf("no bundled preset for project type %q", pt)
		}
	}
}

// TestBundledPresetsGenerate resolves every bundled preset against the real
// catalog and renders it, on its own and as a multilingual project, for
// every target that has a renderer. Neither step may report an error or a
// warning: a starter preset must give a clean first run.
func TestBundledPresetsGenerate(t *testing.T) {
	c, presets := loadBundled(t)
	targets := []spec.Target{spec.TargetClaude, spec.TargetAgentsMD}

	for _, p := range presets {
		if len(p.Languages(c)) == 0 {
			t.Errorf("%s: selects no language, so learning mode could not name one", p.ID)
		}

		multilingual := p.Spec(spec.Project{Name: p.ID, Goal: p.Summary}, targets)
		multilingual.Selections = append(multilingual.Selections, p.MultilingualSelections()...)
		multilingual.I18n = &spec.I18n{DefaultLocale: "en", Locales: []string{"en", "it"}}

		variants := map[string]spec.Spec{
			"plain":        p.Spec(spec.Project{Name: p.ID, Goal: p.Summary}, targets),
			"multilingual": multilingual,
		}
		for name, s := range variants {
			t.Run(p.ID+"/"+name, func(t *testing.T) {
				s.CatalogVersion = c.Version()
				res, warnings, err := resolve.Resolve(c, spec.New(s))
				if err != nil {
					t.Fatalf("resolve: %v", err)
				}
				if len(warnings) > 0 {
					t.Errorf("resolve warnings: %v", warnings)
				}
				files, warnings, err := render.Render(res, nil)
				if err != nil {
					t.Fatalf("render: %v", err)
				}
				if len(warnings) > 0 {
					t.Errorf("render warnings: %v", warnings)
				}
				if len(files) == 0 {
					t.Error("render returned no files")
				}
			})
		}
	}
}
