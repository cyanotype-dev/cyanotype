package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cyanotype-dev/cyanotype/internal/contract"
)

var update = flag.Bool("update", false, "rewrite the golden files of the JSON output")

// jsonProject is a small generated project for the JSON golden files: one
// target and one entry keep the diffs short.
func jsonProject(t *testing.T) string {
	t.Helper()
	dir := newProject(t, map[string]string{"cyanotype.yaml": minimalSpec})
	if r := runCLI(t, nonInteractive, failReader{t}, "generate", "--dir", dir, "--yes"); r.code != codeOK {
		t.Fatalf("generate: exit code %d\n%s", r.code, r.stderr)
	}
	return dir
}

// noProject is the setup of a command that reads no project folder.
func noProject(*testing.T) string { return "" }

// writeFile writes content to the slash-separated path name under dir.
func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestJSONGolden runs commands with --json and compares stdout with the
// golden files in testdata/json, which document the contract by example.
// Every case also checks what the contract promises: stdout holds one JSON
// value and nothing else goes to stderr, and the exit code is the one the
// command has without --json.
func TestJSONGolden(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T) string // returns the project folder
		args  []string
		code  int
		noDir bool // the command takes no --dir
	}{
		{
			name:  "catalog-search",
			setup: noProject,
			args:  []string{"catalog", "search", "package", "manager"},
			code:  codeOK,
			noDir: true,
		},
		{
			name:  "catalog-search-stack",
			setup: noProject,
			args:  []string{"catalog", "search", "--category", "ui-components", "--stack", "tailwind,nextjs"},
			code:  codeOK,
			noDir: true,
		},
		{
			name:  "catalog-search-nothing",
			setup: noProject,
			args:  []string{"catalog", "search", "kubernetes"},
			code:  codeOK,
			noDir: true,
		},
		{
			name: "spec",
			setup: func(t *testing.T) string {
				return newProject(t, map[string]string{"cyanotype.yaml": minimalSpec + "commands:\n  test: go test ./...\n"})
			},
			args: []string{"spec"},
			code: codeOK,
		},
		{
			name: "spec-errors",
			setup: func(t *testing.T) string {
				return newProject(t, map[string]string{"cyanotype.yaml": minimalSpec + "tags: [x]\n"})
			},
			args: []string{"spec"},
			code: codeError,
		},
		{
			name:  "spec-no-spec",
			setup: func(t *testing.T) string { return newProject(t, nil) },
			args:  []string{"spec"},
			code:  codeError,
		},
		{
			name: "detect",
			setup: func(t *testing.T) string {
				return newProject(t, map[string]string{
					"package.json":      `{"dependencies": {"next": "16.0.0"}, "scripts": {"dev": "next dev", "build": "next build"}}`,
					"package-lock.json": "{}",
					"pnpm-lock.yaml":    "lockfileVersion: '9.0'\n",
				})
			},
			args: []string{"detect"},
			code: codeOK,
		},
		{
			name: "detect-unreadable",
			setup: func(t *testing.T) string {
				return newProject(t, map[string]string{"package.json": "{not json", "pyproject.toml": "[project]\nname = \"x\"\n"})
			},
			args: []string{"detect"},
			code: codeOK,
		},
		{
			name:  "detect-nothing",
			setup: func(t *testing.T) string { return newProject(t, nil) },
			args:  []string{"detect"},
			code:  codeOK,
		},
		{
			name:  "validate",
			setup: jsonProject,
			args:  []string{"validate"},
			code:  codeOK,
		},
		{
			name: "validate-errors",
			setup: func(t *testing.T) string {
				return newProject(t, map[string]string{"cyanotype.yaml": strings.Replace(minimalSpec,
					"[conventional-commits]", "[conventional-commits, pnpm]", 1)})
			},
			args: []string{"validate"},
			code: codeError,
		},
		{
			name: "validate-markers",
			setup: func(t *testing.T) string {
				dir := jsonProject(t)
				writeFile(t, dir, "AGENTS.md", readFile(t, dir, "AGENTS.md")+"<!-- /cyanotype:custom -->\n")
				return dir
			},
			args: []string{"validate"},
			code: codeError,
		},
		{
			name: "select-dry-run",
			setup: func(t *testing.T) string {
				return newProject(t, map[string]string{"cyanotype.yaml": minimalSpec + "# a comment Save keeps\n"})
			},
			args: []string{"select", "--add", "small-commits,branch-per-task", "--remove", "go", "--dry-run"},
			code: codeNotWritten,
		},
		{
			name: "select-yes",
			setup: func(t *testing.T) string {
				return newProject(t, map[string]string{"cyanotype.yaml": minimalSpec})
			},
			args: []string{"select", "--remove", "conventional-commits", "--add", "small-commits", "--yes"},
			code: codeOK,
		},
		{
			name: "select-nothing-to-change",
			setup: func(t *testing.T) string {
				return newProject(t, map[string]string{"cyanotype.yaml": minimalSpec})
			},
			args: []string{"select", "--add", "conventional-commits"},
			code: codeOK,
		},
		{
			name: "select-refused",
			setup: func(t *testing.T) string {
				return newProject(t, map[string]string{"cyanotype.yaml": minimalSpec})
			},
			args: []string{"select", "--add", "pnpm,pnpn"},
			code: codeError,
		},
		{
			name: "select-locked",
			setup: func(t *testing.T) string {
				return newProject(t, map[string]string{"cyanotype.yaml": minimalSpec + "locked: [conventional-commits]\n"})
			},
			args: []string{"select", "--remove", "conventional-commits", "--yes"},
			code: codeError,
		},
		{
			name:  "select-usage",
			setup: func(t *testing.T) string { return newProject(t, map[string]string{"cyanotype.yaml": minimalSpec}) },
			args:  []string{"select"},
			code:  codeUsage,
		},
		{
			name:  "validate-no-spec",
			setup: func(t *testing.T) string { return newProject(t, nil) },
			args:  []string{"validate"},
			code:  codeError,
		},
		{
			name:  "generate-dry-run",
			setup: func(t *testing.T) string { return newProject(t, map[string]string{"cyanotype.yaml": minimalSpec}) },
			args:  []string{"generate", "--dry-run"},
			code:  codeNotWritten,
		},
		{
			name:  "generate-yes",
			setup: func(t *testing.T) string { return newProject(t, map[string]string{"cyanotype.yaml": minimalSpec}) },
			args:  []string{"generate", "--yes"},
			code:  codeOK,
		},
		{
			name:  "generate-no-terminal",
			setup: func(t *testing.T) string { return newProject(t, map[string]string{"cyanotype.yaml": minimalSpec}) },
			args:  []string{"generate"},
			code:  codeNotWritten,
		},
		{
			name:  "generate-up-to-date",
			setup: jsonProject,
			args:  []string{"generate"},
			code:  codeOK,
		},
		{
			// AGENTS.md leaves the targets: it is stale and deleted, and
			// CLAUDE.md is created with the content AGENTS.md had.
			name: "generate-changes",
			setup: func(t *testing.T) string {
				dir := jsonProject(t)
				writeFile(t, dir, "cyanotype.yaml", strings.Replace(minimalSpec, "[agents-md]", "[claude]", 1))
				return dir
			},
			args: []string{"generate", "--dry-run"},
			code: codeNotWritten,
		},
		{
			name: "generate-hand-written-dry-run",
			setup: func(t *testing.T) string {
				return newProject(t, map[string]string{"cyanotype.yaml": minimalSpec, "AGENTS.md": "# Our rules\n"})
			},
			args: []string{"generate", "--dry-run"},
			code: codeNotWritten,
		},
		{
			name: "generate-hand-written-no-choice",
			setup: func(t *testing.T) string {
				return newProject(t, map[string]string{"cyanotype.yaml": minimalSpec, "AGENTS.md": "# Our rules\n"})
			},
			args: []string{"generate", "--yes"},
			code: codeUsage,
		},
		{
			name: "generate-hand-written-keep",
			setup: func(t *testing.T) string {
				return newProject(t, map[string]string{"cyanotype.yaml": minimalSpec, "AGENTS.md": "# Our rules\n"})
			},
			args: []string{"generate", "--dry-run", "--existing", "keep"},
			code: codeNotWritten,
		},
		{
			name: "generate-hand-written-move",
			setup: func(t *testing.T) string {
				return newProject(t, map[string]string{"cyanotype.yaml": minimalSpec, "AGENTS.md": "# Our rules\n"})
			},
			args: []string{"generate", "--yes", "--existing", "move"},
			code: codeOK,
		},
		{
			name: "generate-spec-errors",
			setup: func(t *testing.T) string {
				return newProject(t, map[string]string{"cyanotype.yaml": "version: 1\ntargets: [claude, vscode]\nproject:\n  name: tiny\n"})
			},
			args: []string{"generate"},
			code: codeError,
		},
		{
			name: "generate-resolve-errors",
			setup: func(t *testing.T) string {
				return newProject(t, map[string]string{"cyanotype.yaml": strings.Replace(minimalSpec,
					"[conventional-commits]", "[conventional-commits, pnpn, pnpm]", 1)})
			},
			args: []string{"generate"},
			code: codeError,
		},
		{
			name: "generate-syntax-error",
			setup: func(t *testing.T) string {
				return newProject(t, map[string]string{"cyanotype.yaml": "version: 1\ntargets: [claude\n"})
			},
			args: []string{"generate"},
			code: codeError,
		},
		{
			name: "generate-markers",
			setup: func(t *testing.T) string {
				dir := jsonProject(t)
				writeFile(t, dir, "AGENTS.md", strings.Replace(readFile(t, dir, "AGENTS.md"), "\n## ", "\n<!-- cyanotype:custom -->\n## ", 1))
				return dir
			},
			args: []string{"generate"},
			code: codeError,
		},
		{
			name:  "generate-no-spec",
			setup: func(t *testing.T) string { return newProject(t, nil) },
			args:  []string{"generate"},
			code:  codeError,
		},
		{
			name:  "generate-usage",
			setup: func(t *testing.T) string { return newProject(t, nil) },
			args:  []string{"generate", "--dry-run", "--existing", "overwrite"},
			code:  codeUsage,
		},
		{
			name:  "check-clean",
			setup: jsonProject,
			args:  []string{"check"},
			code:  codeOK,
		},
		{
			name: "check-drift",
			setup: func(t *testing.T) string {
				dir := jsonProject(t)
				writeFile(t, dir, "AGENTS.md", readFile(t, dir, "AGENTS.md")+"- A rule added by hand.\n")
				writeFile(t, dir, "go.mod", "module example.com/tiny\n\ngo 1.27\n")
				return dir
			},
			args: []string{"check"},
			code: codeDrift,
		},
		{
			name: "check-warnings",
			setup: func(t *testing.T) string {
				dir := jsonProject(t)
				writeFile(t, dir, "go.mod", "module example.com/tiny\n\ngo 1.27\n")
				return dir
			},
			args: []string{"check"},
			code: codeOK,
		},
		{
			name: "check-warnings-strict",
			setup: func(t *testing.T) string {
				dir := jsonProject(t)
				writeFile(t, dir, "go.mod", "module example.com/tiny\n\ngo 1.27\n")
				return dir
			},
			args: []string{"check", "--strict"},
			code: codeDrift,
		},
		{
			name: "check-cannot-run",
			setup: func(t *testing.T) string {
				dir := jsonProject(t)
				writeFile(t, dir, "cyanotype.lock", "version: 1\nfiles: [oops]\n")
				return dir
			},
			args: []string{"check"},
			code: codeError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := tt.setup(t)
			args := append(append([]string{}, tt.args...), "--json")
			if !tt.noDir {
				args = append(args, "--dir", dir)
			}
			// Even on a terminal, --json never asks or reads stdin.
			r := runCLI(t, interactive, failReader{t}, args...)
			if r.code != tt.code || r.err != nil {
				t.Errorf("exit code %d, error %v; want code %d", r.code, r.err, tt.code)
			}
			if r.stderr != "" {
				t.Errorf("stderr is not empty:\n%s", r.stderr)
			}
			assertOneJSONValue(t, r.stdout)
			got := r.stdout
			if dir != "" {
				got = normalizeDir(got, dir)
			}
			compareGolden(t, tt.name, got)
		})
	}
}

// TestJSONImport checks that generate --json --existing import returns the
// prompt that generate --existing import prints, and writes nothing.
func TestJSONImport(t *testing.T) {
	files := map[string]string{"cyanotype.yaml": minimalSpec, "AGENTS.md": "# Our rules\n"}
	dir := newProject(t, files)
	text := runCLI(t, nonInteractive, failReader{t}, "generate", "--dir", dir, "--existing", "import")
	r := runCLI(t, nonInteractive, failReader{t}, "generate", "--dir", dir, "--existing", "import", "--json")
	if r.code != codeOK || r.stderr != "" {
		t.Fatalf("exit code %d, stderr:\n%s", r.code, r.stderr)
	}
	var out contract.Generate
	if err := json.Unmarshal([]byte(r.stdout), &out); err != nil {
		t.Fatal(err)
	}
	if out.Import == nil || out.Import.Prompt != text.stdout || len(out.Import.Files) != 1 || out.Import.Files[0] != "AGENTS.md" {
		t.Errorf("import = %+v, want the prompt generate prints for AGENTS.md", out.Import)
	}
	if got := tree(t, dir); len(got) != len(files) {
		t.Errorf("files after import: %q", keys(got))
	}
}

func readFile(t *testing.T, dir, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name))) //nolint:gosec // test temp dir
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// assertOneJSONValue fails the test unless s holds exactly one JSON value.
func assertOneJSONValue(t *testing.T, s string) {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(s))
	var v map[string]any
	if err := dec.Decode(&v); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, s)
	}
	if dec.More() {
		t.Errorf("stdout holds more than one JSON value:\n%s", s)
	}
}

// normalizeDir replaces the project folder in s with <dir>, so golden files
// do not depend on where the test ran.
func normalizeDir(s, dir string) string {
	quoted, _ := json.Marshal(dir)
	return strings.ReplaceAll(s, strings.Trim(string(quoted), `"`), "<dir>")
}

// compareGolden compares got with testdata/json/<name>.json, or rewrites
// the file with -update.
func compareGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", "json", name+".json")
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path) //nolint:gosec // path is built from a test case name
	if err != nil {
		t.Fatalf("%v (run go test ./cmd/cyanotype -run TestJSON -update to create it)", err)
	}
	if !bytes.Equal(bytes.ReplaceAll(want, []byte("\r\n"), []byte("\n")), []byte(got)) {
		t.Errorf("stdout differs from %s (run go test ./cmd/cyanotype -run TestJSON -update to rewrite it):\n%s", path, got)
	}
}
