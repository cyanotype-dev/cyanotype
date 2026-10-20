package adopt

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	bundled "github.com/cyanotype-dev/cyanotype/catalog"
	"github.com/cyanotype-dev/cyanotype/internal/catalog"
	"github.com/cyanotype-dev/cyanotype/internal/detect"
	"github.com/cyanotype-dev/cyanotype/internal/resolve"
	"github.com/cyanotype-dev/cyanotype/internal/spec"
)

var update = flag.Bool("update", false, "rewrite golden files with the current output")

func testCatalog(t *testing.T) *catalog.Catalog {
	t.Helper()
	c, err := catalog.LoadRoot(os.DirFS(filepath.Join("testdata", "catalog")))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// golden compares got with testdata/<name>, or rewrites it with -update.
func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.WriteFile(path, []byte(got), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path) //nolint:gosec // a fixed test file
	if err != nil {
		t.Fatal(err)
	}
	if got != string(bytes.ReplaceAll(want, []byte("\r\n"), []byte("\n"))) {
		t.Errorf("prompt differs from %s; run go test ./internal/adopt -update and review the diff.\ngot:\n%s", path, got)
	}
}

// TestPromptInit is the prompt init prints: no spec yet, a draft from
// detection, and a clash to settle.
func TestPromptInit(t *testing.T) {
	c := testCatalog(t)
	found := detect.Result{
		Findings: []detect.Finding{
			{ID: "nextjs", Evidence: []detect.Evidence{{File: "package.json", Dependency: "next"}}},
			{ID: "node", Evidence: []detect.Evidence{{File: "package.json"}}},
			{ID: "npm", Evidence: []detect.Evidence{{File: "package-lock.json"}}},
			{ID: "pnpm", Evidence: []detect.Evidence{{File: "pnpm-lock.yaml"}}},
		},
		Clashes: []detect.Clash{{IDs: []string{"npm", "pnpm"}, Category: "package-manager"}},
	}
	draft := "version: 1\n\ncatalog_version: \"2026.10\"\n\ntargets:\n  - claude\n\nproject:\n  name: shop\n  goal: \"\"\n\nselections:\n  - node\n  - npm\n  - pnpm\n  - nextjs\n"
	got := Prompt(c, PromptInput{Files: []string{"CLAUDE.md"}, Draft: []byte(draft), Found: found, Stack: found.IDs()})
	golden(t, "prompt-init.md", got)
}

// TestPromptGenerate is the prompt generate prints: a spec exists, two
// files to convert, and nothing detected, so every entry is listed.
func TestPromptGenerate(t *testing.T) {
	c := testCatalog(t)
	existing := "version: 1\r\ntargets: [claude, agents-md]\r\nproject:\r\n  name: shop\r\n  goal: An online shop.\r\n"
	got := Prompt(c, PromptInput{Files: []string{"AGENTS.md", "CLAUDE.md"}, Spec: []byte(existing)})
	golden(t, "prompt-generate.md", got)
	if !strings.Contains(got, "`fastapi`") {
		t.Error("with nothing detected, every entry should be listed")
	}
}

// TestPromptLists checks which entries the prompt offers: only those whose
// requirements the stack meets.
func TestPromptLists(t *testing.T) {
	got := Prompt(testCatalog(t), PromptInput{Files: []string{"CLAUDE.md"}, Stack: []string{"node", "pnpm"}})
	for _, id := range []string{"`node`", "`pnpm`", "`npm`", "`nextjs`", "`python`", "`no-test-tampering`"} {
		if !strings.Contains(got, id) {
			t.Errorf("prompt does not list %s", id)
		}
	}
	if strings.Contains(got, "`fastapi`") {
		t.Error("prompt lists fastapi, whose requirement python is not in the stack")
	}
}

// TestSpecExample checks that the example in the prompt is a valid spec
// for the bundled catalog, so the agent is never taught a wrong format.
func TestSpecExample(t *testing.T) {
	c, err := catalog.LoadRoot(bundled.FS())
	if err != nil {
		t.Fatal(err)
	}
	f, err := spec.Load(strings.NewReader(fmt.Sprintf(specExample, c.Version())))
	if err != nil {
		t.Fatalf("the example does not load:\n%v", err)
	}
	if _, _, err := resolve.Resolve(c, f); err != nil {
		t.Fatalf("the example does not resolve against the bundled catalog:\n%v", err)
	}
}
