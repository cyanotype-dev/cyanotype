package generate

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Permissions of what Write creates. Generated files belong to the
// repository and are read by every collaborator and tool, so they are
// readable by all, like the files git checks out; the umask still applies.
const (
	filePerm fs.FileMode = 0o644
	dirPerm  fs.FileMode = 0o755
)

// Write puts the created and modified files of changes on disk under root
// and returns the paths it wrote, in order. Unchanged files are not
// touched, and no file is ever deleted.
//
// Every path is checked before anything is written: a path that leaves the
// project root, or whose target is a symbolic link or not a regular file,
// stops the whole write. Root itself refuses to follow a symbolic link out
// of the project. Each file is written to a temporary file in its folder and
// then renamed over the target, so a crash never leaves a half-written
// file. Missing folders are created.
//
// When writing one file fails, the files written before it stay, and
// written lists them so the caller can say so.
func Write(root *os.Root, changes []Change) (written []string, err error) {
	for _, c := range changes {
		if c.Status == Unchanged {
			continue
		}
		if err := checkTarget(root, c.Path); err != nil {
			return nil, err
		}
	}
	for _, c := range changes {
		if c.Status == Unchanged {
			continue
		}
		if err := writeFile(root, filepath.FromSlash(c.Path), c.New); err != nil {
			return written, fmt.Errorf("write %s: %w", c.Path, err)
		}
		written = append(written, c.Path)
	}
	return written, nil
}

// checkTarget reports a path Write must not write to.
func checkTarget(root *os.Root, path string) error {
	// filepath.IsLocal rejects absolute paths, ".." elements and, on
	// Windows, reserved names such as NUL or con.md; fs.ValidPath rejects
	// what is not a clean slash-separated path. Backslashes are separators
	// on Windows only, so they are rejected everywhere and a path means the
	// same on every OS, as in the spec.
	name := filepath.FromSlash(path)
	if !fs.ValidPath(path) || strings.ContainsRune(path, '\\') || !filepath.IsLocal(name) {
		return fmt.Errorf("refusing to write %q: it is not a path inside the project root", path)
	}
	info, err := root.Lstat(name)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil
	case err != nil:
		return fmt.Errorf("check %s: %w", path, err)
	case info.Mode()&fs.ModeSymlink != 0:
		// Renaming over a link would replace the link with a file, such as
		// a CLAUDE.md linked to AGENTS.md: the user's layout, not ours.
		return fmt.Errorf("refusing to replace %s: it is a symbolic link", path)
	case !info.Mode().IsRegular():
		return fmt.Errorf("refusing to replace %s: it is not a regular file", path)
	}
	return nil
}

// writeFile atomically replaces the file name under root with data. An
// existing file keeps its permissions.
func writeFile(root *os.Root, name string, data []byte) (err error) {
	dir := filepath.Dir(name)
	if dir != "." {
		if err := root.MkdirAll(dir, dirPerm); err != nil {
			return err
		}
	}
	perm := filePerm
	if info, err := root.Lstat(name); err == nil {
		perm = info.Mode().Perm()
	}

	tmp, f, err := createTemp(root, dir, filepath.Base(name), perm)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = f.Close()
			_ = root.Remove(tmp) // only our own temporary file
		}
	}()
	if _, err := f.Write(data); err != nil {
		return err
	}
	// Sync before the rename, so the new name never points to data that
	// is not on disk yet.
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return root.Rename(tmp, name)
}

// createTemp creates a new temporary file next to base in dir, such as
// ".AGENTS.md.cyanotype-K3J5Q2ZB.tmp": a dotfile that names the file it
// replaces and the tool that left it, should a crash leave it behind.
func createTemp(root *os.Root, dir, base string, perm fs.FileMode) (string, *os.File, error) {
	for range 10 {
		name := filepath.Join(dir, "."+base+".cyanotype-"+rand.Text()[:8]+".tmp")
		f, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return "", nil, err
		}
		return name, f, nil
	}
	return "", nil, fmt.Errorf("create a temporary file for %s: too many name collisions", base)
}
