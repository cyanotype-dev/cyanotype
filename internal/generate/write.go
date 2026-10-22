package generate

import (
	"bytes"
	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/cyanotype-dev/cyanotype/internal/adopt"
	"github.com/cyanotype-dev/cyanotype/internal/lock"
)

// Permissions of what Write creates. Generated files belong to the
// repository and are read by every collaborator and tool, so they are
// readable by all, like the files git checks out; the umask still applies.
const (
	filePerm fs.FileMode = 0o644
	dirPerm  fs.FileMode = 0o755
)

// Write puts the created and modified files of changes on disk under root,
// removes the deleted ones, and returns the paths it wrote or removed, in
// order. Unchanged files are not touched. The only files ever removed are
// those of Deleted changes: files cyanotype generated, no longer generates,
// and nobody changed since.
//
// Every path is checked before anything is written, and any of these stops
// the whole write: a hand-written instruction file (see Change.HandWritten)
// without a choice that writes, or chosen for replacing when git could not
// give it back (see adopt.Git); a moved file whose destination exists; a
// path that leaves the project root; a target that is a symbolic link or
// not a regular file. Root itself refuses to follow a symbolic link out of
// the project. Each file is written to a temporary file in its folder and
// then renamed over the target, so a crash never leaves a half-written
// file. Missing folders are created; folders are never removed.
//
// Moved files are written first, so a hand-written file is replaced only
// once its content is safe in its new place. cyanotype.lock comes next:
// should a later write fail, a lock that claims a rule never written
// forgets it on the next run, while a lock left behind would leave rules
// nobody owns. Deleted files come last, and each is read again just before
// it is removed: one that changed since Preview is refused. When one file
// fails, the files written before it stay, and written lists them so the
// caller can say so.
func Write(root *os.Root, changes []Change) (written []string, err error) {
	for _, c := range changes {
		if c.Status == Unchanged {
			continue
		}
		if err := checkChange(root, c); err != nil {
			return nil, err
		}
		if err := checkTarget(root, c.Path); err != nil {
			return nil, err
		}
	}
	// Moved files, the lock, the rest, then deletions, each group in its
	// order.
	var moved, locks, rest, deleted []Change
	for _, c := range changes {
		switch {
		case c.MovedFrom != "":
			moved = append(moved, c)
		case c.Path == lock.Path:
			locks = append(locks, c)
		case c.Status == Deleted:
			deleted = append(deleted, c)
		default:
			rest = append(rest, c)
		}
	}
	for _, c := range slices.Concat(moved, locks, rest) {
		if c.Status == Unchanged {
			continue
		}
		if err := writeFile(root, filepath.FromSlash(c.Path), c.New); err != nil {
			return written, fmt.Errorf("write %s: %w", c.Path, err)
		}
		written = append(written, c.Path)
	}
	for _, c := range deleted {
		if err := deleteFile(root, c); err != nil {
			return written, err
		}
		written = append(written, c.Path)
	}
	return written, nil
}

// deleteFile removes the file of a Deleted change, once it has checked that
// the file still holds what Preview read: an edit made since, while the
// user looked at the diff, must not be lost.
func deleteFile(root *os.Root, c Change) error {
	name := filepath.FromSlash(c.Path)
	now, err := root.ReadFile(name)
	if err != nil {
		return fmt.Errorf("delete %s: %w", c.Path, err)
	}
	if !bytes.Equal(now, c.Old) {
		return fmt.Errorf("refusing to delete %s: it changed after cyanotype compared it; run cyanotype generate again", c.Path)
	}
	if err := root.Remove(name); err != nil {
		return fmt.Errorf("delete %s: %w", c.Path, err)
	}
	return nil
}

// checkChange reports a change Write must not make because of what is at
// its path: a hand-written file nobody chose to replace, one git could not
// give back, or the destination of a moved file that already exists.
func checkChange(root *os.Root, c Change) error {
	switch {
	case c.HandWritten && !c.Choice.Writes():
		return fmt.Errorf("refusing to replace %s: %w", c.Path, ErrHandWritten)
	case c.HandWritten && c.Choice == adopt.Replace:
		if err := (adopt.Git{}).CheckRecoverable(root.Name(), c.Path); err != nil {
			return fmt.Errorf("refusing to replace %s: %w", c.Path, err)
		}
	case c.MovedFrom != "":
		if _, err := root.Lstat(filepath.FromSlash(c.Path)); !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("refusing to move %s to %s: that path already exists", c.MovedFrom, c.Path)
		}
	}
	return nil
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
