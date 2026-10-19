package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	bundled "github.com/cyanotype-dev/cyanotype/catalog"
	"github.com/cyanotype-dev/cyanotype/internal/catalog"
	"github.com/cyanotype-dev/cyanotype/internal/designdoctest"
	"github.com/cyanotype-dev/cyanotype/internal/render"
	"github.com/cyanotype-dev/cyanotype/internal/resolve"
	"github.com/cyanotype-dev/cyanotype/internal/spec"
)

// result is what one run of the CLI produced.
type result struct {
	stdout, stderr string
	code           int
	err            error // what main would print, when it prints anything
}

// runCLI runs cyanotype with args as if from a terminal described by con,
// with stdin as its input.
func runCLI(t *testing.T, con console, stdin io.Reader, args ...string) result {
	t.Helper()
	var stdout, stderr bytes.Buffer
	cmd := newRootCmdWithConsole("test", func() console { return con })
	cmd.SetArgs(args)
	cmd.SetIn(stdin)
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	err := cmd.Execute()
	code, report := exitCode(err)
	r := result{stdout: stdout.String(), stderr: stderr.String(), code: code}
	if report {
		r.err = err
	}
	return r
}

// failReader fails the test if anything reads it: non-interactive runs
// must never wait for input.
type failReader struct{ t *testing.T }

func (r failReader) Read([]byte) (int, error) {
	r.t.Error("stdin was read in a non-interactive run")
	return 0, io.EOF
}

var (
	nonInteractive = console{}
	interactive    = console{interactive: true}
)

// newProject writes files, given by slash-separated path, into a new
// temporary folder.
func newProject(t *testing.T, files map[string]string) string {
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
	return dir
}

// tree returns every file under dir by slash-separated path.
func tree(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(p) //nolint:gosec // test temp dir
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		out[filepath.ToSlash(rel)] = string(data)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func keys(m map[string]string) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

func designExample(t *testing.T) string {
	t.Helper()
	return designdoctest.SpecExample(t, filepath.Join("..", ".."))
}

// minimalSpec targets AGENTS.md only.
const minimalSpec = `version: 1
targets: [agents-md]
project:
  name: tiny
  goal: A tiny project for tests.
selections: [conventional-commits]
`

// TestGenerateDesignExample runs generate end to end on the spec example
// of docs/DESIGN.md with the bundled catalog, and checks that it writes
// exactly what the renderer returns. Comparing with the renderer rather
// than with golden files keeps catalog text edits from breaking this test;
// the renderer's own golden files check the text.
func TestGenerateDesignExample(t *testing.T) {
	example := designExample(t)
	dir := newProject(t, map[string]string{"cyanotype.yaml": example})

	r := runCLI(t, nonInteractive, failReader{t}, "generate", "--dir", dir, "--yes")
	if r.code != codeOK || r.err != nil {
		t.Fatalf("exit code %d, error %v\nstderr:\n%s", r.code, r.err, r.stderr)
	}

	want := map[string]string{"cyanotype.yaml": example}
	for _, f := range renderExample(t, example) {
		want[f.Path] = string(f.Content)
	}
	got := tree(t, dir)
	if !slices.Equal(keys(got), keys(want)) {
		t.Fatalf("files = %q, want %q", keys(got), keys(want))
	}
	for path, content := range want {
		if got[path] != content {
			t.Errorf("%s differs from the rendered file:\n%s", path, got[path])
		}
	}

	for _, s := range []string{
		"--- /dev/null\n+++ b/AGENTS.md\n",
		"  created    .claude/settings.json\n",
		"  created    CLAUDE.md\n",
		"4 files to write (4 created, 0 modified).\n",
		"Wrote 4 files.\n",
	} {
		if !strings.Contains(r.stdout, s) {
			t.Errorf("stdout is missing %q:\n%s", s, r.stdout)
		}
	}
	wantWarning := `cyanotype.yaml:5: warning: targets[2]: cyanotype cannot generate files for "cursor" yet`
	if !strings.Contains(r.stderr, wantWarning) {
		t.Errorf("stderr is missing %q:\n%s", wantWarning, r.stderr)
	}
}

// renderExample renders the example with the bundled catalog and no
// existing files.
func renderExample(t *testing.T, example string) []render.File {
	t.Helper()
	c, err := catalog.LoadRoot(bundled.FS())
	if err != nil {
		t.Fatal(err)
	}
	f, err := spec.Load(strings.NewReader(example))
	if err != nil {
		t.Fatal(err)
	}
	res, _, err := resolve.Resolve(c, f)
	if err != nil {
		t.Fatal(err)
	}
	files, _, err := render.Render(res, nil)
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func TestGenerateDryRunWritesNothing(t *testing.T) {
	dir := newProject(t, map[string]string{"cyanotype.yaml": designExample(t)})
	r := runCLI(t, interactive, failReader{t}, "generate", "--dir", dir, "--dry-run")
	if r.code != codeNotWritten || r.err != nil {
		t.Errorf("exit code %d, error %v; want %d", r.code, r.err, codeNotWritten)
	}
	if got := keys(tree(t, dir)); !slices.Equal(got, []string{"cyanotype.yaml"}) {
		t.Errorf("--dry-run wrote files: %q", got)
	}
	if !strings.Contains(r.stdout, "+++ b/AGENTS.md") || !strings.Contains(r.stdout, "Dry run: nothing was written.") {
		t.Errorf("stdout lacks the diff or the dry-run note:\n%s", r.stdout)
	}
}

func TestGenerateNonInteractiveWritesNothing(t *testing.T) {
	dir := newProject(t, map[string]string{"cyanotype.yaml": minimalSpec})
	r := runCLI(t, nonInteractive, failReader{t}, "generate", "--dir", dir)
	if r.code != codeNotWritten || r.err != nil {
		t.Errorf("exit code %d, error %v; want %d", r.code, r.err, codeNotWritten)
	}
	if got := keys(tree(t, dir)); !slices.Equal(got, []string{"cyanotype.yaml"}) {
		t.Errorf("a non-interactive run without --yes wrote files: %q", got)
	}
	want := "Run \"cyanotype generate --dir " + dir + " --yes\" to write these files."
	if !strings.Contains(r.stderr, "stdin is not a terminal") || !strings.Contains(r.stderr, want) {
		t.Errorf("stderr does not explain how to write:\n%s", r.stderr)
	}
}

func TestGenerateConfirmation(t *testing.T) {
	tests := []struct {
		answer  string
		code    int
		written bool
	}{
		{"y\n", codeOK, true},
		{"YES\r\n", codeOK, true},
		{"n\n", codeNotWritten, false},
		{"\n", codeNotWritten, false},
		{"", codeNotWritten, false}, // end of input
	}
	for _, tt := range tests {
		t.Run(strings.TrimSpace(tt.answer), func(t *testing.T) {
			dir := newProject(t, map[string]string{"cyanotype.yaml": minimalSpec})
			r := runCLI(t, interactive, strings.NewReader(tt.answer), "generate", "--dir", dir)
			if r.code != tt.code || r.err != nil {
				t.Errorf("exit code %d, error %v; want %d", r.code, r.err, tt.code)
			}
			if !strings.Contains(r.stderr, "Write these files? [y/N] ") {
				t.Errorf("no confirmation prompt on stderr:\n%s", r.stderr)
			}
			_, err := os.Stat(filepath.Join(dir, "AGENTS.md"))
			if written := err == nil; written != tt.written {
				t.Errorf("AGENTS.md written = %v, want %v", written, tt.written)
			}
		})
	}
}

func TestGenerateSecondRunUnchanged(t *testing.T) {
	dir := newProject(t, map[string]string{"cyanotype.yaml": designExample(t)})
	if r := runCLI(t, nonInteractive, failReader{t}, "generate", "--dir", dir, "--yes"); r.code != codeOK {
		t.Fatalf("first run: exit code %d, error %v\n%s", r.code, r.err, r.stderr)
	}
	before := tree(t, dir)

	// Without --yes and without a terminal: nothing to write means nothing
	// to ask, so the run succeeds.
	r := runCLI(t, nonInteractive, failReader{t}, "generate", "--dir", dir)
	if r.code != codeOK || r.err != nil {
		t.Errorf("second run: exit code %d, error %v; want %d", r.code, r.err, codeOK)
	}
	if strings.Contains(r.stdout, "+++ ") {
		t.Errorf("second run shows a diff:\n%s", r.stdout)
	}
	for _, path := range []string{".claude/settings.json", ".gitignore", "AGENTS.md", "CLAUDE.md"} {
		if !strings.Contains(r.stdout, "  unchanged  "+path+"\n") {
			t.Errorf("second run does not report %s unchanged:\n%s", path, r.stdout)
		}
	}
	if !strings.Contains(r.stdout, "Everything is up to date; nothing to write.") {
		t.Errorf("second run does not say it is up to date:\n%s", r.stdout)
	}
	after := tree(t, dir)
	for path, content := range before {
		if after[path] != content {
			t.Errorf("%s changed on the second run", path)
		}
	}
}

// TestGenerateMergesExisting checks that the user's own settings and
// .gitignore lines survive, with CRLF endings kept in .gitignore.
func TestGenerateMergesExisting(t *testing.T) {
	const settings = `{
  "model": "opus",
  "permissions": {
    "deny": ["Bash(curl:*)"]
  }
}
`
	const gitignore = "node_modules/\r\n.env\r\n"
	dir := newProject(t, map[string]string{
		"cyanotype.yaml":        designExample(t),
		".claude/settings.json": settings,
		".gitignore":            gitignore,
	})
	r := runCLI(t, nonInteractive, failReader{t}, "generate", "--dir", dir, "--yes")
	if r.code != codeOK {
		t.Fatalf("exit code %d, error %v\n%s", r.code, r.err, r.stderr)
	}
	for _, s := range []string{"  modified   .claude/settings.json\n", "  modified   .gitignore\n"} {
		if !strings.Contains(r.stdout, s) {
			t.Errorf("stdout is missing %q:\n%s", s, r.stdout)
		}
	}

	files := tree(t, dir)
	var got struct {
		Model       string `json:"model"`
		Permissions struct {
			Deny []string `json:"deny"`
		} `json:"permissions"`
	}
	if err := json.Unmarshal([]byte(files[".claude/settings.json"]), &got); err != nil {
		t.Fatalf("settings.json is not valid JSON: %v\n%s", err, files[".claude/settings.json"])
	}
	if got.Model != "opus" {
		t.Errorf("settings.json lost the user's model: %q", got.Model)
	}
	if len(got.Permissions.Deny) < 2 || got.Permissions.Deny[0] != "Bash(curl:*)" {
		t.Errorf("deny = %q, want the user's rule first and generated rules after it", got.Permissions.Deny)
	}

	ignore := files[".gitignore"]
	if !strings.HasPrefix(ignore, gitignore+"\r\n# cyanotype:begin") || !strings.HasSuffix(ignore, "# cyanotype:end\r\n") {
		t.Errorf(".gitignore = %q, want the user's lines kept and the block appended with CRLF", ignore)
	}
}

// TestGenerateRefusesPathEscape checks that a spec trying to put topic
// files outside the project is rejected before anything is written. The
// generate package tests Write's own refusal of such paths.
func TestGenerateRefusesPathEscape(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "project")
	if err := os.Mkdir(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	text := minimalSpec + "output:\n  split: always\n  topic_dir: ../outside\n"
	if err := os.WriteFile(filepath.Join(dir, "cyanotype.yaml"), []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}

	r := runCLI(t, nonInteractive, failReader{t}, "generate", "--dir", dir, "--yes")
	if r.code != codeError {
		t.Errorf("exit code %d, want %d", r.code, codeError)
	}
	if want := `cyanotype.yaml:9: error: output.topic_dir: "../outside" must be a relative path inside the project`; !strings.Contains(r.stderr, want) {
		t.Errorf("stderr is missing %q:\n%s", want, r.stderr)
	}
	if got := keys(tree(t, parent)); !slices.Equal(got, []string{"project/cyanotype.yaml"}) {
		t.Errorf("files were written: %q", got)
	}
}

func TestGenerateErrors(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		args  []string
		code  int
		// stderr lists what cyanotype prints; err is what main prints.
		stderr []string
		err    string
	}{
		{
			name:  "unknown entry",
			files: map[string]string{"cyanotype.yaml": strings.Replace(minimalSpec, "conventional-commits", "conventional-comits", 1)},
			code:  codeError,
			stderr: []string{
				`cyanotype.yaml:6: error: selections[0]: unknown entry "conventional-comits"; did you mean "conventional-commits"?`,
				"1 error; nothing was written.",
			},
		},
		{
			name:  "YAML syntax error",
			files: map[string]string{"cyanotype.yaml": "version: 1\ntargets: [claude\nproject: x\n"},
			code:  codeError,
			stderr: []string{
				"cyanotype.yaml: error: invalid YAML near line ",
				"   2 | targets: [claude\n",
			},
		},
		{
			name:  "several errors",
			files: map[string]string{"cyanotype.yaml": "version: 1\ntargets: [vscode]\nproject:\n  name: x\n"},
			code:  codeError,
			stderr: []string{
				`cyanotype.yaml:2: error: targets[0]: must be`,
				`cyanotype.yaml:3: error: project.goal: is required`,
				"2 errors; nothing was written.",
			},
		},
		{
			name: "protected section without its end marker",
			files: map[string]string{
				"cyanotype.yaml": minimalSpec,
				"AGENTS.md":      "# tiny\n\n<!-- cyanotype:custom -->\nmine\n",
			},
			code:   codeError,
			stderr: []string{"error: render AGENTS.md: "},
		},
		{
			name:  "no spec",
			files: map[string]string{},
			code:  codeError,
			err:   "no cyanotype.yaml in ",
		},
		{
			name:  "dry run with yes",
			files: map[string]string{"cyanotype.yaml": minimalSpec},
			args:  []string{"--dry-run", "--yes"},
			code:  codeUsage,
			err:   "--dry-run and --yes cannot be used together",
		},
		{
			name:  "unknown flag",
			files: map[string]string{"cyanotype.yaml": minimalSpec},
			args:  []string{"--force"},
			code:  codeUsage,
			err:   "unknown flag: --force",
		},
		{
			name:  "argument",
			files: map[string]string{"cyanotype.yaml": minimalSpec},
			args:  []string{"extra"},
			code:  codeUsage,
			err:   `unknown command "extra"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := newProject(t, tt.files)
			before := tree(t, dir)
			args := append([]string{"generate", "--dir", dir, "--yes"}, tt.args...)
			if slices.Contains(tt.args, "--yes") {
				args = append([]string{"generate", "--dir", dir}, tt.args...)
			}
			r := runCLI(t, nonInteractive, failReader{t}, args...)
			if r.code != tt.code {
				t.Errorf("exit code %d, want %d (error %v)\nstderr:\n%s", r.code, tt.code, r.err, r.stderr)
			}
			for _, s := range tt.stderr {
				if !strings.Contains(r.stderr, s) {
					t.Errorf("stderr is missing %q:\n%s", s, r.stderr)
				}
			}
			switch {
			case tt.err == "" && r.err != nil:
				t.Errorf("main would also print %q", r.err)
			case tt.err != "" && (r.err == nil || !strings.Contains(r.err.Error(), tt.err)):
				t.Errorf("error = %v, want one containing %q", r.err, tt.err)
			}
			if after := tree(t, dir); !slices.Equal(keys(after), keys(before)) {
				t.Errorf("files changed from %q to %q", keys(before), keys(after))
			}
		})
	}
}

func TestGenerateColors(t *testing.T) {
	dir := newProject(t, map[string]string{"cyanotype.yaml": minimalSpec})
	plain := runCLI(t, nonInteractive, failReader{t}, "generate", "--dir", dir, "--dry-run")
	if strings.Contains(plain.stdout+plain.stderr, "\x1b[") {
		t.Errorf("output without a color terminal has escape sequences:\n%q", plain.stdout)
	}

	colored := runCLI(t, console{colorOut: true}, failReader{t}, "generate", "--dir", dir, "--dry-run")
	for _, s := range []string{green + "+# tiny" + reset, cyan + "@@ ", green + "created   " + reset} {
		if !strings.Contains(colored.stdout, s) {
			t.Errorf("colored stdout is missing %q:\n%q", s, colored.stdout)
		}
	}
}

func TestColorEnabled(t *testing.T) {
	tests := []struct {
		name     string
		terminal bool
		env      map[string]string
		want     bool
	}{
		{"terminal", true, nil, true},
		{"not a terminal", false, nil, false},
		{"NO_COLOR set", true, map[string]string{"NO_COLOR": "1"}, false},
		{"NO_COLOR empty", true, map[string]string{"NO_COLOR": ""}, true},
		{"dumb terminal", true, map[string]string{"TERM": "dumb"}, false},
		{"other terminal", true, map[string]string{"TERM": "xterm-256color"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			getenv := func(k string) string { return tt.env[k] }
			if got := colorEnabled(tt.terminal, getenv); got != tt.want {
				t.Errorf("colorEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExitCode(t *testing.T) {
	plain := errors.New("disk full")
	tests := []struct {
		name       string
		err        error
		code       int
		wantReport bool
	}{
		{"success", nil, codeOK, false},
		{"plain error", plain, codeError, true},
		{"usage", usageError(plain), codeUsage, true},
		{"already reported", &exitError{code: codeNotWritten}, codeNotWritten, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, report := exitCode(tt.err)
			if code != tt.code || report != tt.wantReport {
				t.Errorf("exitCode() = %d, %v; want %d, %v", code, report, tt.code, tt.wantReport)
			}
		})
	}
}
