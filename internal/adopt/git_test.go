package adopt

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// fakeGitEnv makes the test binary act as git, so the tests can make git
// fail in ways a real repository cannot. TestMain reads it.
const fakeGitEnv = "CYANOTYPE_FAKE_GIT"

// TestMain runs the test binary as a fake git when fakeGitEnv says how it
// should behave, and as the tests otherwise.
func TestMain(m *testing.M) {
	if mode := os.Getenv(fakeGitEnv); mode != "" {
		os.Exit(fakeGit(mode, os.Args[1:]))
	}
	os.Exit(m.Run())
}

// fakeGit answers git's commands as mode says, and returns the exit code.
func fakeGit(mode string, args []string) int {
	sub := ""
	if len(args) > 2 {
		sub = args[2] // after "-C dir"
	}
	switch {
	case mode == "fail":
		fmt.Fprintln(os.Stderr, "fatal: something unexpected happened\nsecond line")
		return 128
	case mode == "garbage" && sub == "rev-parse":
		fmt.Println("maybe")
	case mode == "silent-ls-files" && sub == "rev-parse":
		fmt.Println("true")
	case mode == "silent-ls-files" && sub == "ls-files":
		// Succeeds, but lists nothing.
	case mode == "status-fails" && sub == "rev-parse":
		fmt.Println("true")
	case mode == "status-fails" && sub == "ls-files":
		fmt.Println("CLAUDE.md")
	case mode == "status-fails" && sub == "status":
		fmt.Fprintln(os.Stderr, "error: index file corrupt")
		return 1
	}
	return 0
}

// requireGit fails the test when git is not installed: the cases below are
// about what real git reports, and skipping them would hide that replace
// went untested.
func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Fatal("git is not installed; the real-repository cases need it")
	}
}

// gitRun runs git in dir and fails the test on error.
func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	all := append([]string{"-C", dir, "-c", "user.name=Test", "-c", "user.email=test@example.com",
		"-c", "commit.gpgsign=false", "-c", "core.autocrlf=false"}, args...)
	cmd := exec.Command("git", all...) //nolint:gosec // test code runs git with fixed arguments
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

// repo returns a new git repository holding a committed CLAUDE.md, with
// GIT_CEILING_DIRECTORIES set so git never finds a repository above it.
func repo(t *testing.T) (string, Git) {
	t.Helper()
	requireGit(t)
	dir := filepath.Join(t.TempDir(), "project")
	if err := os.Mkdir(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	gitRun(t, dir, "init", "-q")
	write(t, dir, "CLAUDE.md", "# Shop\n")
	gitRun(t, dir, "add", "CLAUDE.md")
	gitRun(t, dir, "commit", "-q", "-m", "init")
	return dir, Git{Env: append(os.Environ(), "GIT_CEILING_DIRECTORIES="+filepath.Dir(dir))}
}

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestCheckRecoverableRepository(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, dir string)
		file  string
		want  string // "" means recoverable
	}{
		{"committed and clean", func(*testing.T, string) {}, "CLAUDE.md", ""},
		{"untracked", func(t *testing.T, dir string) { write(t, dir, "AGENTS.md", "# Rules\n") }, "AGENTS.md", "AGENTS.md is not tracked by git"},
		{"changed, not staged", func(t *testing.T, dir string) { write(t, dir, "CLAUDE.md", "# Shop, edited\n") }, "CLAUDE.md", "CLAUDE.md has changes that are not committed"},
		{"changed and staged", func(t *testing.T, dir string) {
			write(t, dir, "CLAUDE.md", "# Shop, staged\n")
			gitRun(t, dir, "add", "CLAUDE.md")
		}, "CLAUDE.md", "CLAUDE.md has changes that are not committed"},
		{"added but never committed", func(t *testing.T, dir string) {
			write(t, dir, "AGENTS.md", "# Rules\n")
			gitRun(t, dir, "add", "AGENTS.md")
		}, "AGENTS.md", "AGENTS.md has changes that are not committed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir, g := repo(t)
			tt.setup(t, dir)
			checkResult(t, g.CheckRecoverable(dir, tt.file), tt.want)
		})
	}
}

func TestCheckRecoverableNotARepository(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	write(t, dir, "CLAUDE.md", "# Shop\n")
	g := Git{Env: append(os.Environ(), "GIT_CEILING_DIRECTORIES="+filepath.Dir(dir))}
	checkResult(t, g.CheckRecoverable(dir, "CLAUDE.md"), "the project folder is not in a git repository")
}

func TestCheckRecoverableGitMissing(t *testing.T) {
	dir := t.TempDir()
	missing := Git{Path: filepath.Join(dir, "no-such-git")}
	checkResult(t, missing.CheckRecoverable(dir, "CLAUDE.md"), "git is not installed or not in PATH")

	t.Setenv("PATH", "")
	checkResult(t, Git{}.CheckRecoverable(dir, "CLAUDE.md"), "git is not installed or not in PATH")
}

func TestCheckRecoverableGitFails(t *testing.T) {
	tests := []struct{ mode, want string }{
		{"fail", "git failed to check the repository (fatal: something unexpected happened)"},
		{"garbage", `git answered "maybe"`},
		{"silent-ls-files", "git did not list CLAUDE.md as tracked"},
		{"status-fails", "git failed to check CLAUDE.md for changes (error: index file corrupt)"},
	}
	for _, tt := range tests {
		t.Run(tt.mode, func(t *testing.T) {
			g := Git{Path: os.Args[0], Env: append(os.Environ(), fakeGitEnv+"="+tt.mode)}
			checkResult(t, g.CheckRecoverable(t.TempDir(), "CLAUDE.md"), tt.want)
		})
	}
}

// checkResult checks that err is nil when want is empty, and otherwise a
// refusal wrapping ErrNotRecoverable that says want and suggests the
// choices that lose nothing.
func checkResult(t *testing.T, err error, want string) {
	t.Helper()
	switch {
	case want == "" && err != nil:
		t.Errorf("CheckRecoverable() error = %v, want nil", err)
	case want == "":
	case !errors.Is(err, ErrNotRecoverable):
		t.Errorf("CheckRecoverable() error = %v, want ErrNotRecoverable", err)
	case !strings.Contains(err.Error(), want) || !strings.Contains(err.Error(), "choose keep or move"):
		t.Errorf("CheckRecoverable() error = %q, want it to contain %q and suggest keep or move", err, want)
	}
}
