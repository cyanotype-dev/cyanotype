package adopt

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// ErrNotRecoverable reports a file Replace must not overwrite, because git
// could not give it back afterwards. Errors from Git.CheckRecoverable wrap
// it, with the reason.
var ErrNotRecoverable = errors.New("git could not give it back")

// Git runs the git command to tell whether a file can be recovered after
// it is replaced.
type Git struct {
	// Path is the git executable; "" means "git", looked up in PATH. Tests
	// set it to a missing or fake executable to check every failure.
	Path string
	// Env, when not nil, is the environment git runs with, as in
	// exec.Cmd.Env.
	Env []string
}

// CheckRecoverable returns nil only when the file at name, relative to the
// project folder dir, is tracked by git and has no changes, staged or
// not, so its content is in the last commit and can be recovered with git
// show or git restore. Every other outcome is an error wrapping
// ErrNotRecoverable that says why and suggests keep or move: the file is
// untracked or changed, the folder is not in a git repository, git is not
// installed, or a git command failed or answered something unexpected.
// A failure is never taken as "clean".
func (g Git) CheckRecoverable(dir, name string) error {
	refuse := func(reason string) error {
		return fmt.Errorf("%w: %s; commit it first, or choose keep or move, which lose nothing", ErrNotRecoverable, reason)
	}

	exe := g.Path
	if exe == "" {
		exe = "git"
	}
	if _, err := exec.LookPath(exe); err != nil {
		return refuse(fmt.Sprintf("git is not installed or not in PATH, so cyanotype cannot check that %s is committed", name))
	}

	out, err := g.run(exe, dir, "rev-parse", "--is-inside-work-tree")
	switch {
	case err != nil && strings.Contains(err.Error(), "not a git repository"):
		return refuse(fmt.Sprintf("the project folder is not in a git repository, so nothing holds a copy of %s", name))
	case err != nil:
		return refuse(fmt.Sprintf("git failed to check the repository (%v)", err))
	case strings.TrimSpace(out) != "true":
		return refuse(fmt.Sprintf("git answered %q when asked whether the project folder is in a working tree", strings.TrimSpace(out)))
	}

	out, err = g.run(exe, dir, "ls-files", "--error-unmatch", "--", name)
	switch {
	case err != nil && strings.Contains(err.Error(), "did not match"):
		return refuse(fmt.Sprintf("%s is not tracked by git", name))
	case err != nil:
		return refuse(fmt.Sprintf("git failed to check whether %s is tracked (%v)", name, err))
	case strings.TrimSpace(out) == "":
		return refuse(fmt.Sprintf("git did not list %s as tracked", name))
	}

	out, err = g.run(exe, dir, "status", "--porcelain", "--", name)
	switch {
	case err != nil:
		return refuse(fmt.Sprintf("git failed to check %s for changes (%v)", name, err))
	case strings.TrimSpace(out) != "":
		return refuse(fmt.Sprintf("%s has changes that are not committed", name))
	}
	return nil
}

// run runs git with args in dir and returns its standard output. A git
// that fails returns an error holding the first line of what it printed
// on standard error.
func (g Git) run(exe, dir string, args ...string) (string, error) {
	cmd := exec.Command(exe, append([]string{"-C", dir}, args...)...) //nolint:gosec // the executable is git, or one a test chose
	if g.Env != nil {
		cmd.Env = g.Env
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		msg, _, _ := strings.Cut(strings.TrimSpace(stderr.String()), "\n")
		if msg == "" {
			msg = err.Error()
		}
		return stdout.String(), errors.New(msg)
	}
	return stdout.String(), nil
}
