package resolve

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/cyanotype-dev/cyanotype/internal/catalog"
	"github.com/cyanotype-dev/cyanotype/internal/spec"
)

// Result is a spec resolved against the catalog: everything a renderer
// needs to write instruction files, already checked.
type Result struct {
	// Spec is the spec with defaults applied, so renderers never have to
	// know the defaults themselves.
	Spec spec.Spec
	// CatalogVersion is the version of the catalog the result was resolved
	// with, which a spec without catalog_version can pin; empty for a
	// catalog without a manifest.
	CatalogVersion string
	// Catalog is the catalog the result was resolved with. Renderers read
	// its entries as the catalog writes them, placeholders unfilled, and
	// the rules of entries that are not selected, to tell the user about
	// rules an older cyanotype may have left behind.
	Catalog *catalog.Catalog
	// Entries are the selected entries in the order of spec.selections,
	// which is the order their text appears in generated files.
	Entries []Entry
}

// Entry is a selected catalog entry, ready to render.
type Entry struct {
	// Entry is the catalog entry, with the placeholders in Snippet.Rule and
	// Snippet.Why already filled. Its slices are shared with the catalog
	// and must not be modified.
	catalog.Entry
	// Locked reports whether the spec lists the entry in locked, so a
	// personal cyanotype.local.yaml may not weaken it.
	Locked bool
}

// Warning is a problem worth telling the user about that does not stop
// generation, such as a catalog_version pin older than the loaded catalog.
type Warning struct {
	// Code names the kind of warning, such as CodeCatalogOlder, for
	// interfaces that report warnings as data, such as cyanotype check; ""
	// for a kind nothing needs to tell apart. Codes are stable, while
	// messages may change.
	Code string
	// File is the project file the warning is about, such as
	// "docs/agents/git.md", or "" when it is about the spec.
	File string
	// Field is a YAML path such as "catalog_version".
	Field string
	// Line is the 1-based line of the field, or 0 when unknown.
	Line int
	Msg  string
}

// String formats w like a *spec.ValidationError: "line 3: field: message",
// or "path: message" for a warning about another file.
func (w Warning) String() string {
	if w.File != "" {
		return w.File + ": " + w.Msg
	}
	return (&spec.ValidationError{Field: w.Field, Line: w.Line, Msg: w.Msg}).Error()
}

// Codes of the warnings resolve and render return; see Warning.Code.
const (
	// CodeCatalogOlder: the spec pins a catalog older than the loaded one.
	CodeCatalogOlder = "catalog-older"
	// CodeUnsupportedTarget: a target cyanotype cannot generate files for
	// yet.
	CodeUnsupportedTarget = "unsupported-target"
	// CodeTooLong: a main file longer than output.max_lines.
	CodeTooLong = "too-long"
	// CodeStale: a file cyanotype generated earlier and no longer
	// generates, kept because someone changed it.
	CodeStale = "stale"
	// CodeUnrecorded: a file with the cyanotype header that is not
	// generated and that no cyanotype.lock records.
	CodeUnrecorded = "unrecorded"
	// CodeUnownedRules: permission rules found in .claude/settings.json
	// before any cyanotype.lock existed, which are treated as the user's.
	CodeUnownedRules = "unowned-rules"
)

// knownQuestions lists the answer keys a spec may use. Questions are not in
// the catalog yet; they will move there together with category metadata.
var knownQuestions = []string{"domain_notes"}

// Resolve checks the spec in f against c and returns the selected entries
// in a form renderers can use directly.
//
// Errors are *spec.ValidationError values joined with errors.Join, sorted
// by line, so callers handle them like the errors of spec.Load. When err is
// not nil the result is nil. Warnings are returned in both cases, so the
// user sees them together with any errors.
//
// Resolve runs spec.Validate first, so a spec built in code with spec.New
// gets the same checks as a loaded one.
func Resolve(c *catalog.Catalog, f *spec.File) (*Result, []Warning, error) {
	r := &resolver{cat: c, file: f, spec: f.Spec}
	if err := spec.Validate(f.Spec); err != nil {
		return nil, nil, r.addLines(err)
	}

	r.checkCatalogVersion()
	entries := r.resolveSelections()
	r.checkAnswers()

	if len(r.errs) > 0 {
		// Stable, so problems on one line keep the order they were found in.
		slices.SortStableFunc(r.errs, func(a, b *spec.ValidationError) int { return a.Line - b.Line })
		errs := make([]error, len(r.errs))
		for i, e := range r.errs {
			errs[i] = e
		}
		return nil, r.warnings, errors.Join(errs...)
	}
	return &Result{
		Spec:           f.Spec.WithDefaults(),
		CatalogVersion: c.Version(),
		Catalog:        c,
		Entries:        entries,
	}, r.warnings, nil
}

// resolver collects the problems found while resolving one spec.
type resolver struct {
	cat      *catalog.Catalog
	file     *spec.File
	spec     spec.Spec
	errs     []*spec.ValidationError
	warnings []Warning
}

func (r *resolver) errorf(field, format string, args ...any) {
	r.errs = append(r.errs, &spec.ValidationError{Field: field, Line: r.file.Line(field), Msg: fmt.Sprintf(format, args...)})
}

func (r *resolver) warnf(code, field, format string, args ...any) {
	r.warnings = append(r.warnings, Warning{Code: code, Field: field, Line: r.file.Line(field), Msg: fmt.Sprintf(format, args...)})
}

// addLines fills in the line of each error spec.Validate returned, which
// knows the fields but not the file.
func (r *resolver) addLines(err error) error {
	errs := []error{err}
	var joined interface{ Unwrap() []error }
	if errors.As(err, &joined) {
		errs = joined.Unwrap()
	}
	for _, e := range errs {
		var ve *spec.ValidationError
		if errors.As(e, &ve) && ve.Line == 0 && ve.Field != "" {
			ve.Line = r.file.Line(ve.Field)
		}
	}
	return errors.Join(errs...)
}

// checkCatalogVersion compares the spec's catalog_version pin with the
// loaded catalog. A newer pin is an error, as a newer spec version is: the
// spec may select entries this catalog does not have. An older pin is only
// a warning, because the bundled catalog is the only one available and
// refusing it would make every spec unusable after an upgrade.
func (r *resolver) checkCatalogVersion() {
	pin, loaded := r.spec.CatalogVersion, r.cat.Version()
	if pin == "" || loaded == "" {
		return // nothing pinned, or a catalog without a manifest
	}
	switch catalog.CompareVersions(pin, loaded) {
	case 1:
		r.errorf("catalog_version", "%q is newer than this cyanotype's catalog (%s); upgrade cyanotype", pin, loaded)
	case -1:
		// cyanotype update does not exist yet, so the message says how to
		// move the pin by hand.
		r.warnf(CodeCatalogOlder, "catalog_version", "the spec pins %s but this cyanotype's catalog is %s, whose entries may have changed; "+
			"review the changes with cyanotype generate --dry-run, then set catalog_version to %q", pin, loaded, loaded)
	}
}

// selected is a selection that exists in the catalog.
type selected struct {
	index int // position in spec.selections
	entry catalog.Entry
}

func selectionField(i int) string {
	return fmt.Sprintf("selections[%d]", i)
}

// resolveSelections checks every selection and returns the resolved
// entries. Unknown ids are reported once and left out of the other checks,
// so a typo does not cause a cascade of errors.
func (r *resolver) resolveSelections() []Entry {
	var sel []selected
	indexOf := make(map[string]int) // id -> position in spec.selections
	for i, id := range r.spec.Selections {
		e, ok := r.cat.Get(id)
		if !ok {
			r.errorf(selectionField(i), "%s", r.unknownEntry(id))
			continue
		}
		sel = append(sel, selected{index: i, entry: e})
		indexOf[id] = i
	}

	r.checkRadio(sel)
	for _, s := range sel {
		r.checkRequires(s, indexOf)
		r.checkConflicts(s, indexOf)
	}
	values := r.checkPlaceholders(sel)

	entries := make([]Entry, 0, len(sel))
	for _, s := range sel {
		e := s.entry
		e.Snippet.Rule = catalog.FillPlaceholders(e.Snippet.Rule, values)
		e.Snippet.Why = catalog.FillPlaceholders(e.Snippet.Why, values)
		entries = append(entries, Entry{Entry: e, Locked: slices.Contains(r.spec.Locked, e.ID)})
	}
	return entries
}

// unknownEntry explains a selection that is not in the catalog, with a
// suggestion when one is close enough.
func (r *resolver) unknownEntry(id string) string {
	msg := fmt.Sprintf("unknown entry %q", id)
	pin, loaded := r.spec.CatalogVersion, r.cat.Version()
	if pin != "" && loaded != "" && pin != loaded {
		msg += fmt.Sprintf(" in catalog %s (the spec pins %s; it may have been renamed or removed)", loaded, pin)
	}
	all := r.cat.Entries() // sorted by id, as suggest expects
	ids := make([]string, len(all))
	for i, e := range all {
		ids[i] = e.ID
	}
	if s, ok := suggest(id, ids); ok {
		msg += fmt.Sprintf("; did you mean %q?", s)
	}
	return msg
}

// checkRadio reports a second entry from the same radio category. Inside
// a radio category exclusivity is implicit, so the entries do not list
// each other in conflicts.
func (r *resolver) checkRadio(sel []selected) {
	first := make(map[string]selected) // radio category -> first selection in it
	for _, s := range sel {
		e := s.entry
		if e.Select != catalog.SelectRadio {
			continue
		}
		f, ok := first[e.Category]
		if !ok {
			first[e.Category] = s
			continue
		}
		r.errorf(selectionField(s.index), "%q and %q (%s) are both in radio category %q; keep only one",
			e.ID, f.entry.ID, selectionField(f.index), e.Category)
	}
}

// missing is a required entry that is not selected.
type missing struct {
	id      string
	through string // the unselected entry that requires it; empty when required directly
}

// missingGroup is a requires_one_of group none of whose alternatives is
// selected.
type missingGroup struct {
	ids     []string
	through string // the unselected entry that requires it; empty when required directly
}

// checkRequires reports what s requires but is not selected. Requires are
// transitive: selecting an entry means needing everything its requirements
// need too. The search only goes through entries that are not selected,
// because a selected entry reports its own missing requirements; that way
// each gap appears once for every selection that needs it.
//
// All missing requires are reported in one message, and each unmet
// requires_one_of group in a message of its own. The search never goes into
// the alternatives of an unmet group, because nobody has chosen one yet.
// Cycles, such as node requiring one of npm or pnpm and both requiring node,
// end because every entry is visited once and a group counts as met when
// any alternative is selected, s itself included.
func (r *resolver) checkRequires(s selected, indexOf map[string]int) {
	queue := make([]missing, 0, len(s.entry.Requires))
	for _, id := range s.entry.Requires {
		queue = append(queue, missing{id: id})
	}
	seen := map[string]bool{s.entry.ID: true} // also guards against cycles
	groups := unmetGroups(s.entry, "", indexOf)

	var gaps []missing
	for len(queue) > 0 {
		m := queue[0]
		queue = queue[1:]
		if seen[m.id] {
			continue
		}
		seen[m.id] = true
		if _, ok := indexOf[m.id]; ok {
			continue
		}
		gaps = append(gaps, m)
		// The catalog guarantees that requires only name existing entries.
		req, _ := r.cat.Get(m.id)
		for _, id := range req.Requires {
			queue = append(queue, missing{id: id, through: m.id})
		}
		groups = append(groups, unmetGroups(req, m.id, indexOf)...)
	}

	field := selectionField(s.index)
	if len(gaps) > 0 {
		names := make([]string, len(gaps))
		for i, g := range gaps {
			names[i] = fmt.Sprintf("%q", g.id) + throughSuffix(g.through)
		}
		verb := "is"
		if len(gaps) > 1 {
			verb = "are"
		}
		r.errorf(field, "%q requires %s, which %s not selected", s.entry.ID, joinNames(names), verb)
	}
	for _, g := range groups {
		names := make([]string, len(g.ids))
		for i, id := range g.ids {
			names[i] = fmt.Sprintf("%q", id)
		}
		r.errorf(field, "%q requires one of %s%s, but none is selected",
			s.entry.ID, joinList(names, "or"), throughSuffix(g.through))
	}
}

// unmetGroups returns the requires_one_of groups of e that have no selected
// alternative, marked as required through the entry named by through.
func unmetGroups(e catalog.Entry, through string, indexOf map[string]int) []missingGroup {
	var out []missingGroup
	for _, group := range e.RequiresOneOf {
		met := slices.ContainsFunc(group, func(id string) bool {
			_, ok := indexOf[id]
			return ok
		})
		if !met {
			out = append(out, missingGroup{ids: group, through: through})
		}
	}
	return out
}

// throughSuffix names the unselected entry a requirement comes through, or
// returns "" for a direct requirement.
func throughSuffix(through string) string {
	if through == "" {
		return ""
	}
	return fmt.Sprintf(" (through %q)", through)
}

// checkConflicts reports s when it conflicts with an earlier selection.
// Conflicts are declared on both sides, so checking only earlier
// selections reports each pair once, on the later entry.
func (r *resolver) checkConflicts(s selected, indexOf map[string]int) {
	for _, id := range s.entry.Conflicts {
		if j, ok := indexOf[id]; ok && j < s.index {
			r.errorf(selectionField(s.index), "%q conflicts with %q (%s); keep only one", s.entry.ID, id, selectionField(j))
		}
	}
}

// filler computes the value of one placeholder from the selected entries.
type filler struct {
	// fill returns the value, or false when the selection cannot provide one.
	fill func(sel []selected) (string, bool)
	// unfilled says why fill returned false, for the error message.
	unfilled string
}

// fillers has one filler for every name in catalog.Placeholders.
var fillers = map[string]filler{
	catalog.PlaceholderLanguages: {fill: languages, unfilled: `no entry of category "language" is selected`},
}

// languageCategory is the category whose entries fill {languages}.
const languageCategory = "language"

// languages joins the names of the selected languages, in selection order.
func languages(sel []selected) (string, bool) {
	var names []string
	for _, s := range sel {
		if s.entry.Category == languageCategory {
			names = append(names, s.entry.Name)
		}
	}
	return joinNames(names), len(names) > 0
}

// checkPlaceholders reports placeholders that selected entries use but the
// selection cannot fill, and returns the values of those it can.
func (r *resolver) checkPlaceholders(sel []selected) map[string]string {
	values := make(map[string]string)
	for name, f := range fillers {
		if v, ok := f.fill(sel); ok {
			values[name] = v
		}
	}

	for _, s := range sel {
		used := catalog.UsedPlaceholders(s.entry.Snippet.Rule)
		for _, name := range catalog.UsedPlaceholders(s.entry.Snippet.Why) {
			if !slices.Contains(used, name) {
				used = append(used, name)
			}
		}
		for _, name := range used {
			if _, ok := values[name]; !ok {
				r.errorf(selectionField(s.index), "%q uses {%s}, but %s", s.entry.ID, name, fillers[name].unfilled)
			}
		}
	}
	return values
}

// checkAnswers reports answer keys that match no known question.
func (r *resolver) checkAnswers() {
	for _, key := range slices.Sorted(maps.Keys(r.spec.Answers)) {
		if slices.Contains(knownQuestions, key) {
			continue
		}
		msg := fmt.Sprintf("unknown question %q", key)
		if s, ok := suggest(key, knownQuestions); ok {
			msg += fmt.Sprintf("; did you mean %q?", s)
		}
		r.errorf("answers."+key, "%s", msg)
	}
}

// joinNames joins names as an English list without a serial comma:
// "Go", "Go and TypeScript", "Go, Python and TypeScript".
func joinNames(names []string) string {
	return joinList(names, "and")
}

// joinList joins names as an English list whose last two items are linked
// by conj, without a serial comma: "a, b or c" for conj "or".
func joinList(names []string, conj string) string {
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	}
	return strings.Join(names[:len(names)-1], ", ") + " " + conj + " " + names[len(names)-1]
}
