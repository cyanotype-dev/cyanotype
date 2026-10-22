package spec

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite golden files with the current output")

// save loads in, applies edit to the spec and returns what Save writes.
func save(t *testing.T, in string, edit func(s *Spec)) string {
	t.Helper()
	f, err := Load(strings.NewReader(in))
	if err != nil {
		t.Fatalf("Load() error:\n%v", err)
	}
	if edit != nil {
		edit(&f.Spec)
	}
	var out bytes.Buffer
	if err := f.Save(&out); err != nil {
		t.Fatalf("Save() error:\n%v", err)
	}
	return out.String()
}

// TestSaveRoundTrip loads a commented file in a non-canonical layout,
// changes one selection and compares the result with a golden file: the
// comments and key order survive and the layout becomes canonical.
func TestSaveRoundTrip(t *testing.T) {
	in, err := os.ReadFile(filepath.Join("testdata", "roundtrip", "input.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	got := save(t, string(in), func(s *Spec) {
		i := slices.Index(s.Selections, "postgres")
		if i < 0 {
			t.Fatal(`input has no "postgres" selection`)
		}
		s.Selections[i] = "sqlite"
	})

	const golden = "testdata/roundtrip/golden.yaml"
	if *update {
		if err := os.WriteFile(golden, []byte(got), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Errorf("Save() output differs from %s (run go test ./internal/spec -update to rewrite it):\n%s", golden, got)
	}
}

// TestSaveCanonicalIsUnchanged checks that a file already in canonical
// layout comes back byte for byte when nothing changes.
func TestSaveCanonicalIsUnchanged(t *testing.T) {
	in, err := os.ReadFile(filepath.Join("testdata", "canonical.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if got := save(t, string(in), nil); got != string(in) {
		t.Errorf("Save() changed a canonical file:\n%s", got)
	}
}

// TestSaveIsStable checks that saving the output of Save again changes nothing.
func TestSaveIsStable(t *testing.T) {
	in, err := os.ReadFile(filepath.Join("testdata", "roundtrip", "input.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	first := save(t, string(in), nil)
	if second := save(t, first, nil); second != first {
		t.Errorf("second Save() =\n%s\nwant the first output\n%s", second, first)
	}
}

func TestSaveMerge(t *testing.T) {
	// head is a valid start shared by every case; it is already canonical.
	const head = "version: 1\n\ntargets: [claude]\n\nproject:\n  name: x\n  goal: y\n"
	tests := []struct {
		name string
		in   string
		edit func(s *Spec)
		want string
	}{
		{
			name: "changed scalar keeps its line comment",
			in:   head + "mode: simple # last used\n",
			edit: func(s *Spec) { s.Mode = ModeAdvanced },
			want: head + "\nmode: advanced # last used\n",
		},
		{
			name: "unchanged catalog version keeps its plain style",
			in:   "version: 1\ncatalog_version: 2026.10\n" + strings.TrimPrefix(head, "version: 1\n"),
			want: "version: 1\n\ncatalog_version: 2026.10\n" + strings.TrimPrefix(head, "version: 1\n"),
		},
		{
			name: "changed catalog version gets the quotes it needs",
			in:   "version: 1\ncatalog_version: 2026.10\n" + strings.TrimPrefix(head, "version: 1\n"),
			edit: func(s *Spec) { s.CatalogVersion = "2026.11" },
			want: "version: 1\n\ncatalog_version: \"2026.11\"\n" + strings.TrimPrefix(head, "version: 1\n"),
		},
		{
			name: "new key goes after the key before it in the spec",
			in:   head,
			edit: func(s *Spec) { s.CatalogVersion = "2026.10" },
			want: "version: 1\n\ncatalog_version: \"2026.10\"\n" + strings.TrimPrefix(head, "version: 1\n"),
		},
		{
			name: "new catalog version is quoted even when YAML would read it as a string",
			in:   head,
			edit: func(s *Spec) { s.CatalogVersion = "2026.10.1" },
			want: "version: 1\n\ncatalog_version: \"2026.10.1\"\n" + strings.TrimPrefix(head, "version: 1\n"),
		},
		{
			name: "cleared block is removed",
			in:   head + "i18n:\n  default_locale: en # main market\n",
			edit: func(s *Spec) { s.I18n = nil },
			want: head,
		},
		{
			name: "added selection keeps flow style",
			in:   head + "selections: [go, pnpm] # stack\n",
			edit: func(s *Spec) { s.Selections = append(s.Selections, "npm") },
			want: head + "\nselections: [go, pnpm, npm] # stack\n",
		},
		{
			name: "removed selection drops only its own comment",
			in:   head + "selections:\n  - go # language\n  - pnpm # package manager\n",
			edit: func(s *Spec) { s.Selections = []string{"pnpm"} },
			want: head + "\nselections:\n  - pnpm # package manager\n",
		},
		{
			name: "reordered selections keep their comments",
			in:   head + "selections:\n  - go # language\n  - pnpm # package manager\n",
			edit: func(s *Spec) { s.Selections = []string{"pnpm", "go"} },
			want: head + "\nselections:\n  - pnpm # package manager\n  - go # language\n",
		},
		{
			name: "flow sequence of 80 characters stays flow",
			in:   head + "selections: [" + strings.Repeat("a", 66) + "]\n",
			want: head + "\nselections: [" + strings.Repeat("a", 66) + "]\n",
		},
		{
			name: "flow sequence of 81 characters becomes a block list",
			in:   head + "selections: [" + strings.Repeat("a", 67) + "]\n",
			want: head + "\nselections:\n  - " + strings.Repeat("a", 67) + "\n",
		},
		{
			name: "nested flow sequence counts its indentation",
			in:   head + "  core_features: [" + strings.Repeat("a", 62) + "]\n",
			want: head + "  core_features:\n    - " + strings.Repeat("a", 62) + "\n",
		},
		{
			name: "flow item with a comment becomes a block list",
			in:   head + "selections: [go, # language\n  pnpm]\n",
			want: head + "\nselections:\n  - go # language\n  - pnpm\n",
		},
		{
			name: "custom rules merge by position",
			in:   head + "custom_rules:\n  # The one rule.\n  - rule: Use metric units.\n    why: One place for conversions. # short\n",
			edit: func(s *Spec) { s.CustomRules[0].Rule = "Use SI units." },
			want: head + "\ncustom_rules:\n  # The one rule.\n  - rule: Use SI units.\n    why: One place for conversions. # short\n",
		},
		{
			name: "blank lines are canonical",
			in:   "\n\nversion: 1\n\n\ntargets: [claude]\nproject:\n\n  name: x\n\n  goal: y\n\n\n",
			want: head,
		},
		{
			name: "CRLF line endings become LF",
			in:   strings.ReplaceAll(head, "\n", "\r\n"),
			want: head,
		},
		{
			name: "file comments stay at the top and bottom",
			in:   "# Header.\n\n" + head + "\n# Footer.\n",
			want: "# Header.\n\n" + head + "\n# Footer.\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := save(t, tt.in, tt.edit); got != tt.want {
				t.Errorf("Save() =\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}

func TestSaveNew(t *testing.T) {
	f := New(Spec{
		Version:    CurrentVersion,
		Targets:    []Target{TargetClaude, TargetAgentsMD},
		Project:    Project{Name: "new", Goal: "A new project."},
		Selections: []string{"go"},
		Output:     Output{MaxLines: 80},
	})
	var out bytes.Buffer
	if err := f.Save(&out); err != nil {
		t.Fatalf("Save() error:\n%v", err)
	}
	want := `version: 1

targets:
  - claude
  - agents-md

project:
  name: new
  goal: A new project.

selections:
  - go

output:
  max_lines: 80
`
	if out.String() != want {
		t.Errorf("Save() =\n%s\nwant\n%s", out.String(), want)
	}
}

func TestSaveInvalid(t *testing.T) {
	f := New(Spec{Version: CurrentVersion})
	var out bytes.Buffer
	if err := f.Save(&out); err == nil {
		t.Fatal("Save() error = nil, want a validation error")
	}
	if out.Len() > 0 {
		t.Errorf("Save() wrote %q for an invalid spec, want nothing", out.String())
	}
}
