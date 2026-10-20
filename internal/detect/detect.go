package detect

import (
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strings"

	"github.com/cyanotype-dev/cyanotype/internal/catalog"
)

// Manifests read by detection, at the project root.
const (
	PackageJSON  = "package.json"
	PyProject    = "pyproject.toml"
	Requirements = "requirements.txt"
	GoMod        = "go.mod"
)

// Evidence is one reason an entry was detected: a file that exists, or a
// dependency a manifest lists.
type Evidence struct {
	// File is relative to the project root, with forward slashes.
	File string
	// Dependency is the package the file lists, or "" when the file's
	// presence alone revealed the entry. Python names are normalized, and Go
	// modules keep their full path, such as "github.com/jackc/pgx/v5".
	Dependency string
}

// String returns the evidence as people read it: "pnpm-lock.yaml", or
// "package.json: next" for a dependency.
func (e Evidence) String() string {
	if e.Dependency == "" {
		return e.File
	}
	return e.File + ": " + e.Dependency
}

// Finding is one detected catalog entry with every reason it was detected.
type Finding struct {
	ID       string
	Evidence []Evidence
}

// Clash is a group of detected entries that cannot all be selected: two
// lockfiles revealing npm and pnpm, for example. Detection reports clashes
// rather than picking one, because only the user knows which is current.
type Clash struct {
	// IDs are the clashing entries, in catalog order.
	IDs []string
	// Category is the radio category the entries share, or "" when they are
	// in different categories and clash through their conflicts field.
	Category string
}

// Problem is a manifest detection could not read. It is not an error: the
// rest of the project is still inspected, and files whose presence alone
// reveals an entry still count.
type Problem struct {
	File string
	Err  error
}

// Error returns the problem as "file: reason".
func (p Problem) Error() string {
	return p.File + ": " + p.Err.Error()
}

// Result is what detection found in a project.
type Result struct {
	// Findings are the detected entries, in catalog order.
	Findings []Finding
	// Clashes lists detected entries that cannot be selected together.
	Clashes []Clash
	// Scripts are the names of the scripts in package.json, sorted.
	Scripts []string
	// Problems are manifests that could not be read.
	Problems []Problem
}

// IDs returns the ids of the detected entries, in catalog order.
func (r Result) IDs() []string {
	ids := make([]string, len(r.Findings))
	for i, f := range r.Findings {
		ids[i] = f.ID
	}
	return ids
}

// Find returns the finding of the entry with the given id.
func (r Result) Find(id string) (Finding, bool) {
	i := slices.IndexFunc(r.Findings, func(f Finding) bool { return f.ID == id })
	if i < 0 {
		return Finding{}, false
	}
	return r.Findings[i], true
}

// Empty reports whether detection found nothing to suggest.
func (r Result) Empty() bool {
	return len(r.Findings) == 0
}

// Detect inspects the project, rooted at the project folder, and returns
// the catalog entries it reveals. The CLI passes the project root's FS,
// tests an fstest.MapFS. Nothing is written.
//
// Only the root of the project is inspected: the files each entry lists,
// and the manifests at the root. Monorepos with a manifest per package are
// not covered yet.
func Detect(c *catalog.Catalog, project fs.FS) Result {
	var r Result
	m := readManifests(project, &r)

	for _, e := range c.Entries() {
		if e.Detect == nil {
			continue
		}
		var ev []Evidence
		for _, path := range e.Detect.Files {
			if exists(project, path) {
				ev = append(ev, Evidence{File: path})
			}
		}
		if d := e.Detect.Dependencies; d != nil {
			ev = append(ev, m.match(d)...)
		}
		if len(ev) > 0 {
			r.Findings = append(r.Findings, Finding{ID: e.ID, Evidence: ev})
		}
	}
	r.Clashes = clashes(c, r.IDs())
	return r
}

// exists reports whether path names a file or folder in the project.
func exists(project fs.FS, path string) bool {
	if !fs.ValidPath(path) {
		return false
	}
	_, err := fs.Stat(project, path)
	return err == nil
}

// manifests holds the dependencies listed by each manifest of the project.
type manifests struct {
	npm    []string
	pypi   map[string][]string // by file: pyproject.toml, requirements.txt
	goMods []string
}

// readManifests reads every manifest at the root of the project. A
// manifest that is missing is skipped; one that cannot be read or parsed
// becomes a Problem in r.
func readManifests(project fs.FS, r *Result) manifests {
	m := manifests{pypi: map[string][]string{}}
	read := func(name string, parse func([]byte) error) {
		data, err := fs.ReadFile(project, name)
		if errors.Is(err, fs.ErrNotExist) {
			return
		}
		if err == nil {
			err = parse(data)
		}
		if err != nil {
			r.Problems = append(r.Problems, Problem{File: name, Err: err})
		}
	}

	read(PackageJSON, func(data []byte) error {
		p, err := parsePackageJSON(data)
		m.npm, r.Scripts = p.dependencies, p.scripts
		return err
	})
	read(PyProject, func(data []byte) error {
		names, err := parsePyProject(data)
		m.pypi[PyProject] = names
		return err
	})
	read(Requirements, func(data []byte) error {
		m.pypi[Requirements] = parseRequirements(data)
		return nil
	})
	read(GoMod, func(data []byte) error {
		modules, err := parseGoMod(data)
		m.goMods = modules
		return err
	})
	return m
}

// match returns the evidence for the dependencies of d that the manifests
// list, in the order of d.
func (m manifests) match(d *catalog.Dependencies) []Evidence {
	var ev []Evidence
	for _, name := range d.NPM {
		if slices.Contains(m.npm, name) {
			ev = append(ev, Evidence{File: PackageJSON, Dependency: name})
		}
	}
	for _, file := range []string{PyProject, Requirements} {
		for _, name := range d.PyPI {
			if slices.Contains(m.pypi[file], name) {
				ev = append(ev, Evidence{File: file, Dependency: name})
			}
		}
	}
	for _, path := range d.Go {
		if i := slices.IndexFunc(m.goMods, func(mod string) bool { return modulePathMatches(mod, path) }); i >= 0 {
			ev = append(ev, Evidence{File: GoMod, Dependency: m.goMods[i]})
		}
	}
	return ev
}

// modulePathMatches reports whether module is path, or one of its major
// versions or subpaths: "github.com/jackc/pgx/v5" matches
// "github.com/jackc/pgx", "github.com/jackc/pgxpool" does not.
func modulePathMatches(module, path string) bool {
	return module == path || strings.HasPrefix(module, path+"/")
}

// clashes returns the groups of ids that cannot be selected together:
// several entries of one radio category, and pairs that declare a conflict.
func clashes(c *catalog.Catalog, ids []string) []Clash {
	var out []Clash
	byCategory := map[string][]string{}
	var categories []string
	for _, id := range ids {
		e, _ := c.Get(id)
		if e.Select != catalog.SelectRadio {
			continue
		}
		if _, ok := byCategory[e.Category]; !ok {
			categories = append(categories, e.Category)
		}
		byCategory[e.Category] = append(byCategory[e.Category], id)
	}
	for _, cat := range categories {
		if group := byCategory[cat]; len(group) > 1 {
			out = append(out, Clash{IDs: group, Category: cat})
		}
	}
	for i, a := range ids {
		e, _ := c.Get(a)
		for _, b := range ids[i+1:] {
			if slices.Contains(e.Conflicts, b) {
				out = append(out, Clash{IDs: []string{a, b}})
			}
		}
	}
	return out
}

// lineError is a parse error at a line of a manifest.
func lineError(line int, format string, args ...any) error {
	return fmt.Errorf("line %d: %s", line, fmt.Sprintf(format, args...))
}
