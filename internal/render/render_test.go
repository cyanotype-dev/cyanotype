package render

import (
	"bytes"
	"errors"
	"flag"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/cyanotype-dev/cyanotype/internal/catalog"
	"github.com/cyanotype-dev/cyanotype/internal/designdoctest"
	"github.com/cyanotype-dev/cyanotype/internal/resolve"
	"github.com/cyanotype-dev/cyanotype/internal/spec"
)

var update = flag.Bool("update", false, "rewrite golden files with the current output")

// loadCatalog loads testdata/catalog: copies of real entries, frozen so that
// editing the real catalog's text does not change every golden file, plus
// test entries for topics the renderer does not know.
func loadCatalog(t *testing.T) *catalog.Catalog {
	t.Helper()
	c, err := catalog.LoadRoot(os.DirFS(filepath.Join("testdata", "catalog")))
	if err != nil {
		t.Fatalf("testdata/catalog is not a valid catalog:\n%v", err)
	}
	return c
}

// resolveSpec loads a spec from text and resolves it against the test catalog.
func resolveSpec(t *testing.T, text string) *resolve.Result {
	t.Helper()
	f, err := spec.Load(strings.NewReader(text))
	if err != nil {
		t.Fatalf("test spec does not load:\n%v", err)
	}
	r, _, err := resolve.Resolve(loadCatalog(t), f)
	if err != nil {
		t.Fatalf("test spec does not resolve:\n%v", err)
	}
	return r
}

// TestGolden renders every case under testdata/golden. A case folder holds
// cyanotype.yaml, an optional existing/ folder with the project's files
// before regeneration, an optional warnings.txt with one expected warning
// per line, and want/ with exactly the files Render must return.
func TestGolden(t *testing.T) {
	cases, err := os.ReadDir(filepath.Join("testdata", "golden"))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		dir := filepath.Join("testdata", "golden", c.Name())
		t.Run(c.Name(), func(t *testing.T) {
			text, err := os.ReadFile(filepath.Join(dir, "cyanotype.yaml")) //nolint:gosec // path built from testdata in test code
			if err != nil {
				t.Fatal(err)
			}
			checkGolden(t, dir, resolveSpec(t, string(text)))
		})
	}
}

// TestDesignExample renders the spec example of docs/DESIGN.md, so the
// documented example always produces the files in testdata/design-example.
func TestDesignExample(t *testing.T) {
	example := designdoctest.SpecExample(t, filepath.Join("..", ".."))
	checkGolden(t, filepath.Join("testdata", "design-example"), resolveSpec(t, example))
}

// checkGolden renders r with dir/existing as the project and compares the
// result with dir/want and dir/warnings.txt.
func checkGolden(t *testing.T, dir string, r *resolve.Result) {
	t.Helper()
	var existing fs.FS
	if _, err := os.Stat(filepath.Join(dir, "existing")); err == nil {
		existing = os.DirFS(filepath.Join(dir, "existing"))
	}
	files, warnings, err := Render(r, existing)
	if err != nil {
		t.Fatalf("Render() error: %v", err)
	}

	wantDir := filepath.Join(dir, "want")
	if *update {
		for _, f := range files {
			p := filepath.Join(wantDir, filepath.FromSlash(f.Path))
			if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, f.Content, 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}

	want := readTree(t, wantDir)
	var got []string
	for _, f := range files {
		got = append(got, f.Path)
		w, ok := want[f.Path]
		switch {
		case !ok:
			t.Errorf("Render() returned %s, which %s does not have (run go test ./internal/render -update)", f.Path, wantDir)
		case !bytes.Equal(f.Content, w):
			t.Errorf("%s differs from %s (run go test ./internal/render -update to rewrite it):\n%s",
				f.Path, filepath.Join(wantDir, filepath.FromSlash(f.Path)), f.Content)
		}
	}
	for p := range want {
		if !slices.Contains(got, p) {
			t.Errorf("%s has %s, which Render() did not return; delete it if that is intended", wantDir, p)
		}
	}

	var wantWarnings []string
	if data, err := os.ReadFile(filepath.Join(dir, "warnings.txt")); err == nil { //nolint:gosec // path built from testdata in test code
		wantWarnings = strings.Split(strings.TrimSpace(string(data)), "\n")
	}
	var gotWarnings []string
	for _, w := range warnings {
		gotWarnings = append(gotWarnings, w.String())
	}
	if !slices.Equal(gotWarnings, wantWarnings) {
		t.Errorf("warnings = %q\nwant %q", gotWarnings, wantWarnings)
	}
}

// readTree returns every file under dir keyed by its slash-separated path.
func readTree(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	out := make(map[string][]byte)
	fsys := os.DirFS(dir)
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(fsys, p)
		out[path.Clean(p)] = data
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// TestDeterministic renders the same input twice and expects the same bytes.
func TestDeterministic(t *testing.T) {
	r := resolveSpec(t, designdoctest.SpecExample(t, filepath.Join("..", "..")))
	first, _, err := Render(r, nil)
	if err != nil {
		t.Fatal(err)
	}
	for range 5 {
		again, _, err := Render(r, nil)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.EqualFunc(first, again, func(a, b File) bool { return a.Path == b.Path && bytes.Equal(a.Content, b.Content) }) {
			t.Fatal("two renders of the same input differ")
		}
	}
}

// TestRegenerateIsStable feeds Render its own output, protected blocks
// included, and expects the same files back: regenerating an unchanged
// project must never move or duplicate hand-written text.
func TestRegenerateIsStable(t *testing.T) {
	dir := filepath.Join("testdata", "golden", "custom-blocks")
	text, err := os.ReadFile(filepath.Join(dir, "cyanotype.yaml")) //nolint:gosec // path built from testdata in test code
	if err != nil {
		t.Fatal(err)
	}
	r := resolveSpec(t, string(text))
	first, _, err := Render(r, os.DirFS(filepath.Join(dir, "existing")))
	if err != nil {
		t.Fatal(err)
	}
	project := fstest.MapFS{}
	for _, f := range first {
		project[f.Path] = &fstest.MapFile{Data: f.Content}
	}
	second, _, err := Render(r, project)
	if err != nil {
		t.Fatal(err)
	}
	for i := range first {
		if !bytes.Equal(first[i].Content, second[i].Content) {
			t.Errorf("%s changed on regeneration:\nfirst:\n%s\nsecond:\n%s", first[i].Path, first[i].Content, second[i].Content)
		}
	}
}

// minimalSpec is a valid spec with only the required fields.
const minimalSpec = `version: 1
targets: [agents-md]
project:
  name: tiny
  goal: A tiny project for tests.
`

func TestRenderCustomBlocksWithCRLF(t *testing.T) {
	existing := fstest.MapFS{"AGENTS.md": &fstest.MapFile{Data: []byte(
		"# tiny\r\n\r\n## Project overview\r\n\r\nOld goal.\r\n\r\n" +
			"<!-- cyanotype:custom -->\r\nKeep me.\r\n<!-- /cyanotype:custom -->\r\n",
	)}}
	files, _, err := Render(resolveSpec(t, minimalSpec), existing)
	if err != nil {
		t.Fatal(err)
	}
	got := string(files[0].Content)
	want := "## Project overview\n\nA tiny project for tests.\n\n<!-- cyanotype:custom -->\nKeep me.\n<!-- /cyanotype:custom -->\n"
	if !strings.Contains(got, want) {
		t.Errorf("AGENTS.md =\n%s\nwant it to contain\n%s", got, want)
	}
	if strings.Contains(got, "\r") {
		t.Errorf("AGENTS.md contains a carriage return:\n%q", got)
	}
}

func TestRenderBadCustomBlocks(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "unclosed",
			content: "# tiny\n\n<!-- cyanotype:custom -->\ntext\n",
			want:    "render AGENTS.md: line 3: <!-- cyanotype:custom --> is never closed with <!-- /cyanotype:custom -->",
		},
		{
			name:    "nested",
			content: "<!-- cyanotype:custom -->\n<!-- cyanotype:custom -->\n<!-- /cyanotype:custom -->\n",
			want:    "render AGENTS.md: line 2: <!-- cyanotype:custom --> inside the block opened on line 1",
		},
		{
			name:    "stray end",
			content: "# tiny\n<!-- /cyanotype:custom -->\n",
			want:    "render AGENTS.md: line 2: <!-- /cyanotype:custom --> without a <!-- cyanotype:custom --> before it",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			existing := fstest.MapFS{"AGENTS.md": &fstest.MapFile{Data: []byte(tt.content)}}
			files, _, err := Render(resolveSpec(t, minimalSpec), existing)
			if err == nil || err.Error() != tt.want {
				t.Errorf("Render() = %d files, error %v\nwant error %q", len(files), err, tt.want)
			}
		})
	}
}

// failingFS fails every read with something other than fs.ErrNotExist.
type failingFS struct{}

func (failingFS) Open(string) (fs.File, error) { return nil, fs.ErrPermission }

func TestRenderReadError(t *testing.T) {
	_, _, err := Render(resolveSpec(t, minimalSpec), failingFS{})
	if !errors.Is(err, fs.ErrPermission) || !strings.Contains(err.Error(), "read existing AGENTS.md") {
		t.Errorf("Render() error = %v, want a wrapped permission error naming AGENTS.md", err)
	}
}

func TestParseCustomBlocksAnchors(t *testing.T) {
	content := `<!-- header -->

# tiny

<!-- cyanotype:custom -->
top
<!-- /cyanotype:custom -->

## Testing

- rule

<!-- cyanotype:custom -->
## Heading inside a block
<!-- /cyanotype:custom -->

### Not a section

  <!-- cyanotype:custom -->
indented markers
  <!-- /cyanotype:custom -->
`
	blocks, err := parseCustomBlocks([]byte(content))
	if err != nil {
		t.Fatal(err)
	}
	var anchors []string
	for _, b := range blocks {
		anchors = append(anchors, b.anchor)
	}
	// The heading inside the second block must not anchor the third.
	if want := []string{"", "Testing", "Testing"}; !slices.Equal(anchors, want) {
		t.Errorf("anchors = %q, want %q", anchors, want)
	}
	if got := blocks[2].lines[0]; got != "  <!-- cyanotype:custom -->" {
		t.Errorf("block lines are not kept verbatim: first line %q", got)
	}
}

func TestLanguagePolicy(t *testing.T) {
	tests := []struct {
		name string
		in   spec.Languages
		want []string
	}{
		{
			name: "all english",
			in:   spec.Languages{Code: "english", Comments: "english", Docs: "english", Commits: "english", AgentReplies: "english"},
			want: []string{"- Code, comments, docs and commit messages: English.", "- Replies to the user: English."},
		},
		{
			name: "mixed, grouped in order of first appearance",
			in:   spec.Languages{Code: "english", Comments: "italian", Docs: "english", Commits: "italian", AgentReplies: "brazilian-portuguese"},
			want: []string{
				"- Code and docs: English.",
				"- Comments and commit messages: Italian.",
				"- Replies to the user: Brazilian Portuguese.",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := languagePolicy(tt.in); !slices.Equal(got, tt.want) {
				t.Errorf("languagePolicy() =\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

func TestCodeSpan(t *testing.T) {
	tests := []struct{ in, want string }{
		{"pnpm dev", "`pnpm dev`"},
		{"echo `date`", "`` echo `date` ``"},
		{"a ``b`` c", "``` a ``b`` c ```"},
	}
	for _, tt := range tests {
		if got := codeSpan(tt.in); got != tt.want {
			t.Errorf("codeSpan(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestOneLine(t *testing.T) {
	if got, want := oneLine("  First line.\n\n  Second line.  \n"), "First line. Second line."; got != want {
		t.Errorf("oneLine() = %q, want %q", got, want)
	}
}
