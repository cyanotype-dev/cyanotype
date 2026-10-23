package contract

import (
	"github.com/cyanotype-dev/cyanotype/internal/catalog"
	"github.com/cyanotype-dev/cyanotype/internal/detect"
)

// CodeUnreadable is the code of the warning about a manifest detection
// could not read, the same as the kind of the finding check reports.
const CodeUnreadable = "unreadable"

// Detect is the output of cyanotype detect --json and of the MCP
// detect_stack tool: the catalog entries the project's files suggest.
type Detect struct {
	Head
	// Findings are the detected entries, in catalog order.
	Findings []Detected `json:"findings,omitzero"`
	// Clashes are detected entries that cannot all be selected; only the
	// user knows which one the project uses.
	Clashes []Clash `json:"clashes,omitzero"`
	// Scripts are the names of the scripts in package.json, sorted.
	Scripts []string `json:"scripts,omitzero"`
	Diagnostics
}

// Detected is one detected entry with every reason it was detected.
type Detected struct {
	ID       string     `json:"id"`
	Name     string     `json:"name"`
	Evidence []Evidence `json:"evidence"`
}

// Evidence is a file that exists, or a dependency a manifest lists.
type Evidence struct {
	File string `json:"file"`
	// Dependency is the package the file lists; omitted when the file's
	// presence alone revealed the entry.
	Dependency string `json:"dependency,omitempty"`
}

// Clash is a group of detected entries that cannot all be selected.
type Clash struct {
	IDs []string `json:"ids"`
	// Category is the radio category the entries share; omitted when they
	// clash through their conflicts.
	Category string `json:"category,omitempty"`
}

// NewDetect returns the output for what detect.Detect found with the
// catalog c. Manifests it could not read are warnings.
func NewDetect(c *catalog.Catalog, r detect.Result) *Detect {
	out := &Detect{Findings: make([]Detected, 0, len(r.Findings)), Clashes: []Clash{}, Scripts: []string{}}
	for _, f := range r.Findings {
		e, _ := c.Get(f.ID)
		d := Detected{ID: f.ID, Name: e.Name, Evidence: make([]Evidence, 0, len(f.Evidence))}
		for _, ev := range f.Evidence {
			d.Evidence = append(d.Evidence, Evidence{File: ev.File, Dependency: ev.Dependency})
		}
		out.Findings = append(out.Findings, d)
	}
	for _, cl := range r.Clashes {
		out.Clashes = append(out.Clashes, Clash{IDs: cl.IDs, Category: cl.Category})
	}
	out.Scripts = append(out.Scripts, r.Scripts...)
	for _, p := range r.Problems {
		out.Warnings = append(out.Warnings, Diagnostic{Code: CodeUnreadable, Severity: SeverityWarning, File: p.File, Message: p.Err.Error()})
	}
	return out
}
