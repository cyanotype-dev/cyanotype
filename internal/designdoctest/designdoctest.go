// Package designdoctest reads examples out of docs/DESIGN.md, so tests check
// the documented examples themselves rather than copies that drift apart
// from them. It is meant to be used from tests only, like net/http/httptest.
package designdoctest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// SpecExample returns the first YAML block of the Project spec section of
// docs/DESIGN.md: the example cyanotype.yaml. root is the path of the
// repository root from the calling test's package folder, such as "../..".
// Any problem finding the block fails the test.
func SpecExample(t testing.TB, root string) string {
	t.Helper()
	return firstYAMLBlock(t, root, "Project spec")
}

// firstYAMLBlock returns the content of the first ```yaml block after the
// "## <section>" heading of docs/DESIGN.md.
func firstYAMLBlock(t testing.TB, root, section string) string {
	t.Helper()
	path := filepath.Join(root, "docs", "DESIGN.md")
	data, err := os.ReadFile(path) //nolint:gosec // path is built from a constant in test code, never user input
	if err != nil {
		t.Fatal(err)
	}
	doc := strings.ReplaceAll(string(data), "\r\n", "\n")

	_, body, ok := strings.Cut(doc, "\n## "+section+"\n")
	if !ok {
		t.Fatalf("docs/DESIGN.md has no %q section", "## "+section)
	}
	_, block, ok := strings.Cut(body, "```yaml\n")
	if !ok {
		t.Fatalf("the %s section of docs/DESIGN.md has no yaml block", section)
	}
	block, _, ok = strings.Cut(block, "```\n")
	if !ok {
		t.Fatalf("the yaml block in the %s section of docs/DESIGN.md is not closed", section)
	}
	return block
}
