package resolve

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/cyanotype-dev/cyanotype/internal/catalog"
	"github.com/cyanotype-dev/cyanotype/internal/spec"
)

// loadCatalog loads a catalog folder under testdata, manifest included.
func loadCatalog(t *testing.T, name string) *catalog.Catalog {
	t.Helper()
	c, err := catalog.LoadRoot(os.DirFS(filepath.Join("testdata", name)))
	if err != nil {
		t.Fatalf("testdata/%s is not a valid catalog:\n%v", name, err)
	}
	return c
}

// header is the start of every test spec. It takes lines 1 to 5, so the
// text a test appends starts on line 6.
const header = `version: 1
targets: [claude]
project:
  name: tiny
  goal: A tiny project for tests.
`

// loadSpec loads header followed by rest.
func loadSpec(t *testing.T, rest string) *spec.File {
	t.Helper()
	f, err := spec.Load(strings.NewReader(header + rest))
	if err != nil {
		t.Fatalf("test spec does not load:\n%v", err)
	}
	return f
}

// messages returns the text of each error joined in err.
func messages(err error) []string {
	if err == nil {
		return nil
	}
	errs := []error{err}
	var joined interface{ Unwrap() []error }
	if errors.As(err, &joined) {
		errs = joined.Unwrap()
	}
	out := make([]string, len(errs))
	for i, e := range errs {
		out[i] = e.Error()
	}
	return out
}

func warningStrings(ws []Warning) []string {
	var out []string
	for _, w := range ws {
		out = append(out, w.String())
	}
	return out
}

func TestResolve(t *testing.T) {
	c := loadCatalog(t, "catalog")
	tests := []struct {
		name     string
		spec     string   // appended to header, starting on line 6
		ids      []string // resolved entries in order; checked only without errors
		errs     []string
		warnings []string
	}{
		{
			name: "valid",
			spec: "catalog_version: \"2026.10\"\n" +
				"selections: [no-test-tampering, typescript, node, pnpm, go, learning-guided]\n" +
				"answers:\n  domain_notes: Prices are in cents.\n",
			ids: []string{"no-test-tampering", "typescript", "node", "pnpm", "go", "learning-guided"},
		},
		{
			name: "no selections",
			spec: "",
			ids:  []string{},
		},
		{
			name: "unknown entry with a suggestion",
			spec: "selections:\n  - node\n  - pnpn\n",
			errs: []string{`line 8: selections[1]: unknown entry "pnpn"; did you mean "pnpm"?`},
		},
		{
			name: "unknown entry without a suggestion",
			spec: "selections:\n  - kubernetes\n",
			errs: []string{`line 7: selections[0]: unknown entry "kubernetes"`},
		},
		{
			name: "unknown entry under an older pin",
			spec: "catalog_version: \"2026.09\"\nselections:\n  - ghost\n",
			errs: []string{`line 8: selections[0]: unknown entry "ghost" in catalog 2026.10 ` +
				`(the spec pins 2026.09; it may have been renamed or removed)`},
			warnings: []string{"line 6: catalog_version: the spec pins 2026.09 but this cyanotype's catalog is 2026.10, whose entries may have changed; " +
				`review the changes with cyanotype generate --dry-run, then set catalog_version to "2026.10"`},
		},
		{
			name: "unknown entries are left out of other checks",
			spec: "selections:\n  - typo-entry\n  - learning-guided\n  - go\n",
			errs: []string{`line 7: selections[0]: unknown entry "typo-entry"`},
		},
		{
			name: "two entries of one radio category",
			spec: "selections:\n  - node\n  - pnpm\n  - npm\n",
			errs: []string{`line 9: selections[2]: "npm" and "pnpm" (selections[1]) are both in radio category "package-manager"; keep only one`},
		},
		{
			name: "checkboxes of one category combine",
			spec: "selections: [go, python, typescript]\n",
			ids:  []string{"go", "python", "typescript"},
		},
		{
			name: "missing direct requirement",
			spec: "selections:\n  - pnpm\n",
			errs: []string{`line 7: selections[0]: "pnpm" requires "node", which is not selected`},
		},
		{
			name: "missing transitive requirements",
			spec: "selections:\n  - vitest\n",
			errs: []string{`line 7: selections[0]: "vitest" requires "vite" and "node" (through "vite"), which are not selected`},
		},
		{
			name: "a selected requirement reports its own gaps",
			spec: "selections:\n  - vitest\n  - vite\n",
			errs: []string{`line 8: selections[1]: "vite" requires "node", which is not selected`},
		},
		{
			name: "requirements satisfied in any order",
			spec: "selections: [vitest, vite, node]\n",
			ids:  []string{"vitest", "vite", "node"},
		},
		{
			name: "requirement cycle",
			spec: "selections:\n  - cycle-a\n",
			errs: []string{`line 7: selections[0]: "cycle-a" requires "cycle-b", which is not selected`},
		},
		{
			name: "conflict",
			spec: "selections:\n  - sqlite\n  - go\n  - supabase\n",
			errs: []string{`line 9: selections[2]: "supabase" conflicts with "sqlite" (selections[0]); keep only one`},
		},
		{
			name: "placeholder without a value",
			spec: "selections:\n  - learning-guided\n",
			errs: []string{`line 7: selections[0]: "learning-guided" uses {languages}, but no entry of category "language" is selected`},
		},
		{
			name: "unknown answer",
			spec: "answers:\n  domain_note: Prices are in cents.\n  extra: More text.\n",
			errs: []string{
				`line 7: answers.domain_note: unknown question "domain_note"; did you mean "domain_notes"?`,
				`line 8: answers.extra: unknown question "extra"`,
			},
		},
		{
			name: "pin newer than the catalog",
			spec: "catalog_version: \"2026.11\"\n",
			errs: []string{`line 6: catalog_version: "2026.11" is newer than this cyanotype's catalog (2026.10); upgrade cyanotype`},
		},
		{
			name: "pin of a later release in the same month",
			spec: "catalog_version: \"2026.10.1\"\n",
			errs: []string{`line 6: catalog_version: "2026.10.1" is newer than this cyanotype's catalog (2026.10); upgrade cyanotype`},
		},
		{
			name: "pin older than the catalog",
			spec: "catalog_version: \"2026.09\"\nselections: [go]\n",
			ids:  []string{"go"},
			warnings: []string{"line 6: catalog_version: the spec pins 2026.09 but this cyanotype's catalog is 2026.10, whose entries may have changed; " +
				`review the changes with cyanotype generate --dry-run, then set catalog_version to "2026.10"`},
		},
		{
			name: "errors sorted by line",
			spec: "answers:\n  notes: x\nselections:\n  - pnpm\n  - ghost\ncatalog_version: \"2027.01\"\n",
			errs: []string{
				`line 7: answers.notes: unknown question "notes"`,
				`line 9: selections[0]: "pnpm" requires "node", which is not selected`,
				`line 10: selections[1]: unknown entry "ghost" in catalog 2026.10 (the spec pins 2027.01; it may have been renamed or removed)`,
				`line 11: catalog_version: "2027.01" is newer than this cyanotype's catalog (2026.10); upgrade cyanotype`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, warnings, err := Resolve(c, loadSpec(t, tt.spec))

			if got := messages(err); !slices.Equal(got, tt.errs) {
				t.Errorf("errors =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(tt.errs, "\n"))
			}
			if got := warningStrings(warnings); !slices.Equal(got, tt.warnings) {
				t.Errorf("warnings =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(tt.warnings, "\n"))
			}
			if err != nil {
				if res != nil {
					t.Error("Resolve() returned a result together with an error")
				}
				return
			}
			ids := []string{}
			for _, e := range res.Entries {
				ids = append(ids, e.ID)
			}
			if !slices.Equal(ids, tt.ids) {
				t.Errorf("entries = %q, want %q", ids, tt.ids)
			}
		})
	}
}

// TestResolveOneOf checks requires_one_of groups, including the cycle
// between node, which requires one of npm or pnpm, and the package
// managers, which require node.
func TestResolveOneOf(t *testing.T) {
	c := loadCatalog(t, "one-of")
	tests := []struct {
		name string
		spec string   // appended to header, starting on line 6
		ids  []string // resolved entries in order; checked only without errors
		errs []string
	}{
		{
			name: "unmet group",
			spec: "selections:\n  - node\n",
			errs: []string{`line 7: selections[0]: "node" requires one of "npm" or "pnpm", but none is selected`},
		},
		{
			name: "cycle through a group met by the selection itself",
			spec: "selections:\n  - pnpm\n",
			errs: []string{`line 7: selections[0]: "pnpm" requires "node", which is not selected`},
		},
		{
			name: "cycle with every entry selected",
			spec: "selections: [pnpm, node]\n",
			ids:  []string{"pnpm", "node"},
		},
		{
			name: "two alternatives of one radio category",
			spec: "selections:\n  - node\n  - npm\n  - pnpm\n",
			errs: []string{`line 9: selections[2]: "pnpm" and "npm" (selections[1]) are both in radio category "package-manager"; keep only one`},
		},
		{
			name: "missing requires and an unmet group",
			spec: "selections:\n  - shadcn-ui\n",
			errs: []string{
				`line 7: selections[0]: "shadcn-ui" requires "tailwind", which is not selected`,
				`line 7: selections[0]: "shadcn-ui" requires one of "nextjs" or "vite-react", but none is selected`,
			},
		},
		{
			name: "group met by one alternative",
			spec: "selections: [shadcn-ui, tailwind, vite-react, node, pnpm]\n",
			ids:  []string{"shadcn-ui", "tailwind", "vite-react", "node", "pnpm"},
		},
		{
			name: "a selected alternative reports its own gaps",
			spec: "selections:\n  - shadcn-ui\n  - tailwind\n  - nextjs\n",
			errs: []string{
				`line 9: selections[2]: "nextjs" requires "node", which is not selected`,
				`line 9: selections[2]: "nextjs" requires one of "npm" or "pnpm" (through "node"), but none is selected`,
			},
		},
		{
			name: "unmet group through an unselected requirement",
			spec: "selections:\n  - docs-kit\n  - tailwind\n",
			errs: []string{
				`line 7: selections[0]: "docs-kit" requires "shadcn-ui", which is not selected`,
				`line 7: selections[0]: "docs-kit" requires one of "nextjs" or "vite-react" (through "shadcn-ui"), but none is selected`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, warnings, err := Resolve(c, loadSpec(t, tt.spec))
			if len(warnings) > 0 {
				t.Errorf("warnings = %q, want none", warningStrings(warnings))
			}
			if got := messages(err); !slices.Equal(got, tt.errs) {
				t.Errorf("errors =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(tt.errs, "\n"))
			}
			if err != nil {
				return
			}
			var ids []string
			for _, e := range res.Entries {
				ids = append(ids, e.ID)
			}
			if !slices.Equal(ids, tt.ids) {
				t.Errorf("entries = %q, want %q", ids, tt.ids)
			}
		})
	}
}

func TestResolveResult(t *testing.T) {
	c := loadCatalog(t, "catalog")
	f := loadSpec(t, "selections: [typescript, learning-guided, python, go, no-test-tampering]\n"+
		"locked: [no-test-tampering]\n")
	res, warnings, err := Resolve(c, f)
	if err != nil || len(warnings) > 0 {
		t.Fatalf("Resolve() warnings %q, error:\n%v", warningStrings(warnings), err)
	}

	if res.CatalogVersion != "2026.10" {
		t.Errorf("CatalogVersion = %q, want %q", res.CatalogVersion, "2026.10")
	}
	if want := f.Spec.WithDefaults(); !reflect.DeepEqual(res.Spec, want) {
		t.Errorf("Spec =\n%+v\nwant the spec with defaults\n%+v", res.Spec, want)
	}

	learning := res.Entries[1]
	if want := "The user is learning TypeScript, Python and Go. Keep `{languages}` literal in code."; learning.Snippet.Rule != want {
		t.Errorf("filled rule = %q, want %q", learning.Snippet.Rule, want)
	}
	if want := "The user wants to understand TypeScript, Python and Go code."; learning.Snippet.Why != want {
		t.Errorf("filled why = %q, want %q", learning.Snippet.Why, want)
	}
	// Filling works on a copy: the catalog keeps the placeholder.
	if e, _ := c.Get("learning-guided"); !strings.Contains(e.Snippet.Rule, "learning {languages}") {
		t.Errorf("catalog entry changed to %q", e.Snippet.Rule)
	}

	for _, e := range res.Entries {
		if want := e.ID == "no-test-tampering"; e.Locked != want {
			t.Errorf("%s: Locked = %v, want %v", e.ID, e.Locked, want)
		}
	}
}

// TestResolveValidatesSpec checks that a spec built in code goes through
// spec.Validate too, with lines when the spec comes from a file.
func TestResolveValidatesSpec(t *testing.T) {
	c := loadCatalog(t, "catalog")

	f := spec.New(spec.Spec{Version: spec.CurrentVersion, Project: spec.Project{Name: "x", Goal: "y"}})
	_, _, err := Resolve(c, f)
	if got, want := messages(err), []string{"targets: is required; list at least one agent"}; !slices.Equal(got, want) {
		t.Errorf("errors = %q, want %q", got, want)
	}

	loaded := loadSpec(t, "selections: [go]\n")
	loaded.Spec.Selections = append(loaded.Spec.Selections, "go")
	_, _, err = Resolve(c, loaded)
	if got, want := messages(err), []string{`line 6: selections[1]: "go" is listed twice`}; !slices.Equal(got, want) {
		t.Errorf("errors = %q, want %q", got, want)
	}
}

// TestResolveUnversionedCatalog checks that a catalog without a manifest,
// as in development, skips the pin check.
func TestResolveUnversionedCatalog(t *testing.T) {
	c, err := catalog.Load(os.DirFS(filepath.Join("testdata", "catalog", "entries")))
	if err != nil {
		t.Fatal(err)
	}
	res, warnings, err := Resolve(c, loadSpec(t, "catalog_version: \"2030.01\"\nselections: [go]\n"))
	if err != nil || len(warnings) > 0 {
		t.Fatalf("Resolve() warnings %q, error:\n%v", warningStrings(warnings), err)
	}
	if res.CatalogVersion != "" {
		t.Errorf("CatalogVersion = %q, want empty", res.CatalogVersion)
	}
}

// TestFillersCoverPlaceholders checks that every placeholder the catalog
// accepts can be filled here.
func TestFillersCoverPlaceholders(t *testing.T) {
	for _, name := range catalog.Placeholders() {
		if _, ok := fillers[name]; !ok {
			t.Errorf("placeholder {%s} has no filler", name)
		}
	}
	if len(fillers) != len(catalog.Placeholders()) {
		t.Errorf("fillers has %d entries, catalog.Placeholders() %d", len(fillers), len(catalog.Placeholders()))
	}
}

func TestJoinNames(t *testing.T) {
	tests := []struct {
		names []string
		want  string
	}{
		{nil, ""},
		{[]string{"Go"}, "Go"},
		{[]string{"Go", "TypeScript"}, "Go and TypeScript"},
		{[]string{"Go", "Python", "TypeScript"}, "Go, Python and TypeScript"},
	}
	for _, tt := range tests {
		if got := joinNames(tt.names); got != tt.want {
			t.Errorf("joinNames(%q) = %q, want %q", tt.names, got, tt.want)
		}
	}
}

func TestJoinList(t *testing.T) {
	tests := []struct {
		names []string
		conj  string
		want  string
	}{
		{nil, "or", ""},
		{[]string{`"npm"`}, "or", `"npm"`},
		{[]string{`"npm"`, `"pnpm"`}, "or", `"npm" or "pnpm"`},
		{[]string{`"npm"`, `"pnpm"`, `"yarn"`}, "or", `"npm", "pnpm" or "yarn"`},
		{[]string{"Go", "Python", "TypeScript"}, "and", "Go, Python and TypeScript"},
	}
	for _, tt := range tests {
		if got := joinList(tt.names, tt.conj); got != tt.want {
			t.Errorf("joinList(%q, %q) = %q, want %q", tt.names, tt.conj, got, tt.want)
		}
	}
}

// TestResolveCodes checks the stable code of every check Resolve adds,
// which the JSON output exposes to scripts and agents.
func TestResolveCodes(t *testing.T) {
	tests := []struct {
		name    string
		catalog string
		spec    string // appended to header
		want    []string
	}{
		{"unknown entry", "catalog", "selections: [pnpn]\n", []string{CodeUnknownEntry}},
		{"radio clash", "catalog", "selections: [node, pnpm, npm]\n", []string{CodeRadioClash}},
		{"missing requirement", "catalog", "selections: [pnpm]\n", []string{CodeMissingRequirement}},
		{"missing one of", "one-of", "selections: [node]\n", []string{CodeMissingOneOf}},
		{"conflict", "catalog", "selections: [sqlite, go, supabase]\n", []string{CodeConflict}},
		{"unfilled placeholder", "catalog", "selections: [learning-guided]\n", []string{CodeUnfilledPlaceholder}},
		{"unknown question", "catalog", "answers:\n  domain_note: x\n", []string{CodeUnknownQuestion}},
		{"newer catalog pin", "catalog", "catalog_version: \"2099.01\"\n", []string{CodeCatalogNewer}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := Resolve(loadCatalog(t, tt.catalog), loadSpec(t, tt.spec))
			if got := errorCodes(t, err); !slices.Equal(got, tt.want) {
				t.Errorf("codes = %q, want %q\nerror: %v", got, tt.want, err)
			}
		})
	}

	// A spec built in code skips Load, so Resolve runs spec.Validate, whose
	// errors keep their own codes.
	f := spec.New(spec.Spec{Version: 1, Targets: []spec.Target{spec.TargetClaude},
		Project:    spec.Project{Name: "tiny", Goal: "A tiny project for tests."},
		Selections: []string{"go"}, Locked: []string{"node"}})
	_, _, err := Resolve(loadCatalog(t, "catalog"), f)
	if got, want := errorCodes(t, err), []string{spec.CodeNotSelected}; !slices.Equal(got, want) {
		t.Errorf("spec built in code: codes = %q, want %q", got, want)
	}
}

// errorCodes returns the Code of each *spec.ValidationError joined in err,
// failing the test when err is nil or holds another kind of error.
func errorCodes(t *testing.T, err error) []string {
	t.Helper()
	if err == nil {
		t.Fatal("Resolve() succeeded, want an error")
	}
	errs := []error{err}
	var joined interface{ Unwrap() []error }
	if errors.As(err, &joined) {
		errs = joined.Unwrap()
	}
	var out []string
	for _, e := range errs {
		var ve *spec.ValidationError
		if !errors.As(e, &ve) {
			t.Fatalf("error %q is not a *spec.ValidationError", e)
		}
		out = append(out, ve.Code)
	}
	return out
}
