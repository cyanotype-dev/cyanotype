package preset

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/cyanotype-dev/cyanotype/internal/catalog"
	"github.com/cyanotype-dev/cyanotype/internal/spec"
)

// testCatalog loads the small catalog the test presets select from.
func testCatalog(t *testing.T) *catalog.Catalog {
	t.Helper()
	c, err := catalog.LoadRoot(os.DirFS(filepath.Join("testdata", "catalog")))
	if err != nil {
		t.Fatalf("testdata/catalog is invalid:\n%v", err)
	}
	return c
}

func TestLoadRootValid(t *testing.T) {
	presets, err := LoadRoot(os.DirFS(filepath.Join("testdata", "valid")), testCatalog(t))
	if err != nil {
		t.Fatalf("LoadRoot: %v", err)
	}

	want := []Preset{
		{
			ID: "go-tool", Name: "Go tool", Summary: "A Go command-line tool", ProjectType: Script,
			Selections: []string{"go", "golangci-lint"},
			Commands:   spec.Commands{Build: "go build ./...", Test: "go test ./..."},
		},
		{
			ID: "next-site", Name: "Next.js site", Summary: "A Next.js website", ProjectType: Website,
			Selections:     []string{"typescript", "node", "pnpm", "nextjs"},
			I18nSelections: []string{"i18n-next-intl"},
			Output:         spec.Output{Split: spec.SplitAlways},
		},
	}
	if len(presets) != len(want) {
		t.Fatalf("got %d presets, want %d: %+v", len(presets), len(want), presets)
	}
	for i := range want {
		got := presets[i]
		if got.ID != want[i].ID || got.Name != want[i].Name || got.Summary != want[i].Summary ||
			got.ProjectType != want[i].ProjectType || got.Commands != want[i].Commands || got.Output != want[i].Output ||
			!slices.Equal(got.Selections, want[i].Selections) || !slices.Equal(got.I18nSelections, want[i].I18nSelections) {
			t.Errorf("preset %d = %+v, want %+v", i, got, want[i])
		}
	}
}

func TestLoadRootMissingFolder(t *testing.T) {
	presets, err := LoadRoot(os.DirFS(filepath.Join("testdata", "catalog")), testCatalog(t))
	if err != nil || presets != nil {
		t.Errorf("LoadRoot without a presets folder = %v, %v; want no presets and no error", presets, err)
	}
}

func TestLoadRootInvalid(t *testing.T) {
	tests := []struct {
		dir  string
		want []string
	}{
		{"unknown-fields", []string{
			`alpha.yaml: line 6: unknown field "targets"; a preset holds only selections, i18n_selections, commands and output, since the other spec fields describe one project`,
			`alpha.yaml: line 7: unknown field "project"; a preset holds only selections, i18n_selections, commands and output, since the other spec fields describe one project`,
		}},
		{"missing-fields", []string{
			`alpha.yaml: name: is required`,
			`alpha.yaml: summary: is required`,
			`alpha.yaml: selections: is required; list at least one catalog entry`,
		}},
		{"bad-metadata", []string{
			`alpha.yaml: name: must be a single line`,
			`alpha.yaml: id: "Beta" must be kebab-case (lowercase letters, digits and single hyphens)`,
			`alpha.yaml: project_type: must be "website", "mobile-app", "api" or "script", got "game"`,
		}},
		{"unknown-entry", []string{
			`alpha.yaml: selections[1]: unknown entry "golangci-lnt"; did you mean "golangci-lint"?`,
		}},
		{"not-resolving", []string{
			`alpha.yaml: selections[3]: "pnpm" and "npm" (selections[2]) are both in radio category "package-manager"; keep only one`,
			`alpha.yaml: selections[0]: "golangci-lint" requires "go", which is not selected`,
		}},
		{"i18n-selections", []string{
			`alpha.yaml: i18n_selections[0]: "i18n-next-intl" requires "nextjs" and "node" (through "nextjs"), which are not selected`,
			`alpha.yaml: i18n_selections[0]: "i18n-next-intl" requires one of "npm" or "pnpm" (through "node"), but none is selected`,
		}},
		{"i18n-duplicate", []string{
			`alpha.yaml: i18n_selections[0]: "i18n-rules" is listed twice`,
		}},
		{"spec-fields", []string{
			`alpha.yaml: commands.test: must be a single line`,
			`alpha.yaml: selections[1]: "go" is listed twice`,
			`alpha.yaml: output.split: must be "auto", "always" or "never", got "sometimes"`,
		}},
		{"file-layout", []string{
			`nested: subfolders are not allowed; presets live in one flat folder`,
			`notes.md: only .yaml files are allowed`,
		}},
		{"documents", []string{
			`alpha.yaml: a file must hold exactly one YAML document`,
			`empty.yaml: file is empty`,
		}},
	}
	c := testCatalog(t)
	for _, tt := range tests {
		t.Run(tt.dir, func(t *testing.T) {
			_, err := LoadRoot(os.DirFS(filepath.Join("testdata", "invalid", tt.dir)), c)
			if err == nil {
				t.Fatal("LoadRoot succeeded, want errors")
			}
			got := messages(t, err)
			if !slices.Equal(got, tt.want) {
				t.Errorf("errors:\n%s\nwant:\n%s", lines(got), lines(tt.want))
			}
		})
	}
}

// messages returns the text of every problem in err, checking that each
// one is a *ValidationError.
func messages(t *testing.T, err error) []string {
	t.Helper()
	errs := []error{err}
	var joined interface{ Unwrap() []error }
	if errors.As(err, &joined) {
		errs = joined.Unwrap()
	}
	var out []string
	for _, e := range errs {
		var ve *ValidationError
		if !errors.As(e, &ve) {
			t.Errorf("%v is a %T, want *ValidationError", e, e)
		}
		out = append(out, e.Error())
	}
	return out
}

func lines(s []string) string {
	out := ""
	for _, l := range s {
		out += "  " + l + "\n"
	}
	return out
}
