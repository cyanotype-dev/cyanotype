package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// checkSpec selects an entry with permission rules, so a check covers
// every kind of generated file.
const checkSpec = `version: 1
targets: [agents-md, claude]
project:
  name: tiny
  goal: A tiny project for tests.
selections: [go, conventional-commits, secrets-in-env]
`

// generatedProject returns a project folder holding checkSpec and the
// files generate writes for it.
func generatedProject(t *testing.T) string {
	t.Helper()
	dir := newProject(t, map[string]string{"cyanotype.yaml": checkSpec})
	if r := runCLI(t, nonInteractive, failReader{t}, "generate", "--dir", dir, "--yes"); r.code != codeOK {
		t.Fatalf("generate: exit code %d\n%s", r.code, r.stderr)
	}
	return dir
}

func TestCheckClean(t *testing.T) {
	dir := generatedProject(t)
	// Even on a terminal, check never reads stdin.
	r := runCLI(t, interactive, failReader{t}, "check", "--dir", dir)
	if r.code != codeOK || r.err != nil {
		t.Fatalf("exit code %d, error %v\nstdout:\n%s\nstderr:\n%s", r.code, r.err, r.stdout, r.stderr)
	}
	if !strings.HasSuffix(r.stdout, "No drift found.\n") {
		t.Errorf("stdout:\n%s", r.stdout)
	}
}

func TestCheckDrift(t *testing.T) {
	dir := generatedProject(t)
	p := filepath.Join(dir, "AGENTS.md")
	data, err := os.ReadFile(p) //nolint:gosec // test temp dir
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(strings.Replace(string(data), "## Git workflow\n", "## Git workflow\n\n- Mine.\n", 1)), 0o600); err != nil { //nolint:gosec // test temp dir
		t.Fatal(err)
	}
	r := runCLI(t, nonInteractive, failReader{t}, "check", "--dir", dir)
	if r.code != codeDrift || r.err != nil {
		t.Fatalf("exit code %d, error %v; want %d\nstdout:\n%s", r.code, r.err, codeDrift, r.stdout)
	}
	for _, s := range []string{
		filepath.Join(dir, "AGENTS.md") + ": error: was edited by hand since cyanotype generated it",
		"\n1 error.\n",
	} {
		if !strings.Contains(r.stdout, s) {
			t.Errorf("stdout is missing %q:\n%s", s, r.stdout)
		}
	}
}

// TestCheckStrict has only a warning, a dependency the spec does not
// select: it passes, unless --strict.
func TestCheckStrict(t *testing.T) {
	dir := generatedProject(t)
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module x\n\nrequire github.com/jackc/pgx/v5 v5.7.0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := runCLI(t, nonInteractive, failReader{t}, "check", "--dir", dir)
	if r.code != codeOK {
		t.Errorf("without --strict: exit code %d, want %d\n%s", r.code, codeOK, r.stdout)
	}
	if want := filepath.Join(dir, "go.mod") + ": warning: lists github.com/jackc/pgx/v5, which suggests PostgreSQL"; !strings.Contains(r.stdout, want) {
		t.Errorf("stdout is missing %q:\n%s", want, r.stdout)
	}
	if !strings.Contains(r.stdout, "1 warning; no errors.\n") {
		t.Errorf("stdout:\n%s", r.stdout)
	}

	r = runCLI(t, nonInteractive, failReader{t}, "check", "--dir", dir, "--strict")
	if r.code != codeDrift || !strings.Contains(r.stdout, "1 warning; --strict fails on warnings.\n") {
		t.Errorf("with --strict: exit code %d, want %d\n%s", r.code, codeDrift, r.stdout)
	}
}

func TestCheckErrors(t *testing.T) {
	tests := []struct {
		name   string
		files  map[string]string
		args   []string
		code   int
		stderr string
	}{
		{
			name:   "spec that does not resolve",
			files:  map[string]string{"cyanotype.yaml": strings.Replace(checkSpec, "secrets-in-env", "secrets-in-envv", 1)},
			code:   codeError,
			stderr: "1 error; nothing was checked.",
		},
		{name: "no spec", files: map[string]string{}, code: codeError},
		{name: "extra argument", files: map[string]string{}, args: []string{"now"}, code: codeUsage},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := newProject(t, tt.files)
			r := runCLI(t, nonInteractive, failReader{t}, append([]string{"check", "--dir", dir}, tt.args...)...)
			if r.code != tt.code {
				t.Errorf("exit code %d, want %d\nstderr:\n%s", r.code, tt.code, r.stderr)
			}
			if !strings.Contains(r.stderr, tt.stderr) {
				t.Errorf("stderr is missing %q:\n%s", tt.stderr, r.stderr)
			}
		})
	}
}

func TestCheckSummary(t *testing.T) {
	tests := []struct {
		errs, warns int
		strict      bool
		want        string
	}{
		{0, 0, false, "No drift found."},
		{1, 0, false, "1 error."},
		{2, 1, true, "2 errors and 1 warning."},
		{0, 2, false, "2 warnings; no errors."},
		{0, 1, true, "1 warning; --strict fails on warnings."},
	}
	for _, tt := range tests {
		if got := checkSummary(tt.errs, tt.warns, tt.strict); got != tt.want {
			t.Errorf("checkSummary(%d, %d, %v) = %q, want %q", tt.errs, tt.warns, tt.strict, got, tt.want)
		}
	}
}
