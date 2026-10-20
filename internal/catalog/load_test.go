package catalog

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadValid(t *testing.T) {
	c, err := Load(os.DirFS(filepath.Join("testdata", "valid")))
	if err != nil {
		t.Fatalf("Load() error:\n%v", err)
	}

	want := []Entry{
		{
			ID:            "alpha",
			Name:          "Alpha",
			Category:      "language",
			Select:        SelectCheckbox,
			URL:           "https://alpha.example.org",
			Summary:       "A language for tests",
			RequiresOneOf: [][]string{{"beta", "gamma"}},
			Level:         LevelBeginner,
			Topic:         "style",
			Snippet:       Snippet{Rule: "Write Alpha.", Why: "Tests need a language."},
			Detect:        &Detect{Files: []string{"alpha.mod"}},
		},
		{
			ID:        "beta",
			Name:      "Beta",
			Category:  "database",
			Select:    SelectRadio,
			Summary:   "A database for tests",
			Requires:  []string{"alpha"},
			Conflicts: []string{"gamma"},
			Snippet:   Snippet{Rule: "Store data in Beta.", Why: "Tests need a database."},
			Enforce:   &Enforce{Claude: &ClaudeEnforce{Deny: []string{"Bash(other-db:*)"}}},
		},
		{
			ID:        "gamma",
			Name:      "Gamma",
			Category:  "framework",
			Select:    SelectRadio,
			Summary:   "A framework for tests",
			Conflicts: []string{"beta"},
			Snippet:   Snippet{Rule: "Build pages with Gamma.\nKeep one component per file.\n", Why: "Tests need a framework."},
		},
	}
	if got := c.Entries(); !reflect.DeepEqual(got, want) {
		t.Errorf("Entries() =\n%+v\nwant\n%+v", got, want)
	}

	if got, ok := c.Get("beta"); !ok || got.Name != "Beta" {
		t.Errorf(`Get("beta") = %+v, %v; want entry named Beta, true`, got, ok)
	}
	if _, ok := c.Get("missing"); ok {
		t.Error(`Get("missing") ok = true, want false`)
	}

	// Entries returns a copy: changing it must not change the catalog.
	c.Entries()[0] = Entry{}
	if got := c.Entries()[0].ID; got != "alpha" {
		t.Errorf("after modifying the returned slice, first entry id = %q, want %q", got, "alpha")
	}
}

func TestLoadInvalid(t *testing.T) {
	// want lists every expected problem in order; msg is matched as a substring.
	type wantErr struct{ file, field, msg string }
	tests := []struct {
		dir  string
		want []wantErr
	}{
		{"missing-fields", []wantErr{
			{"alpha.yaml", "name", "is required"},
			{"alpha.yaml", "summary", "is required"},
			{"alpha.yaml", "snippet.why", "is required"},
		}},
		{"bad-enums", []wantErr{
			{"alpha.yaml", "select", `must be "radio" or "checkbox", got "dropdown"`},
			{"alpha.yaml", "level", `got "expert"`},
		}},
		{"bad-format", []wantErr{
			{"Alpha_One.yaml", "id", "must be kebab-case"},
			{"Alpha_One.yaml", "category", "must be kebab-case"},
			{"Alpha_One.yaml", "topic", "must be kebab-case"},
			{"Alpha_One.yaml", "url", "must be an absolute https URL"},
		}},
		{"filename-mismatch", []wantErr{
			{"alpha.yaml", "id", "rename the file to beta.yaml"},
		}},
		{"duplicate-id", []wantErr{
			{"alpha-copy.yaml", "id", "does not match the file name"},
			{"alpha.yaml", "id", `"alpha" is already used by alpha-copy.yaml`},
		}},
		{"unknown-references", []wantErr{
			{"alpha.yaml", "requires[0]", `unknown entry "ghost"`},
			{"alpha.yaml", "conflicts[0]", `unknown entry "phantom"`},
		}},
		{"reference-lists", []wantErr{
			{"alpha.yaml", "requires[3]", "is empty"},
			{"alpha.yaml", "requires[1]", "cannot reference itself"},
			{"alpha.yaml", "requires[2]", `"beta" is listed twice`},
			{"alpha.yaml", "conflicts[0]", `"beta" is also listed in requires`},
		}},
		{"one-of-groups", []wantErr{
			{"alpha.yaml", "requires_one_of[0]", "needs at least two alternatives; move a single id to requires"},
			{"alpha.yaml", "requires_one_of[0][0]", `"beta" is also listed in requires`},
			{"alpha.yaml", "requires_one_of[1][3]", "is empty"},
			{"alpha.yaml", "requires_one_of[1][0]", "cannot reference itself"},
			{"alpha.yaml", "requires_one_of[1][2]", `"delta" is listed twice`},
			{"alpha.yaml", "requires_one_of[2][0]", `"gamma" is also listed in conflicts`},
		}},
		{"unknown-one-of", []wantErr{
			{"alpha.yaml", "requires_one_of[0][1]", `unknown entry "ghost"`},
		}},
		{"asymmetric-conflict", []wantErr{
			{"beta.yaml", "conflicts", `missing "alpha": alpha.yaml declares a conflict`},
		}},
		{"same-radio-category", []wantErr{
			{"alpha.yaml", "conflicts[0]", `same radio category "package-manager"`},
			{"beta.yaml", "conflicts[0]", `same radio category "package-manager"`},
		}},
		{"mixed-select", []wantErr{
			{"beta.yaml", "select", `"checkbox" differs from "radio" used by alpha.yaml in category "database"`},
		}},
		{"unknown-placeholder", []wantErr{
			{"alpha.yaml", "snippet.rule", "unknown placeholder {language}; known placeholders: {languages}"},
			{"alpha.yaml", "snippet.why", "unknown placeholder {reason}"},
		}},
		{"blank-list-items", []wantErr{
			{"alpha.yaml", "enforce.claude.deny[1]", "is empty"},
			{"alpha.yaml", "detect.files[0]", "is empty"},
		}},
		{"bad-detect", []wantErr{
			{"alpha.yaml", "detect.dependencies.npm[1]", "is empty"},
			{"alpha.yaml", "detect.dependencies.npm[2]", `"al pha" contains spaces`},
			{"alpha.yaml", "detect.dependencies.npm[3]", `"alpha" is listed twice`},
			{"alpha.yaml", "detect.dependencies.pypi[0]", `"Alpha_Tools" must be written in its normalized form, "alpha-tools"`},
			{"alpha.yaml", "detect.dependencies.go[0]", "without leading or trailing slashes"},
			{"alpha.yaml", "detect.run_script", "must contain {script} exactly once"},
		}},
		{"empty-dependencies", []wantErr{
			{"alpha.yaml", "detect.dependencies", "lists no dependencies"},
		}},
		// Ecosystems are fields, so a misspelled or unsupported one is an
		// error rather than a list detection never reads.
		{"unknown-ecosystem", []wantErr{
			{"alpha.yaml", "", `line 11: unknown field "cargo"`},
		}},
		{"bad-ask", []wantErr{
			{"alpha.yaml", "enforce.claude.ask[1]", "is empty"},
			{"alpha.yaml", "enforce.claude.ask[2]", `"Bash(beta *)" is also listed in enforce.claude.deny, which wins`},
		}},
		{"bad-with", []wantErr{
			{"alpha.yaml", "enforce.claude.with.Not_Kebab", "must be an entry id in kebab-case"},
			{"alpha.yaml", "enforce.claude.with.alpha", "an entry cannot reference itself"},
			{"alpha.yaml", "enforce.claude.with.delta", "has no ask rules"},
			{"alpha.yaml", "enforce.claude.with.gamma.ask[1]", "is empty"},
			{"alpha.yaml", "enforce.claude.with.gamma.ask[2]", `"Bash(beta *)" is also listed in enforce.claude.deny, which wins`},
			{"alpha.yaml", "enforce.claude.with.beta", `"beta" is in the same radio category "package-manager"`},
			{"alpha.yaml", "enforce.claude.with.ghost", `unknown entry "ghost"`},
		}},
		// Only ask rules are supported inside with; deny is a possible
		// later extension, so for now it is an unknown field.
		{"with-deny", []wantErr{
			{"alpha.yaml", "", `line 13: unknown field "deny"`},
		}},
		{"bad-gitignore", []wantErr{
			{"alpha.yaml", "enforce.gitignore[4]", "is empty"},
			{"alpha.yaml", "enforce.gitignore[1]", "leading or trailing spaces"},
			{"alpha.yaml", "enforce.gitignore[2]", "starts with #, which makes it a comment"},
			{"alpha.yaml", "enforce.gitignore[3]", `".env" is listed twice`},
			{"alpha.yaml", "enforce.gitignore[5]", "must be a single line"},
		}},
		{"unknown-fields", []wantErr{
			{"alpha.yaml", "", `line 6: unknown field "tags"`},
			{"alpha.yaml", "", `line 10: unknown field "notes"`},
		}},
		{"wrong-type", []wantErr{
			{"alpha.yaml", "", "line 6: cannot unmarshal"},
		}},
		{"syntax-error", []wantErr{
			{"alpha.yaml", "", "invalid YAML"},
		}},
		{"empty-file", []wantErr{
			{"alpha.yaml", "", "file is empty"},
		}},
		{"multiple-documents", []wantErr{
			{"alpha.yaml", "", "exactly one YAML document"},
		}},
		{"folder-layout", []wantErr{
			{"README.md", "", "only .yaml files are allowed"},
			{"alpha.yml", "", "only .yaml files are allowed"},
			{"languages", "", "subfolders are not allowed"},
		}},
	}

	for _, tt := range tests {
		t.Run(tt.dir, func(t *testing.T) {
			c, err := Load(os.DirFS(filepath.Join("testdata", "invalid", tt.dir)))
			if err == nil {
				t.Fatalf("Load() = %d entries, want error", len(c.Entries()))
			}
			if c != nil {
				t.Errorf("Load() returned a catalog together with an error")
			}

			got := validationErrors(t, err)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d errors, want %d:\n%v", len(got), len(tt.want), err)
			}
			for i, w := range tt.want {
				g := got[i]
				if g.File != w.file || g.Field != w.field || !strings.Contains(g.Msg, w.msg) {
					t.Errorf("error %d = %q\nwant file %q, field %q, message containing %q", i, g, w.file, w.field, w.msg)
				}
			}
		})
	}
}

func TestLoadMissingFolder(t *testing.T) {
	_, err := Load(os.DirFS(filepath.Join("testdata", "does-not-exist")))
	if err == nil {
		t.Fatal("Load() error = nil, want error")
	}
	var ve *ValidationError
	if errors.As(err, &ve) {
		t.Errorf("Load() error = %v, want an I/O error, not a validation error", err)
	}
}

// validationErrors unpacks the errors joined by Load.
func validationErrors(t *testing.T, err error) []*ValidationError {
	t.Helper()
	joined, ok := err.(interface{ Unwrap() []error })
	if !ok {
		t.Fatalf("error %v is not a joined error", err)
	}
	var out []*ValidationError
	for _, e := range joined.Unwrap() {
		var ve *ValidationError
		if !errors.As(e, &ve) {
			t.Fatalf("error %v is not a *ValidationError", e)
		}
		out = append(out, ve)
	}
	return out
}
