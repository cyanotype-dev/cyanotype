package generate

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	bundled "github.com/cyanotype-dev/cyanotype/catalog"
	"github.com/cyanotype-dev/cyanotype/internal/catalog"
	"github.com/cyanotype-dev/cyanotype/internal/lock"
	"github.com/cyanotype-dev/cyanotype/internal/spec"
)

// testSpec targets AGENTS.md and a target without a renderer, so Preview
// returns AGENTS.md, .gitignore and a warning on line 2.
const testSpec = `version: 1
targets: [agents-md, cursor]
project:
  name: tiny
  goal: A tiny project for tests.
selections: [conventional-commits]
`

func loadCatalog(t *testing.T) *catalog.Catalog {
	t.Helper()
	c, err := catalog.LoadRoot(bundled.FS())
	if err != nil {
		t.Fatalf("the bundled catalog is invalid:\n%v", err)
	}
	return c
}

func loadSpec(t *testing.T, text string) *spec.File {
	t.Helper()
	f, err := spec.Load(strings.NewReader(text))
	if err != nil {
		t.Fatalf("test spec does not load:\n%v", err)
	}
	return f
}

// project creates a project folder holding files, given by slash-separated
// path, and opens it as a root.
func project(t *testing.T, files map[string]string) (string, *os.Root) {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	return dir, root
}

func preview(t *testing.T, root *os.Root) []Change {
	t.Helper()
	changes, _, err := Preview(loadCatalog(t), loadSpec(t, testSpec), root.FS())
	if err != nil {
		t.Fatalf("Preview() error:\n%v", err)
	}
	return changes
}

func statuses(changes []Change) map[string]Status {
	out := make(map[string]Status)
	for _, c := range changes {
		out[c.Path] = c.Status
	}
	return out
}

func TestLoadSpec(t *testing.T) {
	_, root := project(t, nil)
	if _, err := LoadSpec(root); !errors.Is(err, ErrNoSpec) {
		t.Errorf("LoadSpec() on an empty folder error = %v, want ErrNoSpec", err)
	}

	_, root = project(t, map[string]string{SpecFile: testSpec})
	f, err := LoadSpec(root)
	if err != nil {
		t.Fatalf("LoadSpec() error:\n%v", err)
	}
	if f.Spec.Project.Name != "tiny" {
		t.Errorf("project.name = %q, want %q", f.Spec.Project.Name, "tiny")
	}
}

func TestPreviewWarnings(t *testing.T) {
	_, root := project(t, nil)
	_, warnings, err := Preview(loadCatalog(t), loadSpec(t, testSpec), root.FS())
	if err != nil {
		t.Fatalf("Preview() error:\n%v", err)
	}
	if len(warnings) != 1 || warnings[0].Field != "targets[1]" || warnings[0].Line != 2 {
		t.Errorf("warnings = %+v, want one on targets[1], line 2", warnings)
	}
}

func TestPreviewErrors(t *testing.T) {
	_, root := project(t, nil)
	text := strings.Replace(testSpec, "conventional-commits", "conventional-comits", 1)
	changes, _, err := Preview(loadCatalog(t), loadSpec(t, text), root.FS())
	var ve *spec.ValidationError
	if !errors.As(err, &ve) || ve.Line != 6 || !strings.Contains(ve.Msg, `did you mean "conventional-commits"`) {
		t.Errorf("Preview() error = %v, want an unknown entry on line 6", err)
	}
	if changes != nil {
		t.Errorf("Preview() returned changes together with an error")
	}
}

// TestPreviewAndWrite runs the cycle the CLI runs: preview an empty project,
// write it, then preview again.
func TestPreviewAndWrite(t *testing.T) {
	dir, root := project(t, nil)
	changes := preview(t, root)
	want := map[string]Status{".gitignore": Created, "AGENTS.md": Created, lock.Path: Created}
	if got := statuses(changes); !mapsEqual(got, want) {
		t.Fatalf("first preview = %v, want %v", got, want)
	}
	for _, c := range changes {
		if d := c.Diff(); !strings.HasPrefix(d, "--- /dev/null\n+++ b/"+c.Path+"\n") {
			t.Errorf("%s diff does not show a new file:\n%s", c.Path, d)
		}
	}

	written, err := Write(root, changes)
	if err != nil {
		t.Fatalf("Write() error: %v", err)
	}
	// The lock is written first; see Write.
	if !slices.Equal(written, []string{lock.Path, ".gitignore", "AGENTS.md"}) {
		t.Errorf("Write() wrote %q", written)
	}
	checkLockRecords(t, changes)
	for _, c := range changes {
		got, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(c.Path))) //nolint:gosec // test temp dir
		if err != nil || !bytes.Equal(got, c.New) {
			t.Errorf("%s on disk differs from the generated content (error %v)", c.Path, err)
		}
	}
	assertNoTempFiles(t, dir)

	again := preview(t, root)
	want = map[string]Status{".gitignore": Unchanged, "AGENTS.md": Unchanged, lock.Path: Unchanged}
	if got := statuses(again); !mapsEqual(got, want) {
		t.Errorf("second preview = %v, want %v", got, want)
	}
	for _, c := range again {
		if c.Diff() != "" {
			t.Errorf("%s is unchanged but has a diff:\n%s", c.Path, c.Diff())
		}
	}
}

// TestPreviewCRLF checks that line endings alone never make a file
// modified, and that a real change in a CRLF file is reported with only its
// changed lines in the diff.
func TestPreviewCRLF(t *testing.T) {
	_, root := project(t, nil)
	var agents []byte
	for _, c := range preview(t, root) {
		if c.Path == "AGENTS.md" {
			agents = c.New
		}
	}
	crlf := bytes.ReplaceAll(agents, []byte("\n"), []byte("\r\n"))

	dir, root := project(t, map[string]string{"AGENTS.md": string(crlf)})
	c := find(t, preview(t, root), "AGENTS.md")
	if c.Status != Unchanged {
		t.Errorf("AGENTS.md with CRLF endings has status %v, want unchanged", c.Status)
	}
	if _, err := Write(root, []Change{c}); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md")); !bytes.Equal(got, crlf) { //nolint:gosec // test temp dir
		t.Error("an unchanged file was rewritten")
	}

	edited := bytes.Replace(crlf, []byte("# tiny\r\n"), []byte("# old name\r\n"), 1)
	_, root = project(t, map[string]string{"AGENTS.md": string(edited)})
	c = find(t, preview(t, root), "AGENTS.md")
	if c.Status != Modified || !c.CRLF {
		t.Errorf("edited CRLF AGENTS.md: status %v, CRLF %v; want modified, true", c.Status, c.CRLF)
	}
	if d := c.Diff(); strings.Count(d, "\n-") != 1 || !strings.Contains(d, "\n-# old name\n+# tiny\n") {
		t.Errorf("diff should change only the title:\n%s", d)
	}
}

func TestWriteRefusesUnsafePaths(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{"parent folder", "../evil.md", "not a path inside the project root"},
		{"nested parent", "docs/../../evil.md", "not a path inside the project root"},
		{"absolute", "/evil.md", "not a path inside the project root"},
		{"backslashes", `..\evil.md`, "not a path inside the project root"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir, root := project(t, nil)
			changes := []Change{
				{Path: "AGENTS.md", Status: Created, New: []byte("ok\n")},
				{Path: tt.path, Status: Created, New: []byte("evil\n")},
			}
			written, err := Write(root, changes)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Write() error = %v, want one containing %q", err, tt.want)
			}
			if written != nil {
				t.Errorf("Write() reports %q written", written)
			}
			assertEmpty(t, dir)
			if _, err := os.Stat(filepath.Join(filepath.Dir(dir), "evil.md")); err == nil {
				t.Error("evil.md was written outside the project root")
			}
		})
	}
}

func TestWriteRefusesSymlinks(t *testing.T) {
	outside := t.TempDir()
	dir, root := project(t, map[string]string{"AGENTS.md": "shared\n"})
	symlink(t, "AGENTS.md", filepath.Join(dir, "CLAUDE.md"))
	symlink(t, outside, filepath.Join(dir, "out"))

	tests := []struct {
		name string
		path string
		want string
	}{
		{"link as target", "CLAUDE.md", "it is a symbolic link"},
		{"link to a folder outside the root", "out/evil.md", "check out/evil.md"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Write(root, []Change{{Path: tt.path, Status: Modified, New: []byte("evil\n")}})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Write() error = %v, want one containing %q", err, tt.want)
			}
		})
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md")); string(got) != "shared\n" { //nolint:gosec // test temp dir
		t.Errorf("AGENTS.md was changed through the link: %q", got)
	}
	assertEmpty(t, outside)
}

func TestWriteRefusesFolders(t *testing.T) {
	_, root := project(t, map[string]string{"AGENTS.md/keep": "x"})
	_, err := Write(root, []Change{{Path: "AGENTS.md", Status: Modified, New: []byte("x\n")}})
	if err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Errorf("Write() error = %v, want a refusal to replace a folder", err)
	}
}

func TestWriteCreatesFolders(t *testing.T) {
	dir, root := project(t, nil)
	changes := []Change{{Path: "docs/agents/testing.md", Status: Created, New: []byte("# Testing\n")}}
	if _, err := Write(root, changes); err != nil {
		t.Fatalf("Write() error: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "docs", "agents", "testing.md")) //nolint:gosec // test temp dir
	if err != nil || string(got) != "# Testing\n" {
		t.Errorf("docs/agents/testing.md = %q, %v", got, err)
	}
	assertNoTempFiles(t, dir)
}

func TestWriteKeepsPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no Unix permission bits")
	}
	dir, root := project(t, map[string]string{"AGENTS.md": "old\n"})
	p := filepath.Join(dir, "AGENTS.md")
	if err := os.Chmod(p, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Write(root, []Change{{Path: "AGENTS.md", Status: Modified, New: []byte("new\n")}}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(p)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("AGENTS.md mode = %v, %v; want 0600", info.Mode(), err)
	}
}

func find(t *testing.T, changes []Change, path string) Change {
	t.Helper()
	for _, c := range changes {
		if c.Path == path {
			return c
		}
	}
	t.Fatalf("no change for %s", path)
	return Change{}
}

// symlink creates a symbolic link, skipping the test where the system does
// not allow it, such as Windows without Developer Mode.
func symlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("cannot create symbolic links here: %v", err)
	}
}

func assertEmpty(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) > 0 {
		t.Errorf("%s holds %d files, want none", dir, len(entries))
	}
}

func assertNoTempFiles(t *testing.T, dir string) {
	t.Helper()
	err := filepath.WalkDir(dir, func(p string, _ os.DirEntry, err error) error {
		if strings.HasSuffix(p, ".tmp") {
			t.Errorf("temporary file left behind: %s", p)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func mapsEqual(a, b map[string]Status) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
