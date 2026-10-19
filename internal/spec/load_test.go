package spec

import (
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"testing/iotest"

	"go.yaml.in/yaml/v3"
)

// loadFile opens a file under testdata and loads it. name is slash-separated.
func loadFile(t *testing.T, name string) (*File, error) {
	t.Helper()
	f, err := os.DirFS("testdata").Open(name)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	return Load(f)
}

func TestLoadValid(t *testing.T) {
	f, err := loadFile(t, "valid/minimal.yaml")
	if err != nil {
		t.Fatalf("Load() error:\n%v", err)
	}
	want := Spec{
		Version: 1,
		Targets: []Target{TargetClaude},
		Project: Project{Name: "tiny", Goal: "A tiny project for tests."},
	}
	if !reflect.DeepEqual(f.Spec, want) {
		t.Errorf("Spec =\n%+v\nwant\n%+v", f.Spec, want)
	}
}

func TestLoadInvalid(t *testing.T) {
	// want lists every expected problem in order; msg is matched as a substring.
	const anyLine = -1
	type wantErr struct {
		field string
		line  int
		msg   string
	}
	tests := []struct {
		file string
		want []wantErr
	}{
		{"missing-required", []wantErr{
			{"targets", 0, "is required"},
			{"project.name", 3, "is required"},
			{"project.goal", 3, "is required"},
		}},
		{"bad-enums", []wantErr{
			{"targets[1]", 2, `must be "claude", "agents-md", "gemini", "cursor" or "copilot", got "vscode"`},
			{"mode", 3, `must be "simple" or "advanced", got "expert"`},
			{"i18n.translations_by", 9, `got "robots"`},
			{"output.split", 11, `must be "auto", "always" or "never", got "sometimes"`},
		}},
		{"bad-formats", []wantErr{
			{"catalog_version", 2, `"2026.13" must be a catalog version`},
			{"project.brief", 7, "must be a relative path inside the project"},
			{"languages.code", 9, "must be a lowercase language name"},
			{"selections[1]", 10, `"Next.js" must be a catalog entry id`},
			{"i18n.default_locale", 12, `use "-" instead of "_"`},
			{"i18n.locales[0]", 13, `use "-" instead of "_"`},
			{"i18n.locales[1]", 13, `"EN" must be a BCP 47 tag`},
			{"answers.DomainNotes", 15, "key must be snake_case"},
			{"output.max_lines", 17, "must be positive, got -5"},
			{"output.topic_dir", 18, "must be a relative path inside the project"},
		}},
		{"bad-lists", []wantErr{
			{"targets[2]", 2, `"claude" is listed twice`},
			{"project.name", 4, "must be a single line"},
			{"project.core_features[1]", 8, "is empty"},
			{"selections[2]", 9, `"typescript" is listed twice`},
			{"selections[3]", 9, "is empty"},
			{"locked[0]", 10, `"no-test-tampering" is not in selections`},
			{"i18n.locales", 13, `must include the default locale "it"`},
			{"answers.domain_notes", 15, "is empty"},
			{"custom_rules[0].why", 17, "is required"},
			{"custom_rules[1].rule", 18, "is required"},
		}},
		{"unknown-fields", []wantErr{
			{"project.owner", 6, `unknown field "owner"; check for a typo`},
			{"tags", 7, `unknown field "tags"`},
		}},
		{"wrong-type", []wantErr{
			{"targets", 2, `expected a list, got text "claude"`},
			{"output.max_lines", 7, `expected an integer, got text "many"`},
		}},
		{"syntax-error", []wantErr{
			// yaml.v3 reports some parser errors one line early, so the line is not checked.
			{"", anyLine, "invalid YAML: did not find expected ',' or ']'"},
		}},
		{"empty", []wantErr{
			{"", 0, "file is empty"},
		}},
		{"comments-only", []wantErr{
			{"", 0, "file is empty"},
		}},
		{"multiple-documents", []wantErr{
			{"", 6, "exactly one YAML document"},
		}},
		{"not-a-mapping", []wantErr{
			{"", 1, "top level must be a mapping"},
		}},
		{"aliases", []wantErr{
			{"targets", 2, "anchors are not supported"},
			{"selections", 6, "aliases are not supported"},
		}},
		{"version-missing", []wantErr{
			{"version", 0, "is required"},
		}},
		{"version-string", []wantErr{
			{"version", 1, `must be an integer, got "1"`},
		}},
		{"version-zero", []wantErr{
			{"version", 1, `"0" is not a valid version`},
		}},
		{"version-newer", []wantErr{
			{"version", 1, "version 2 is newer than this cyanotype supports (1); upgrade cyanotype"},
		}},
	}

	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			f, err := loadFile(t, "invalid/"+tt.file+".yaml")
			if err == nil {
				t.Fatalf("Load() = %+v, want error", f.Spec)
			}
			if f != nil {
				t.Errorf("Load() returned a file together with an error")
			}

			got := validationErrors(t, err)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d errors, want %d:\n%v", len(got), len(tt.want), err)
			}
			for i, w := range tt.want {
				g := got[i]
				if g.Field != w.field || (w.line != anyLine && g.Line != w.line) || !strings.Contains(g.Msg, w.msg) {
					t.Errorf("error %d = %q\nwant field %q, line %d, message containing %q", i, g, w.field, w.line, w.msg)
				}
			}
		})
	}
}

func TestLoadTooLarge(t *testing.T) {
	_, err := Load(strings.NewReader(strings.Repeat("#", maxFileSize+1)))
	var ve *ValidationError
	if !errors.As(err, &ve) || !strings.Contains(ve.Msg, "larger than 1 MiB") {
		t.Errorf("Load() error = %v, want a validation error about the size", err)
	}
}

func TestLoadReadError(t *testing.T) {
	_, err := Load(iotest.ErrReader(errors.New("disk on fire")))
	var ve *ValidationError
	if err == nil || errors.As(err, &ve) {
		t.Errorf("Load() error = %v, want an I/O error, not a validation error", err)
	}
}

func TestValidate(t *testing.T) {
	valid := Spec{
		Version: CurrentVersion,
		Targets: []Target{TargetAgentsMD},
		Project: Project{Name: "x", Goal: "y"},
	}
	if err := Validate(valid); err != nil {
		t.Errorf("Validate(valid) = %v, want nil", err)
	}

	tests := []struct {
		name string
		edit func(s *Spec)
		want string
	}{
		{"missing version", func(s *Spec) { s.Version = 0 }, "version: is required"},
		{"other version", func(s *Spec) { s.Version = 7 }, "version: must be 1, got 7"},
		{"blank goal", func(s *Spec) { s.Project.Goal = "  " }, "project.goal: is required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := valid
			tt.edit(&s)
			if err := Validate(s); err == nil || err.Error() != tt.want {
				t.Errorf("Validate() = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestMigrate(t *testing.T) {
	// Fake steps: version 1 renamed "old" to "mid", version 2 renamed it to "new".
	rename := func(from, to string) migration {
		return func(doc *yaml.Node) error {
			k, _ := mappingPair(root(doc), from)
			if k == nil {
				return errors.New(from + " not found")
			}
			k.Value = to
			return nil
		}
	}
	steps := map[int]migration{1: rename("old", "mid"), 2: rename("mid", "new")}

	tests := []struct {
		name     string
		from, to int
		want     string // encoded document, or error substring
		wantErr  bool
	}{
		{"two steps", 1, 3, "version: 3\nnew: x # kept\n", false},
		{"missing step", 1, 4, "no migration from spec version 3 to 4", true},
		{"failing step", 2, 3, "migrate spec from version 2 to 3: mid not found", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var doc yaml.Node
			if err := yaml.Unmarshal([]byte("version: 1\nold: x # kept\n"), &doc); err != nil {
				t.Fatal(err)
			}
			err := migrate(&doc, tt.from, tt.to, steps)
			if tt.wantErr {
				if err == nil || !strings.Contains(err.Error(), tt.want) {
					t.Errorf("migrate() error = %v, want one containing %q", err, tt.want)
				}
				return
			}
			if err != nil {
				t.Fatalf("migrate() error = %v", err)
			}
			out, err := yaml.Marshal(&doc)
			if err != nil {
				t.Fatal(err)
			}
			if string(out) != tt.want {
				t.Errorf("migrated document =\n%s\nwant\n%s", out, tt.want)
			}
		})
	}
}

// validationErrors unpacks the errors joined by Load.
func validationErrors(t *testing.T, err error) []*ValidationError {
	t.Helper()
	var errs []error
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		errs = joined.Unwrap()
	} else {
		errs = []error{err}
	}
	var out []*ValidationError
	for _, e := range errs {
		var ve *ValidationError
		if !errors.As(e, &ve) {
			t.Fatalf("error %v is not a *ValidationError", e)
		}
		out = append(out, ve)
	}
	return out
}
