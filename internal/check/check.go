// Package check finds drift between a project and its cyanotype.yaml: the
// work of cyanotype check, which runs in a terminal, as a pre-commit hook
// or in CI. See docs/DESIGN.md, What the tool checks.
//
// It only reads. Findings are plain data, never printed, so the CLI can
// show them as lines today, and the --json output and the MCP check tool
// of v0.4 can encode the same values.
package check

import (
	"cmp"
	"io/fs"
	"slices"
	"strings"

	"github.com/cyanotype-dev/cyanotype/internal/catalog"
	"github.com/cyanotype-dev/cyanotype/internal/generate"
	"github.com/cyanotype-dev/cyanotype/internal/lock"
	"github.com/cyanotype-dev/cyanotype/internal/resolve"
	"github.com/cyanotype-dev/cyanotype/internal/spec"
)

// Severity says whether a finding fails a check.
type Severity int

// The severities of a Finding.
const (
	// Error is drift the project must fix: files that are not what
	// cyanotype generates, or a committed file that should be ignored.
	Error Severity = iota
	// Warning is worth a look but may be intended, such as a dependency
	// the spec does not select. cyanotype check --strict fails on it too.
	Warning
)

// String returns "error" or "warning".
func (s Severity) String() string {
	if s == Error {
		return "error"
	}
	return "warning"
}

// Kind names what a finding is about. Kinds are stable identifiers, part of
// the contract of the JSON output to come, while messages may change.
type Kind string

// The kinds of finding.
const (
	// OutOfSync: generate would create, change or delete the file,
	// because the spec or the catalog changed since it was generated.
	OutOfSync Kind = "out-of-sync"
	// Edited: someone changed a generated file outside its protected
	// blocks since cyanotype generated it; generate would undo the edit.
	Edited Kind = "edited"
	// HandWritten: an instruction file without the cyanotype header where
	// cyanotype generates one, waiting for a choice of what to do with it.
	HandWritten Kind = "hand-written"
	// TrackedIgnored: git tracks a file that an enforce.gitignore line of
	// a selected entry keeps out of git, such as a committed .env.
	TrackedIgnored Kind = "tracked-ignored"
	// Undeclared: detection finds an entry the spec does not select.
	Undeclared Kind = "undeclared"
	// Unreadable: a manifest detection could not read.
	Unreadable Kind = "unreadable"
	// CatalogOlder: the spec pins a catalog older than the bundled one.
	CatalogOlder Kind = resolve.CodeCatalogOlder
	// TooLong: a main file longer than output.max_lines.
	TooLong Kind = resolve.CodeTooLong
	// MissingReference: a generated file points to a file that does not
	// exist.
	MissingReference Kind = "missing-reference"
	// Stale: a file cyanotype no longer generates, kept because someone
	// changed it.
	Stale Kind = resolve.CodeStale
	// Unrecorded: a file with the cyanotype header that is not generated
	// and that no lock records.
	Unrecorded Kind = resolve.CodeUnrecorded
	// UnownedRules: rules in .claude/settings.json from before the lock,
	// treated as the user's.
	UnownedRules Kind = resolve.CodeUnownedRules
	// SpecWarning: any other warning about the spec, such as a target
	// cyanotype cannot generate files for yet.
	SpecWarning Kind = "spec-warning"
)

// Finding is one problem check found.
type Finding struct {
	Kind     Kind
	Severity Severity
	// Path is the file the finding is about, relative to the project root
	// with forward slashes, such as "AGENTS.md" or "cyanotype.yaml".
	Path string
	// Line is the 1-based line in Path, or 0 when the finding is about the
	// whole file.
	Line int
	// Field is the spec field, such as "catalog_version", for findings
	// about the spec; "" otherwise.
	Field string
	// Msg says what is wrong and what to do, in a sentence that follows
	// the path: "AGENTS.md: was edited by hand ...".
	Msg string
}

// Report is the result of a check.
type Report struct {
	// Findings are sorted by path, then line.
	Findings []Finding
	// Skipped names the checks that could not run, such as the one for
	// tracked files outside a git repository, with the reason. They are
	// not findings: nothing was found, and nothing was checked either.
	Skipped []string
}

// Count returns how many findings have severity s.
func (r Report) Count(s Severity) int {
	n := 0
	for _, f := range r.Findings {
		if f.Severity == s {
			n++
		}
	}
	return n
}

// Run checks the project held by project, whose spec is f, against the
// catalog c.
//
// The error is not a finding: it means the check could not run, such as a
// spec that does not resolve, whose errors are *spec.ValidationError
// values as generate.Preview returns them, or an invalid cyanotype.lock.
func Run(c *catalog.Catalog, f *spec.File, project fs.FS) (Report, error) {
	res, _, err := resolve.Resolve(c, f)
	if err != nil {
		return Report{}, err
	}
	changes, warnings, err := generate.Preview(c, f, project)
	if err != nil {
		return Report{}, err
	}
	prev, err := lock.Read(project)
	if err != nil {
		return Report{}, err
	}

	var r Report
	r.Findings = append(r.Findings, warningFindings(warnings, res, prev)...)
	files, err := fileFindings(changes, prev)
	if err != nil {
		return Report{}, err
	}
	r.Findings = append(r.Findings, files...)
	r.Findings = append(r.Findings, referenceFindings(changes, project)...)
	r.Findings = append(r.Findings, stackFindings(c, f, project)...)

	slices.SortStableFunc(r.Findings, func(a, b Finding) int {
		return cmp.Or(cmp.Compare(a.Path, b.Path), cmp.Compare(a.Line, b.Line))
	})
	return r, nil
}

// warningFindings turns the warnings of resolve and render into findings.
// The one about an older catalog pin says which selected entries changed,
// as far as the lock tells.
func warningFindings(warnings []resolve.Warning, res *resolve.Result, prev *lock.Lock) []Finding {
	var out []Finding
	for _, w := range warnings {
		fd := Finding{Kind: Kind(w.Code), Severity: Warning, Path: w.File, Line: w.Line, Field: w.Field, Msg: w.Msg}
		if w.Code == "" || w.Code == resolve.CodeUnsupportedTarget {
			fd.Kind = SpecWarning
		}
		if fd.Path == "" {
			fd.Path = generate.SpecFile
			if fd.Field != "" {
				fd.Msg = fd.Field + ": " + fd.Msg
			}
		}
		if w.Code == resolve.CodeCatalogOlder {
			fd.Msg += "; " + changedEntries(res, prev)
		}
		out = append(out, fd)
	}
	return out
}

// changedEntries says which selected entries the bundled catalog changed
// since the lock recorded them.
func changedEntries(res *resolve.Result, prev *lock.Lock) string {
	if prev == nil {
		return lock.Path + " is missing, so check cannot tell which selected entries changed"
	}
	var changed []string
	for _, e := range res.Entries {
		recorded, ok := prev.Entries[e.ID]
		if !ok {
			continue // selected after the last generation
		}
		raw, _ := res.Catalog.Get(e.ID)
		if now, err := lock.EntryHash(raw); err == nil && now != recorded {
			changed = append(changed, e.ID)
		}
	}
	if len(changed) == 0 {
		return "no selected entry changed since the files were generated"
	}
	return "selected entries changed since the files were generated: " + strings.Join(changed, ", ")
}
