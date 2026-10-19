// Package generate turns a project's spec into files on disk. Preview
// computes what regeneration would change and shows it as a diff, without
// writing anything; Write then writes the changed files safely.
//
// Every interface shares it: the CLI's generate command asks for
// confirmation between the two steps, and the MCP tools preview and generate
// will call them the same way. Asking stays in the interfaces; this package
// never prints, reads stdin or exits.
package generate

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/cyanotype-dev/cyanotype/internal/catalog"
	"github.com/cyanotype-dev/cyanotype/internal/diff"
	"github.com/cyanotype-dev/cyanotype/internal/render"
	"github.com/cyanotype-dev/cyanotype/internal/resolve"
	"github.com/cyanotype-dev/cyanotype/internal/spec"
)

// SpecFile is the name of the spec at the project root.
const SpecFile = "cyanotype.yaml"

// ErrNoSpec reports a project folder without a cyanotype.yaml.
var ErrNoSpec = errors.New("no " + SpecFile)

// LoadSpec reads and validates the spec at the root of the project. A
// missing file is an error wrapping ErrNoSpec; problems in the file are the
// errors of spec.Load.
func LoadSpec(root *os.Root) (*spec.File, error) {
	f, err := root.Open(SpecFile)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("%w in %s", ErrNoSpec, root.Name())
	}
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", SpecFile, err)
	}
	defer f.Close()
	return spec.Load(f)
}

// Status says what writing a generated file does to the project.
type Status int

// The statuses of a Change.
const (
	// Unchanged means the file on disk already has the generated content,
	// so it is not written.
	Unchanged Status = iota
	// Created means the file does not exist yet.
	Created
	// Modified means the file exists with other content.
	Modified
)

// String returns the status as the CLI shows it: "unchanged", "created" or
// "modified".
func (s Status) String() string {
	switch s {
	case Unchanged:
		return "unchanged"
	case Created:
		return "created"
	case Modified:
		return "modified"
	}
	return fmt.Sprintf("Status(%d)", int(s))
}

// Change is one generated file compared with the file on disk.
type Change struct {
	// Path is relative to the project root, with forward slashes.
	Path   string
	Status Status
	// Old is the file on disk, nil when Status is Created.
	Old []byte
	// New is the generated content, which Write puts on disk.
	New []byte
	// CRLF reports that writing the file turns its CRLF line endings into
	// LF. It is set only for a Modified file: one that differs from the
	// generated content in its line endings alone is Unchanged.
	CRLF bool
}

// Diff returns the change as a unified diff with "a/" and "b/" prefixes, as
// git shows it, or "" for an unchanged file. Line endings are compared as
// LF, so a file with CRLF endings shows only the lines that really change.
func (c Change) Diff() string {
	if c.Status == Unchanged {
		return ""
	}
	oldName := "a/" + c.Path
	if c.Status == Created {
		oldName = "/dev/null"
	}
	return diff.Unified(oldName, "b/"+c.Path, toLF(c.Old), toLF(c.New))
}

// Preview resolves the spec in f against c, renders the files and compares
// each one with the project as it is now, held by existing: the CLI passes
// the project root's FS, tests an fstest.MapFS, and nil means an empty
// project. Nothing is written.
//
// The changes come sorted by path, unchanged files included, so interfaces
// can list them all. Errors are those of resolve.Resolve and render.Render:
// *spec.ValidationError values with field and line for problems in the
// spec. Warnings are returned with errors too, and get the line of their
// field when they name one, so the user sees where each comes from.
func Preview(c *catalog.Catalog, f *spec.File, existing fs.FS) ([]Change, []resolve.Warning, error) {
	res, warnings, err := resolve.Resolve(c, f)
	if err != nil {
		return nil, warnings, err
	}
	files, renderWarnings, err := render.Render(res, existing)
	for _, w := range renderWarnings {
		if w.Line == 0 && w.Field != "" {
			w.Line = f.Line(w.Field)
		}
		warnings = append(warnings, w)
	}
	if err != nil {
		return nil, warnings, err
	}

	changes := make([]Change, 0, len(files))
	for _, file := range files {
		ch, err := compare(existing, file)
		if err != nil {
			return nil, warnings, err
		}
		changes = append(changes, ch)
	}
	return changes, warnings, nil
}

// compare builds the Change for one generated file.
func compare(existing fs.FS, file render.File) (Change, error) {
	ch := Change{Path: file.Path, New: file.Content, Status: Created}
	if existing == nil {
		return ch, nil
	}
	old, err := fs.ReadFile(existing, file.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return ch, nil
	}
	if err != nil {
		return Change{}, fmt.Errorf("read %s: %w", file.Path, err)
	}

	ch.Old = old
	// A file that matches once CRLF is read as LF is left alone: with
	// core.autocrlf, the default of Git for Windows, every checkout has CRLF
	// endings, and rewriting them all on each run would only churn files.
	if bytes.Equal(toLF(old), toLF(file.Content)) {
		ch.Status = Unchanged
		return ch, nil
	}
	ch.Status = Modified
	ch.CRLF = hasCRLF(old) && !hasCRLF(file.Content)
	return ch, nil
}

func hasCRLF(b []byte) bool {
	return bytes.Contains(b, []byte("\r\n"))
}

// toLF turns CRLF line endings into LF.
func toLF(b []byte) []byte {
	if !hasCRLF(b) {
		return b
	}
	return bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
}
