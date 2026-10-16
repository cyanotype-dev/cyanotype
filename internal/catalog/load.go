package catalog

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Catalog is a loaded and validated set of entries.
type Catalog struct {
	entries []Entry // sorted by id
	byID    map[string]int
}

// Entries returns all entries sorted by id.
func (c *Catalog) Entries() []Entry {
	return slices.Clone(c.entries)
}

// Get returns the entry with the given id.
func (c *Catalog) Get(id string) (Entry, bool) {
	i, ok := c.byID[id]
	if !ok {
		return Entry{}, false
	}
	return c.entries[i], true
}

// ValidationError describes one problem in one catalog file.
type ValidationError struct {
	File  string // file name inside the catalog folder
	Field string // YAML path such as "snippet.why" or "requires[1]"; empty for file-level problems
	Msg   string
}

func (e *ValidationError) Error() string {
	if e.Field == "" {
		return e.File + ": " + e.Msg
	}
	return e.File + ": " + e.Field + ": " + e.Msg
}

// file is an entry together with the file it was decoded from.
type file struct {
	name  string
	entry Entry
}

// Load reads every entry at the top level of fsys and validates them.
// Entries are .yaml files in one flat folder; dotfiles are ignored and
// anything else is an error, so nothing is silently skipped.
// All problems are returned together, joined with errors.Join; each one is a
// *ValidationError unless reading the folder itself failed.
func Load(fsys fs.FS) (*Catalog, error) {
	dirEntries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("read catalog folder: %w", err)
	}

	var (
		files []file
		errs  []error
	)
	decodeFailed := false
	for _, d := range dirEntries {
		name := d.Name()
		switch {
		case strings.HasPrefix(name, "."):
			continue
		case d.IsDir():
			errs = append(errs, &ValidationError{File: name, Msg: "subfolders are not allowed; entries live in one flat folder"})
			continue
		case path.Ext(name) != ".yaml":
			errs = append(errs, &ValidationError{File: name, Msg: "only .yaml files are allowed"})
			continue
		}

		data, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}
		e, decodeErrs := decode(name, data)
		if len(decodeErrs) > 0 {
			errs = append(errs, decodeErrs...)
			decodeFailed = true
			continue
		}
		files = append(files, file{name: name, entry: e})
	}

	for _, f := range files {
		errs = append(errs, validateEntry(f.name, f.entry)...)
	}
	// Cross-entry checks need every id: with a file that failed to decode
	// they would report references to it as unknown.
	if !decodeFailed {
		errs = append(errs, validateCatalog(files)...)
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	c := &Catalog{byID: make(map[string]int, len(files))}
	for _, f := range files {
		c.entries = append(c.entries, f.entry)
	}
	slices.SortFunc(c.entries, func(a, b Entry) int { return strings.Compare(a.ID, b.ID) })
	for i, e := range c.entries {
		c.byID[e.ID] = i
	}
	return c, nil
}

var unknownField = regexp.MustCompile(`^(line \d+): field (\S+) not found in type `)

// decode parses exactly one YAML document into an Entry, rejecting unknown fields.
func decode(name string, data []byte) (Entry, []error) {
	var e Entry
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)

	err := dec.Decode(&e)
	if errors.Is(err, io.EOF) {
		return Entry{}, []error{&ValidationError{File: name, Msg: "file is empty"}}
	}
	if err != nil {
		return Entry{}, decodeErrors(name, err)
	}

	var extra yaml.Node
	switch err := dec.Decode(&extra); {
	case errors.Is(err, io.EOF):
		return e, nil
	case err != nil:
		return Entry{}, decodeErrors(name, err)
	default:
		return Entry{}, []error{&ValidationError{File: name, Msg: "a file must hold exactly one YAML document"}}
	}
}

// decodeErrors turns a YAML error into one ValidationError per problem,
// rewording messages that mention Go types.
func decodeErrors(name string, err error) []error {
	var typeErr *yaml.TypeError
	if !errors.As(err, &typeErr) {
		return []error{&ValidationError{File: name, Msg: "invalid YAML: " + strings.TrimPrefix(err.Error(), "yaml: ")}}
	}
	errs := make([]error, 0, len(typeErr.Errors))
	for _, msg := range typeErr.Errors {
		if m := unknownField.FindStringSubmatch(msg); m != nil {
			msg = fmt.Sprintf("%s: unknown field %q", m[1], m[2])
		}
		errs = append(errs, &ValidationError{File: name, Msg: msg})
	}
	return errs
}
