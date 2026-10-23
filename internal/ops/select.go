package ops

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"

	"github.com/cyanotype-dev/cyanotype/internal/catalog"
	"github.com/cyanotype-dev/cyanotype/internal/contract"
	"github.com/cyanotype-dev/cyanotype/internal/diff"
	"github.com/cyanotype-dev/cyanotype/internal/generate"
	"github.com/cyanotype-dev/cyanotype/internal/resolve"
	"github.com/cyanotype-dev/cyanotype/internal/spec"
)

// SpecChange is a new cyanotype.yaml waiting to be written.
type SpecChange struct {
	// Read is the file the change started from, which must still be on
	// disk when it is written.
	Read []byte
	// Data is the new file.
	Data []byte
}

// Select changes the selections of the spec at root: it appends add, in
// order, and removes remove, then writes the spec back as spec.Save does,
// keeping comments, once it resolves against c. Nothing is written: the
// change is returned for WriteSelect, or nil when the spec already has
// those selections or the change has errors.
//
// A spec that would not resolve is refused, so generate keeps working:
// the errors, with the lines of the new file, say what is missing, and a
// second call can add it. Removing a locked selection is an error, since a
// change of choices never removes a locked rule; removing an entry that is
// not selected is a warning. A spec in simple mode moves to advanced mode,
// because simple mode could replace selections chosen one by one.
func Select(c *catalog.Catalog, root *os.Root, add, remove []string) (*contract.Select, *SpecChange) {
	if len(add) == 0 && len(remove) == 0 {
		return failure(contract.Error(contract.CodeUsage, "nothing to change: name entries to add or to remove")), nil
	}
	for _, id := range add {
		if slices.Contains(remove, id) {
			return failure(contract.Error(contract.CodeUsage, fmt.Sprintf("%q is both added and removed", id))), nil
		}
	}
	read, err := root.ReadFile(generate.SpecFile)
	if errors.Is(err, fs.ErrNotExist) {
		return &contract.Select{Diagnostics: failed(fmt.Errorf("%w in %s", generate.ErrNoSpec, root.Name()))}, nil
	}
	if err != nil {
		return &contract.Select{Diagnostics: failed(fmt.Errorf("read %s: %w", generate.SpecFile, err))}, nil
	}
	f, err := spec.Load(bytes.NewReader(read))
	if err != nil {
		return &contract.Select{Diagnostics: failed(err)}, nil
	}

	out := &contract.Select{Added: []string{}, Removed: []string{}, Written: new(false)}
	s := &f.Spec
	for _, id := range compact(remove) {
		switch {
		case !slices.Contains(s.Selections, id):
			out.Warnings = append(out.Warnings, contract.Diagnostic{Code: spec.CodeNotSelected, Severity: contract.SeverityWarning,
				File: generate.SpecFile, Field: "selections", Message: fmt.Sprintf("%q is not selected, so there is nothing to remove", id)})
		case slices.Contains(s.Locked, id):
			out.Errors = append(out.Errors, contract.Diagnostic{Code: contract.CodeLocked, Severity: contract.SeverityError,
				File: generate.SpecFile, Line: f.Line("locked"), Field: "locked",
				Message: fmt.Sprintf("%q is locked, so it cannot be removed; only a person editing cyanotype.yaml can unlock it", id)})
		default:
			s.Selections = slices.DeleteFunc(s.Selections, func(sel string) bool { return sel == id })
			out.Removed = append(out.Removed, id)
		}
	}
	for _, id := range compact(add) {
		if !slices.Contains(s.Selections, id) {
			s.Selections = append(s.Selections, id)
			out.Added = append(out.Added, id)
		}
	}
	if len(out.Errors) > 0 {
		return &contract.Select{Diagnostics: out.Diagnostics}, nil
	}
	if len(out.Added) == 0 && len(out.Removed) == 0 {
		return out, nil
	}
	if s.Mode == spec.ModeSimple {
		s.Mode = spec.ModeAdvanced
	}

	// Resolve before Save, which rewrites the document: errors then name
	// the lines of the file as it is now, and an added entry the line of
	// selections.
	_, warnings, err := resolve.Resolve(c, f)
	out.Warnings = append(out.Warnings, contract.Warnings(warnings)...)
	if err != nil {
		return &contract.Select{Diagnostics: contract.Diagnostics{Warnings: out.Warnings, Errors: contract.Errors(err)}}, nil
	}
	var buf bytes.Buffer
	if err := f.Save(&buf); err != nil {
		return &contract.Select{Diagnostics: contract.Diagnostics{Warnings: out.Warnings, Errors: contract.Errors(err)}}, nil
	}
	out.SpecDiff = diff.Unified("a/"+generate.SpecFile, "b/"+generate.SpecFile,
		bytes.ReplaceAll(read, []byte("\r\n"), []byte("\n")), buf.Bytes())
	return out, &SpecChange{Read: read, Data: buf.Bytes()}
}

// WriteSelect writes the change Select returned with out, and records in
// out that it did, or the error that stopped it.
func WriteSelect(root *os.Root, ch *SpecChange, out *contract.Select) *contract.Select {
	if err := generate.WriteSpec(root, ch.Data, ch.Read); err != nil {
		return &contract.Select{Diagnostics: contract.Diagnostics{Warnings: out.Warnings, Errors: contract.Errors(err)}}
	}
	out.Written = new(true)
	return out
}

// failure returns the output of a change that cannot be made.
func failure(d contract.Diagnostic) *contract.Select {
	return &contract.Select{Diagnostics: contract.Diagnostics{Errors: []contract.Diagnostic{d}}}
}

// compact returns ids without repeats, in their first order.
func compact(ids []string) []string {
	var out []string
	for _, id := range ids {
		if !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	return out
}
