package spec

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strconv"

	"go.yaml.in/yaml/v3"
)

// maxFileSize bounds how much Load reads. A spec is a few kilobytes; the
// limit only protects against reading a wrong file whole.
const maxFileSize = 1 << 20

// File is a spec together with the YAML document it was loaded from. The
// document keeps the user's comments and key order, so Save can write the
// spec back without losing them.
type File struct {
	// Spec holds the choices. Change it, then call Save.
	Spec Spec
	doc  *yaml.Node // nil for a file created with New
}

// New returns a File for a spec that does not exist on disk yet.
func New(s Spec) *File {
	return &File{Spec: s}
}

// Load reads a spec file, upgrades it to CurrentVersion when it is older,
// and validates its structure. Unknown fields are errors. All problems are
// returned together, joined with errors.Join; each one is a
// *ValidationError unless reading r failed.
//
// Load checks the file on its own. Whether the selected entries exist and
// fit together needs the catalog and is checked by internal/resolve.
func Load(r io.Reader) (*File, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxFileSize+1))
	if err != nil {
		return nil, fmt.Errorf("read spec: %w", err)
	}
	if len(data) > maxFileSize {
		return nil, &ValidationError{Msg: "file is larger than 1 MiB"}
	}

	doc, errs := parse(data)
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	// The version is read from the tree before decoding into Go types: a
	// file from another version may have fields these types do not know.
	version, errs := readVersion(doc)
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	if version < CurrentVersion {
		if err := migrate(doc, version, CurrentVersion, migrations); err != nil {
			return nil, err
		}
		// Decode the migrated tree, not the original bytes.
		if data, err = yaml.Marshal(doc); err != nil {
			return nil, fmt.Errorf("encode migrated spec: %w", err)
		}
	}

	var s Spec
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&s); err != nil {
		return nil, errors.Join(decodeErrors(doc, err)...)
	}

	if errs := validate(s); len(errs) > 0 {
		return nil, errors.Join(withLines(doc, errs)...)
	}
	return &File{Spec: s, doc: doc}, nil
}

// parse reads exactly one YAML document whose top level is a mapping.
func parse(data []byte) (*yaml.Node, []error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var doc yaml.Node
	err := dec.Decode(&doc)
	if errors.Is(err, io.EOF) || (err == nil && len(doc.Content) == 0) {
		return nil, []error{&ValidationError{Msg: "file is empty"}}
	}
	if err != nil {
		return nil, syntaxErrors(err)
	}

	var extra yaml.Node
	switch err := dec.Decode(&extra); {
	case errors.Is(err, io.EOF):
	case err != nil:
		return nil, syntaxErrors(err)
	default:
		return nil, []error{&ValidationError{Line: extra.Line, Msg: "a file must hold exactly one YAML document"}}
	}

	if top := root(&doc); top.Kind != yaml.MappingNode {
		return nil, []error{&ValidationError{Line: top.Line, Msg: "the top level must be a mapping of keys to values"}}
	}
	if errs := rejectAliases(&doc); len(errs) > 0 {
		return nil, errs
	}
	return &doc, nil
}

// rejectAliases reports anchors, aliases and merge keys. Save updates the
// tree node by node, and a node shared through an alias cannot be updated
// in one place only; a spec is small enough to write every value out.
func rejectAliases(doc *yaml.Node) []error {
	var errs []error
	var walk func(n *yaml.Node)
	walk = func(n *yaml.Node) {
		msg := ""
		switch {
		case n.Kind == yaml.AliasNode:
			msg = "aliases are not supported; write the value out"
		case n.Anchor != "":
			msg = "anchors are not supported; write the value out"
		case n.Tag == "!!merge":
			msg = "merge keys are not supported; write the values out"
		}
		if msg != "" {
			errs = append(errs, &ValidationError{Field: fieldAtLine(doc, n.Line), Line: n.Line, Msg: msg})
			return
		}
		for _, c := range n.Content {
			walk(c)
		}
	}
	walk(doc)
	return errs
}

// readVersion returns the version field of the document, rejecting
// versions this build cannot read.
func readVersion(doc *yaml.Node) (int, []error) {
	k, v := mappingPair(root(doc), "version")
	if k == nil {
		return 0, []error{&ValidationError{Field: "version", Msg: "is required"}}
	}
	fail := func(format string, args ...any) (int, []error) {
		return 0, []error{&ValidationError{Field: "version", Line: k.Line, Msg: fmt.Sprintf(format, args...)}}
	}

	if v.Kind != yaml.ScalarNode || v.ShortTag() != "!!int" {
		return fail("must be an integer, got %q", v.Value)
	}
	version, err := strconv.Atoi(v.Value)
	switch {
	case err != nil, version < 1:
		return fail("%q is not a valid version", v.Value)
	case version > CurrentVersion:
		return fail("version %d is newer than this cyanotype supports (%d); upgrade cyanotype", version, CurrentVersion)
	}
	return version, nil
}

// A migration upgrades a spec document by one version, in place. It works
// on the node tree rather than on Go types, so the user's comments survive,
// and no Go types of old versions have to be kept around.
type migration func(doc *yaml.Node) error

// migrations maps a version to the step that upgrades it to the next one.
// It is empty while version 1 is the only version.
var migrations = map[int]migration{}

// migrate runs the steps from version from up to version to, then records
// the new version in the document. The file on disk changes only when the
// spec is saved.
func migrate(doc *yaml.Node, from, to int, steps map[int]migration) error {
	for v := from; v < to; v++ {
		step, ok := steps[v]
		if !ok {
			return fmt.Errorf("no migration from spec version %d to %d", v, v+1)
		}
		if err := step(doc); err != nil {
			return fmt.Errorf("migrate spec from version %d to %d: %w", v, v+1, err)
		}
	}
	if _, v := mappingPair(root(doc), "version"); v != nil {
		v.Value, v.Tag, v.Style = strconv.Itoa(to), "!!int", 0
	}
	return nil
}
