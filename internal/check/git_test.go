package check

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

// gitRepo creates a git repository holding files, all committed, even
// those a .gitignore would ignore. It skips the test when git is missing.
func gitRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
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
	for _, args := range [][]string{
		{"init", "-q", "."},
		{"add", "-f", "."},
		{"-c", "user.name=test", "-c", "user.email=test@example.com", "commit", "-q", "-m", "init"},
	} {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...) //nolint:gosec // fixed git arguments in test code
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return dir
}

func TestExecGitTrackedIgnored(t *testing.T) {
	dir := gitRepo(t, map[string]string{
		".env": "A=1\n", ".env.example": "A=\n", ".env.local": "B=1\n", "sub/.env": "C=1\n", "main.go": "package main\n",
	})
	tests := []struct {
		patterns []string
		want     []string
	}{
		{[]string{".env", ".env.*", "!.env.example"}, []string{".env", ".env.local", "sub/.env"}},
		// A negation only re-includes what the lines before it ignore.
		{[]string{"!.env.example", ".env", ".env.*"}, []string{".env", ".env.example", ".env.local", "sub/.env"}},
		{[]string{".env"}, []string{".env", "sub/.env"}},
		{[]string{"*.db"}, nil},
	}
	for _, tt := range tests {
		got, err := ExecGit{}.TrackedIgnored(dir, tt.patterns)
		if err != nil {
			t.Fatal(err)
		}
		slices.Sort(got)
		if !slices.Equal(got, tt.want) {
			t.Errorf("TrackedIgnored(%q) = %q, want %q", tt.patterns, got, tt.want)
		}
	}
}

func TestExecGitUnavailable(t *testing.T) {
	if _, err := exec.LookPath("git"); err == nil {
		// Outside any repository: a temporary folder with no .git above it
		// is not guaranteed, so use GIT_CEILING_DIRECTORIES to stop git
		// from looking further up.
		dir := t.TempDir()
		env := append(os.Environ(), "GIT_CEILING_DIRECTORIES="+filepath.Dir(dir))
		_, err := ExecGit{Env: env}.TrackedIgnored(dir, []string{".env"})
		if !errors.Is(err, ErrGitUnavailable) || !strings.Contains(err.Error(), "not in a git repository") {
			t.Errorf("outside a repository: error = %v, want ErrGitUnavailable", err)
		}
	}
	_, err := ExecGit{Path: filepath.Join(t.TempDir(), "no-such-git")}.TrackedIgnored(t.TempDir(), []string{".env"})
	if !errors.Is(err, ErrGitUnavailable) || !strings.Contains(err.Error(), "not installed") {
		t.Errorf("without git: error = %v, want ErrGitUnavailable", err)
	}
}

// TestTrackedIgnored runs the whole check on a repository where .env and
// a SQLite database were committed before cyanotype's .gitignore block.
func TestTrackedIgnored(t *testing.T) {
	text := strings.Replace(baseSpec, "no-destructive-commands]", "no-destructive-commands, sqlite]", 1)
	project := generated(t, text)
	files := map[string]string{".env": "API_KEY=dummy\n", ".env.example": "API_KEY=\n", "data/app.db": "x"}
	for p, f := range project {
		files[p] = string(f.Data)
	}
	dir := gitRepo(t, files)

	r, err := Run(loadCatalog(t), loadSpec(t, text), os.DirFS(dir), dir, ExecGit{})
	if err != nil {
		t.Fatal(err)
	}
	checkFindings(t, r,
		want{TrackedIgnored, Error, ".env", "is tracked by git, although secrets-in-env keeps it out of git (.gitignore: .env); " +
			"git rm --cached .env stops tracking it, but does not remove it from the history, so any secret it held must also be rotated"},
		want{TrackedIgnored, Error, "data/app.db", "although sqlite keeps it out of git (.gitignore: *.db)"},
	)
	if len(r.Skipped) != 0 {
		t.Errorf("skipped = %q", r.Skipped)
	}
}

// fakeGit answers TrackedIgnored from a fixed list, or with err.
type fakeGit struct {
	tracked []string
	err     error
}

func (g fakeGit) TrackedIgnored(string, []string) ([]string, error) {
	return g.tracked, g.err
}

func TestTrackedIgnoredWithoutGit(t *testing.T) {
	project := generated(t, baseSpec)
	r, err := Run(loadCatalog(t), loadSpec(t, baseSpec), project, ".", fakeGit{err: errors.Join(ErrGitUnavailable, errors.New("no repository"))})
	if err != nil {
		t.Fatal(err)
	}
	checkFindings(t, r)
	if len(r.Skipped) != 1 || !strings.Contains(r.Skipped[0], "the check for tracked files that should be ignored") {
		t.Errorf("skipped = %q", r.Skipped)
	}

	// Any other failure stops the check: it is never taken as clean.
	_, err = Run(loadCatalog(t), loadSpec(t, baseSpec), project, ".", fakeGit{err: errors.New("git ls-files failed: boom")})
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("Run() error = %v, want the git failure", err)
	}
}

// TestTrackedIgnoredReportsOnce lists a file two entries ignore: it is
// reported once.
func TestTrackedIgnoredReportsOnce(t *testing.T) {
	text := strings.Replace(baseSpec, "no-destructive-commands]", "no-destructive-commands, sqlite]", 1)
	project := generated(t, text)
	project[".env"] = &fstest.MapFile{Data: []byte("A=1\n")}
	r, err := Run(loadCatalog(t), loadSpec(t, text), project, ".", fakeGit{tracked: []string{".env"}})
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, f := range r.Findings {
		if f.Kind == TrackedIgnored {
			n++
		}
	}
	if n != 1 {
		t.Errorf("%d tracked-ignored findings, want 1:\n%s", n, describe(r))
	}
}
