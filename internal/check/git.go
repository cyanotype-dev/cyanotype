package check

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"strings"

	"github.com/cyanotype-dev/cyanotype/internal/resolve"
)

// Git answers the questions check asks git. It is an interface so tests
// can answer without a repository; ExecGit runs the git command.
type Git interface {
	// TrackedIgnored returns the files git tracks under dir, relative to
	// it with forward slashes, that the .gitignore patterns would ignore,
	// patterns read in order as in a .gitignore file. An error wrapping
	// ErrGitUnavailable means git cannot answer at all: git is not
	// installed, or dir is not in a repository.
	TrackedIgnored(dir string, patterns []string) ([]string, error)
}

// ErrGitUnavailable reports that git cannot answer, so the checks that
// need it are skipped rather than failed.
var ErrGitUnavailable = errors.New("git is not available")

// ExecGit runs the git command.
type ExecGit struct {
	// Path is the git executable; "" means "git", looked up in PATH.
	Path string
	// Env, when not nil, is the environment git runs with, as in
	// exec.Cmd.Env.
	Env []string
}

// TrackedIgnored implements Git with git ls-files --cached --ignored,
// given each pattern with --exclude and no .gitignore file, so only the
// patterns asked about count.
func (g ExecGit) TrackedIgnored(dir string, patterns []string) ([]string, error) {
	exe := g.Path
	if exe == "" {
		exe = "git"
	}
	if _, err := exec.LookPath(exe); err != nil {
		return nil, fmt.Errorf("%w: git is not installed or not in PATH", ErrGitUnavailable)
	}
	args := []string{"-C", dir, "ls-files", "-z", "--cached", "--ignored"}
	for _, p := range patterns {
		args = append(args, "--exclude="+p)
	}
	cmd := exec.Command(exe, args...) //nolint:gosec // the executable is git, or one a test chose
	if g.Env != nil {
		cmd.Env = g.Env
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		msg, _, _ := strings.Cut(strings.TrimSpace(stderr.String()), "\n")
		if strings.Contains(msg, "not a git repository") {
			return nil, fmt.Errorf("%w: the project folder is not in a git repository", ErrGitUnavailable)
		}
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("git ls-files failed: %s", msg)
	}
	var files []string
	for f := range strings.SplitSeq(stdout.String(), "\x00") {
		if f != "" {
			files = append(files, f)
		}
	}
	return files, nil
}

// trackedFindings reports files git tracks although an enforce.gitignore
// line of a selected entry keeps them out of git, such as a .env committed
// before the .gitignore block existed. .gitignore only stops untracked
// files from being added, so the block alone gives a false sense of
// safety. Each finding names the line that matches: the first of the
// entry's lines that ignores the file, with the entry's negations after it.
func trackedFindings(res *resolve.Result, dir string, git Git) ([]Finding, []string, error) {
	if git == nil {
		return nil, nil, nil
	}
	var (
		out      []Finding
		reported = make(map[string]bool)
	)
	for _, e := range res.Entries {
		if e.Enforce == nil || len(e.Enforce.Gitignore) == 0 {
			continue
		}
		lines := e.Enforce.Gitignore
		tracked, err := git.TrackedIgnored(dir, lines)
		if errors.Is(err, ErrGitUnavailable) {
			return nil, []string{"the check for tracked files that should be ignored: " + err.Error()}, nil
		}
		if err != nil {
			return nil, nil, err
		}
		for _, file := range tracked {
			if reported[file] {
				continue
			}
			reported[file] = true
			line, err := matchingLine(git, dir, lines, file)
			if err != nil {
				return nil, nil, err
			}
			out = append(out, Finding{Kind: TrackedIgnored, Severity: Error, Path: file, Msg: fmt.Sprintf(
				"is tracked by git, although %s keeps it out of git (.gitignore: %s); git rm --cached %s stops tracking it, "+
					"but does not remove it from the history, so any secret it held must also be rotated",
				e.ID, line, file)})
		}
	}
	return out, nil, nil
}

// matchingLine returns the first line of lines that ignores file, read
// with the negations that follow it; or all of lines, joined, should none
// match alone.
func matchingLine(git Git, dir string, lines []string, file string) (string, error) {
	for i, line := range lines {
		if strings.HasPrefix(line, "!") {
			continue
		}
		patterns := []string{line}
		for _, later := range lines[i+1:] {
			if strings.HasPrefix(later, "!") {
				patterns = append(patterns, later)
			}
		}
		tracked, err := git.TrackedIgnored(dir, patterns)
		if err != nil {
			return "", err
		}
		if slices.Contains(tracked, file) {
			return line, nil
		}
	}
	return strings.Join(lines, ", "), nil
}
