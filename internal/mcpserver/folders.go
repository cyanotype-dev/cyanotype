// Package mcpserver is cyanotype's MCP server: it serves the catalog,
// previews and generation to a coding agent over the Model Context
// Protocol, as tools that return the same JSON as the CLI's --json.
package mcpserver

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

var (
	// ErrOutside is returned for a folder outside every allowed folder,
	// or one reached through a link that leaves them.
	ErrOutside = errors.New("outside the folders the server may use")
	// ErrDirRequired is returned when a tool leaves out dir and the server
	// may use more than one folder, so none is the obvious default.
	ErrDirRequired = errors.New("dir is required when the server may use more than one folder")
	// ErrInvalidPath is returned for a path the server never resolves:
	// one with a NUL byte, a network or device path, or a path relative to
	// a drive's current folder on Windows.
	ErrInvalidPath = errors.New("invalid path")
)

// Folders are the folders a connected agent may use through the server's
// tools: the --allow folders, or the folder the server started in. A tool
// reaches a project only through Open, which confines it to them.
//
// Each allowed folder is opened once, when the server starts, and kept
// open: a project is opened inside it with os.Root, which refuses any
// symbolic link or junction that leads out of it, so the check cannot be
// raced by replacing a folder with a link after it was made.
type Folders struct {
	start   string
	allowed []*os.Root
	infos   []fs.FileInfo
}

// OpenFolders opens the allowed folders dirs, relative to start, the
// absolute path of the folder the server started in. With no dirs, start
// is the only allowed folder. Each must exist and be a folder; a folder
// given twice, under any name, is opened once.
func OpenFolders(start string, dirs []string) (*Folders, error) {
	if !filepath.IsAbs(start) {
		return nil, fmt.Errorf("start folder %q is not absolute", start)
	}
	if len(dirs) == 0 {
		dirs = []string{start}
	}
	f := &Folders{start: start}
	for _, d := range dirs {
		if err := checkPath(d); err != nil {
			_ = f.Close()
			return nil, fmt.Errorf("allowed folder %q: %w", d, err)
		}
		r, err := os.OpenRoot(f.abs(d))
		if err != nil {
			_ = f.Close()
			return nil, fmt.Errorf("allowed folder: %w", err)
		}
		info, err := r.Stat(".")
		if err != nil {
			_ = r.Close()
			_ = f.Close()
			return nil, fmt.Errorf("allowed folder: %w", err)
		}
		if f.index(info) >= 0 {
			_ = r.Close()
			continue
		}
		f.allowed = append(f.allowed, r)
		f.infos = append(f.infos, info)
	}
	return f, nil
}

// Names returns the paths of the allowed folders, as they were given,
// made absolute.
func (f *Folders) Names() []string {
	names := make([]string, len(f.allowed))
	for i, r := range f.allowed {
		names[i] = r.Name()
	}
	return names
}

// Close closes the allowed folders.
func (f *Folders) Close() error {
	var errs []error
	for _, r := range f.allowed {
		errs = append(errs, r.Close())
	}
	return errors.Join(errs...)
}

// Open opens the project folder dir, which must be one of the allowed
// folders or inside one. A relative dir is relative to the folder the
// server started in. An empty dir means the allowed folder, when there is
// only one. The caller closes the returned root.
func (f *Folders) Open(dir string) (*os.Root, error) {
	if dir == "" {
		if len(f.allowed) > 1 {
			return nil, ErrDirRequired
		}
		return openIn(f.allowed[0], ".", ".")
	}
	if err := checkPath(dir); err != nil {
		return nil, fmt.Errorf("dir %q: %w", dir, err)
	}
	abs := f.abs(dir)
	i, rel, ok := f.within(abs)
	if !ok {
		// A link from outside to a folder inside an allowed one: try again
		// with the links resolved. os.Root still checks every link below
		// the allowed folder, so this only finds more, never lets more out.
		if resolved, err := filepath.EvalSymlinks(abs); err == nil {
			i, rel, ok = f.within(resolved)
		}
	}
	if !ok {
		return nil, fmt.Errorf("dir %q: %w", dir, ErrOutside)
	}
	return openIn(f.allowed[i], dir, rel)
}

// within walks up from the absolute path abs to the first folder that is
// an allowed one, and returns its index and abs relative to it. Paths are
// compared by the file they name, not by their spelling, so a different
// case or an 8.3 short name on Windows, or a link to an allowed folder,
// still matches. A link below it is left to os.Root.
func (f *Folders) within(abs string) (i int, rel string, ok bool) {
	var rest []string
	for p := abs; ; {
		if info, err := os.Stat(p); err == nil {
			if i := f.index(info); i >= 0 {
				return i, filepath.Join(rest...), true
			}
		}
		parent := filepath.Dir(p)
		if parent == p {
			return 0, "", false
		}
		rest = append([]string{filepath.Base(p)}, rest...)
		p = parent
	}
}

// openIn opens the folder rel inside the allowed folder r. dir is the
// path the tool gave, for messages.
func openIn(r *os.Root, dir, rel string) (*os.Root, error) {
	if rel == "" {
		rel = "."
	}
	sub, err := r.OpenRoot(rel)
	if err == nil {
		return named(r, rel, sub), nil
	}
	// Windows reports a file opened as a folder as missing, so the type
	// is checked first.
	if info, serr := r.Stat(rel); serr == nil && !info.IsDir() {
		return nil, fmt.Errorf("dir %q: not a folder", dir)
	}
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("dir %q: no such folder", dir)
	}
	// Anything else, such as a link that leaves the allowed folder.
	return nil, fmt.Errorf("dir %q: %w", dir, ErrOutside)
}

// named returns the folder sub, opened as rel inside r, under a clean
// name. os.Root names a folder opened inside another by joining the two
// names as they are, "project/." for ".", and that name reaches messages
// and git. So the clean path is opened too, and used only if it is still
// the folder r confined, which a link swapped in meanwhile would change.
func named(r *os.Root, rel string, sub *os.Root) *os.Root {
	clean, err := os.OpenRoot(filepath.Join(r.Name(), rel))
	if err != nil {
		return sub
	}
	a, errA := sub.Stat(".")
	b, errB := clean.Stat(".")
	if errA != nil || errB != nil || !os.SameFile(a, b) {
		_ = clean.Close()
		return sub
	}
	_ = sub.Close()
	return clean
}

// abs makes path absolute against the folder the server started in.
func (f *Folders) abs(path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(f.start, path)
}

// index returns the position of the allowed folder info names, or -1.
func (f *Folders) index(info fs.FileInfo) int {
	for i, a := range f.infos {
		if os.SameFile(info, a) {
			return i
		}
	}
	return -1
}

// checkPath rejects the paths the server never touches, before any of
// them reaches the file system: a NUL byte; a Windows device path
// (\\?\, \\.\); a network path (\\server\share), since only looking one
// up can send the user's credentials to that server; and a path relative
// to a drive's current folder (C:project), which depends on state the
// agent cannot see.
func checkPath(path string) error {
	switch {
	case strings.ContainsRune(path, 0):
		return fmt.Errorf("%w: NUL byte", ErrInvalidPath)
	case isSlash(path, 0) && isSlash(path, 1):
		return fmt.Errorf("%w: network and device paths are not accepted", ErrInvalidPath)
	case filepath.VolumeName(path) != "" && !filepath.IsAbs(path):
		return fmt.Errorf("%w: a path relative to a drive's current folder", ErrInvalidPath)
	}
	return nil
}

// isSlash reports whether path has a slash or a backslash at i. Both
// count on every system: a backslash path only means something on
// Windows, and rejecting it elsewhere costs nothing.
func isSlash(path string, i int) bool {
	return i < len(path) && (path[i] == '/' || path[i] == '\\')
}
