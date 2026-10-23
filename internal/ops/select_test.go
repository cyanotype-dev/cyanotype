package ops

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	bundled "github.com/cyanotype-dev/cyanotype/catalog"
	"github.com/cyanotype-dev/cyanotype/internal/catalog"
	"github.com/cyanotype-dev/cyanotype/internal/contract"
	"github.com/cyanotype-dev/cyanotype/internal/generate"
)

// project writes spec as cyanotype.yaml in a new folder and opens it.
func project(t *testing.T, spec string) (string, *os.Root) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, generate.SpecFile), []byte(spec), 0o600); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	return dir, root
}

func loadCatalog(t *testing.T) *catalog.Catalog {
	t.Helper()
	c, err := catalog.LoadRoot(bundled.FS())
	if err != nil {
		t.Fatal(err)
	}
	return c
}

const simpleSpec = `# Written by cyanotype init.
version: 1

targets: [claude]

mode: simple

project:
  name: tiny
  goal: A tiny project for tests.

selections: [go] # the stack
`

// TestSelectLeavesSimpleMode checks that a change of selections keeps the
// file's comments and moves it to advanced mode, so simple mode never
// replaces entries chosen one by one.
func TestSelectLeavesSimpleMode(t *testing.T) {
	dir, root := project(t, simpleSpec)
	out, ch := Select(loadCatalog(t), root, []string{"golangci-lint"}, nil)
	if ch == nil {
		t.Fatalf("Select() returned no change: %+v", out.Errors)
	}
	out = WriteSelect(root, ch, out)
	if len(out.Errors) > 0 || out.Written == nil || !*out.Written {
		t.Fatalf("WriteSelect() = %+v", out)
	}
	data, err := os.ReadFile(filepath.Join(dir, generate.SpecFile)) //nolint:gosec // test temp dir
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# Written by cyanotype init.\n", "mode: advanced\n", "selections: [go, golangci-lint] # the stack\n"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("cyanotype.yaml lacks %q:\n%s", want, data)
		}
	}
}

// TestSelectRefusesChangedSpec edits the spec between Select and
// WriteSelect, as a user could while reading the diff.
func TestSelectRefusesChangedSpec(t *testing.T) {
	dir, root := project(t, simpleSpec)
	out, ch := Select(loadCatalog(t), root, []string{"golangci-lint"}, nil)
	if ch == nil {
		t.Fatalf("Select() returned no change: %+v", out.Errors)
	}
	edited := simpleSpec + "# my note\n"
	if err := os.WriteFile(filepath.Join(dir, generate.SpecFile), []byte(edited), 0o600); err != nil {
		t.Fatal(err)
	}
	out = WriteSelect(root, ch, out)
	if len(out.Errors) != 1 || out.Errors[0].Code != contract.CodeChangedSincePreview {
		t.Errorf("WriteSelect() errors = %+v, want changed-since-preview", out.Errors)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, generate.SpecFile)); string(data) != edited { //nolint:gosec // test temp dir
		t.Errorf("cyanotype.yaml was changed to:\n%s", data)
	}
}

func TestSelectUsage(t *testing.T) {
	_, root := project(t, simpleSpec)
	tests := []struct {
		name        string
		add, remove []string
	}{
		{"nothing", nil, nil},
		{"both added and removed", []string{"go"}, []string{"go"}},
	}
	for _, tt := range tests {
		out, ch := Select(loadCatalog(t), root, tt.add, tt.remove)
		if ch != nil || len(out.Errors) != 1 || out.Errors[0].Code != contract.CodeUsage {
			t.Errorf("%s: Select() = %+v, want a usage error", tt.name, out.Errors)
		}
	}
}
