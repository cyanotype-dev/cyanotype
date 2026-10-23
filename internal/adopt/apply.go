package adopt

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/cyanotype-dev/cyanotype/internal/render"
)

// Errors wrapped by Apply when a choice cannot be applied to a file, so
// interfaces can tell them apart and suggest another choice.
var (
	// ErrKeepMarkers reports a file that already has cyanotype:custom
	// markers, so it cannot become one protected block.
	ErrKeepMarkers = errors.New("already has cyanotype:custom markers, so it cannot be kept as one block")
	// ErrMoveExists reports a move whose destination already exists.
	ErrMoveExists = errors.New("that file already exists")
)

// MovedFile is a file Move creates: the content of a hand-written file at
// a path of its own.
type MovedFile struct {
	// From is the hand-written file the content comes from.
	From string
	// Path is where the content goes, relative to the project root.
	Path string
	// Content is the hand-written file, byte for byte.
	Content []byte
}

// MovePath returns where Move puts the content of the file at p: the topic
// folder, under "original-" and the file's name in lowercase, such as
// docs/agents/original-claude.md. Never CLAUDE.md or AGENTS.md: Claude Code
// and Codex would read a file of that name in a subfolder as instructions
// for that folder, so the old rules would apply twice, or in the wrong
// place.
func MovePath(topicDir, p string) string {
	return path.Join(topicDir, "original-"+strings.ToLower(path.Base(p)))
}

// Apply returns the project as the renderer must read it once the
// choices are made, and the files Move creates. choices maps the path of
// each hand-written file to its choice; mainPath is the main instruction
// file, which gets the pointers to moved files, and topicDir the folder
// they move to.
//
// The renderer keeps protected blocks, so Apply only changes what the
// renderer reads, never the disk:
//
//   - Keep: the file reads as one protected block holding all of it, which
//     the renderer puts right after the title;
//   - Move: the file reads as absent, and the main file as having a
//     protected block in Further reading that tells the agent to read the
//     moved file;
//   - Replace: the file reads as it is; it is simply allowed to change.
//
// Import writes nothing, so it is an error here, as is a file that already
// has protected blocks, which Keep cannot wrap, and a Move whose
// destination exists.
func Apply(existing fs.FS, choices map[string]Choice, mainPath, topicDir string) (fs.FS, []MovedFile, error) {
	view := &overlay{base: existing, files: map[string][]byte{}}
	var moved []MovedFile
	var pointers []string
	read := func(p string) ([]byte, error) {
		if data, ok := view.files[p]; ok {
			return data, nil
		}
		return fs.ReadFile(existing, p)
	}

	for _, p := range slices.Sorted(maps.Keys(choices)) {
		choice := choices[p]
		old, err := fs.ReadFile(existing, p)
		if err != nil {
			return nil, nil, fmt.Errorf("read %s: %w", p, err)
		}
		switch choice {
		case Keep:
			if hasMarkers(old) {
				return nil, nil, fmt.Errorf("%s %w; choose move or import", p, ErrKeepMarkers)
			}
			view.files[p] = wrap(old)
		case Move:
			dest := MovePath(topicDir, p)
			if _, err := fs.Stat(existing, dest); err == nil {
				return nil, nil, fmt.Errorf("cannot move %s to %s: %w; choose keep or import", p, dest, ErrMoveExists)
			}
			moved = append(moved, MovedFile{From: p, Path: dest, Content: old})
			view.files[p] = nil
			pointers = append(pointers, "- Before starting a task, read "+dest+", which holds the rules written for this project before cyanotype.")
		case Replace:
		case Import:
			return nil, nil, fmt.Errorf("%s: import writes nothing; build the prompt with Prompt instead", p)
		default:
			return nil, nil, fmt.Errorf("%s: unknown choice %q", p, choice)
		}
	}

	if len(pointers) > 0 {
		main, err := read(mainPath)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, nil, fmt.Errorf("read %s: %w", mainPath, err)
		}
		view.files[mainPath] = appendPointers(main, pointers)
	}
	return view, moved, nil
}

// wrap returns content as one protected block. Line endings become LF,
// as the renderer writes every protected block.
func wrap(content []byte) []byte {
	text := strings.TrimRight(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n")
	var b strings.Builder
	b.WriteString(render.CustomBegin + "\n")
	if text != "" {
		b.WriteString(text + "\n")
	}
	b.WriteString(render.CustomEnd + "\n")
	return []byte(b.String())
}

// appendPointers adds a protected block holding pointers under a "Further
// reading" heading to content, so the renderer puts it at the end of the
// generated Further reading section, or at the end of the file when the
// file has no such section.
func appendPointers(content []byte, pointers []string) []byte {
	var b bytes.Buffer
	b.Write(content)
	if len(content) > 0 && !bytes.HasSuffix(content, []byte("\n")) {
		b.WriteByte('\n')
	}
	b.WriteString("## Further reading\n" + render.CustomBegin + "\n")
	for _, p := range pointers {
		b.WriteString(p + "\n")
	}
	b.WriteString(render.CustomEnd + "\n")
	return b.Bytes()
}

// hasMarkers reports whether content has a line holding a protected block
// marker.
func hasMarkers(content []byte) bool {
	for line := range strings.Lines(string(content)) {
		if l := strings.TrimSpace(line); l == render.CustomBegin || l == render.CustomEnd {
			return true
		}
	}
	return false
}

// overlay is an fs.FS that reads files from a map before its base. A nil
// entry reads as a file that does not exist.
type overlay struct {
	base  fs.FS
	files map[string][]byte
}

// Open implements fs.FS.
func (o *overlay) Open(name string) (fs.File, error) {
	data, ok := o.files[name]
	switch {
	case !ok:
		if o.base == nil {
			return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
		}
		return o.base.Open(name)
	case data == nil:
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	return &memFile{name: path.Base(name), Reader: bytes.NewReader(data), size: int64(len(data))}, nil
}

// memFile is a file of an overlay, held in memory.
type memFile struct {
	*bytes.Reader
	name string
	size int64
}

func (f *memFile) Stat() (fs.FileInfo, error) { return memInfo{f.name, f.size}, nil }
func (f *memFile) Close() error               { return nil }

// memInfo describes a memFile.
type memInfo struct {
	name string
	size int64
}

func (i memInfo) Name() string       { return i.name }
func (i memInfo) Size() int64        { return i.size }
func (i memInfo) Mode() fs.FileMode  { return 0o644 }
func (i memInfo) ModTime() time.Time { return time.Time{} }
func (i memInfo) IsDir() bool        { return false }
func (i memInfo) Sys() any           { return nil }
