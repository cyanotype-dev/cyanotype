package check

import (
	"fmt"
	"io/fs"
	"slices"

	"github.com/cyanotype-dev/cyanotype/internal/catalog"
	"github.com/cyanotype-dev/cyanotype/internal/detect"
	"github.com/cyanotype-dev/cyanotype/internal/spec"
)

// stackFindings warns about entries detection finds in the project that
// the spec does not select, such as a dependency added after the spec was
// written. When the spec selects another entry of the same radio category,
// or one the detected entry conflicts with, the finding says so: a
// package-lock.json next to a spec that selects pnpm is more likely a
// leftover than a second package manager.
func stackFindings(c *catalog.Catalog, f *spec.File, project fs.FS) []Finding {
	found := detect.Detect(c, project)
	selected := f.Spec.Selections

	var out []Finding
	for _, d := range found.Findings {
		if slices.Contains(selected, d.ID) || len(d.Evidence) == 0 {
			continue
		}
		e, ok := c.Get(d.ID)
		if !ok {
			continue
		}
		ev := d.Evidence[0]
		fd := Finding{Kind: Undeclared, Severity: Warning, Path: ev.File}
		other, clash := selectedRival(c, e, selected)
		switch {
		case clash && ev.Dependency == "":
			fd.Msg = fmt.Sprintf("suggests %s, but %s selects %s; delete the file if it is a leftover", e.Name, specName, other.Name)
		case clash:
			fd.Msg = fmt.Sprintf("lists %s, which suggests %s, but %s selects %s; remove the dependency if it is a leftover",
				ev.Dependency, e.Name, specName, other.Name)
		case ev.Dependency == "":
			fd.Msg = fmt.Sprintf("suggests %s, which %s does not select; add %s to selections if the project uses it", e.Name, specName, e.ID)
		default:
			fd.Msg = fmt.Sprintf("lists %s, which suggests %s, and %s does not select it; add %s to selections",
				ev.Dependency, e.Name, specName, e.ID)
		}
		out = append(out, fd)
	}
	for _, p := range found.Problems {
		out = append(out, Finding{Kind: Unreadable, Severity: Warning, Path: p.File,
			Msg: fmt.Sprintf("could not be read to detect the stack: %v", p.Err)})
	}
	return out
}

// specName is how messages name the spec.
const specName = "cyanotype.yaml"

// selectedRival returns a selected entry that cannot be selected together
// with e: one of e's radio category, or one e conflicts with.
func selectedRival(c *catalog.Catalog, e catalog.Entry, selected []string) (catalog.Entry, bool) {
	for _, id := range selected {
		s, ok := c.Get(id)
		if !ok {
			continue
		}
		if (e.Select == catalog.SelectRadio && s.Category == e.Category) || slices.Contains(e.Conflicts, id) {
			return s, true
		}
	}
	return catalog.Entry{}, false
}
