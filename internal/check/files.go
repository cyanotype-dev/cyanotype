package check

import (
	"bytes"
	"errors"
	"io/fs"

	"github.com/cyanotype-dev/cyanotype/internal/generate"
	"github.com/cyanotype-dev/cyanotype/internal/lock"
	"github.com/cyanotype-dev/cyanotype/internal/render"
)

// Paths of the files cyanotype merges into rather than generates whole.
// Their content belongs to the user except for cyanotype's rules or block,
// so check compares them as generate does and never looks for references
// in them.
const (
	settingsPath  = ".claude/settings.json"
	gitignorePath = ".gitignore"
)

// fileFindings compares each generated file with the disk, as generate
// would. For a Markdown file or the .gitignore block it also tells a hand
// edit from a change in the spec or the catalog, with the hash the lock
// recorded: the file on disk matches the lock when only the spec moved, and
// what generate would write matches it when only the file moved.
//
// Changes generate makes that lose nothing, such as moving a protected
// block back to the end of its section or turning CRLF into LF, are not
// drift: the part cyanotype owns is the same.
func fileFindings(changes []generate.Change, prev *lock.Lock) ([]Finding, error) {
	var (
		out     []Finding
		drifted bool
		lockCh  *generate.Change
	)
	for i, c := range changes {
		if c.Path == lock.Path {
			lockCh = &changes[i]
			continue
		}
		fd, ok, err := fileFinding(c, prev)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, fd)
			drifted = true
		}
	}
	switch {
	case lockCh == nil:
	case lockCh.Status == generate.Created:
		out = append(out, Finding{Kind: OutOfSync, Severity: Error, Path: lock.Path,
			Msg: "is missing, so check cannot tell hand edits from changes to cyanotype.yaml; run cyanotype generate and commit it"})
	case lockCh.Status == generate.Modified && !drifted:
		// With other drift, the lock changes too; saying so adds nothing.
		out = append(out, Finding{Kind: OutOfSync, Severity: Error, Path: lock.Path,
			Msg: "is out of date, though every generated file is current; run cyanotype generate"})
	}
	return out, nil
}

// fileFinding returns the finding for one generated file, and false when
// it is current.
func fileFinding(c generate.Change, prev *lock.Lock) (Finding, bool, error) {
	fd := Finding{Kind: OutOfSync, Severity: Error, Path: c.Path}
	switch {
	case c.Status == generate.Unchanged:
		return Finding{}, false, nil
	case c.HandWritten:
		fd.Kind = HandWritten
		fd.Msg = "was written by hand, and cyanotype generates this file; run cyanotype generate and choose what to do with it " +
			"(import, keep, move or replace)"
		return fd, true, nil
	case c.Status == generate.Created:
		fd.Msg = "is missing; run cyanotype generate to create it"
		return fd, true, nil
	case c.Status == generate.Deleted:
		fd.Msg = "is no longer generated and nobody changed it since; run cyanotype generate to delete it"
		return fd, true, nil
	case c.Path == settingsPath:
		fd.Msg = "does not hold the permission rules cyanotype generates; run cyanotype generate --dry-run to see the changes"
		return fd, true, nil
	}

	disk, ok, err := render.Owned(c.Path, c.Old)
	if err != nil {
		return Finding{}, false, err
	}
	if !ok {
		fd.Msg = "has no cyanotype block; run cyanotype generate to add it"
		return fd, true, nil
	}
	want, _, err := render.Owned(c.Path, c.New)
	if err != nil {
		return Finding{}, false, err
	}
	if bytes.Equal(disk, want) {
		return Finding{}, false, nil
	}

	recorded := ""
	if prev != nil {
		recorded = prev.Files[c.Path]
	}
	const edited = "was edited by hand since cyanotype generated it, outside its cyanotype:custom blocks"
	const undo = "; cyanotype generate would undo the edits, so move them into a cyanotype:custom block, or into custom_rules in cyanotype.yaml"
	switch recorded {
	case "":
		fd.Msg = "differs from what cyanotype generates, and no lock records it, so check cannot tell whether it was edited by hand; " +
			"review the changes with cyanotype generate --dry-run"
	case lock.Hash(disk):
		fd.Msg = "is out of sync with cyanotype.yaml or the catalog; run cyanotype generate"
	case lock.Hash(want):
		fd.Kind, fd.Msg = Edited, edited+undo
	default:
		fd.Kind, fd.Msg = Edited, edited+", and cyanotype.yaml or the catalog changed too"+undo
	}
	return fd, true, nil
}

// referenceFindings warns about generated files, as they are on disk, that
// send the agent to a file that does not exist, such as a project.brief
// nobody wrote yet. A file generate would create is not reported here: it
// is already out of sync.
func referenceFindings(changes []generate.Change, project fs.FS) []Finding {
	created := make(map[string]bool)
	for _, c := range changes {
		if c.Status == generate.Created {
			created[c.Path] = true
		}
	}
	var out []Finding
	for _, c := range changes {
		if c.Old == nil || c.HandWritten || c.Status == generate.Deleted ||
			c.Path == settingsPath || c.Path == gitignorePath || c.Path == lock.Path {
			continue
		}
		for _, ref := range render.References(c.Old) {
			if created[ref] {
				continue
			}
			if _, err := fs.Stat(project, ref); errors.Is(err, fs.ErrNotExist) {
				out = append(out, Finding{Kind: MissingReference, Severity: Warning, Path: c.Path,
					Msg: "points to " + ref + ", which does not exist"})
			}
		}
	}
	return out
}
