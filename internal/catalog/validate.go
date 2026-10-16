package catalog

import (
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
)

// kebabCase matches ids, categories and topics: lowercase letters and digits
// in groups joined by single hyphens.
var kebabCase = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

var (
	selects = []Select{SelectRadio, SelectCheckbox}
	levels  = []Level{LevelBeginner, LevelIntermediate, LevelAdvanced}
)

// validateEntry checks the fields of one entry on its own.
func validateEntry(name string, e Entry) []error {
	var errs []error
	add := func(field, format string, args ...any) {
		errs = append(errs, &ValidationError{File: name, Field: field, Msg: fmt.Sprintf(format, args...)})
	}

	required := []struct{ field, value string }{
		{"id", e.ID},
		{"name", e.Name},
		{"category", e.Category},
		{"select", string(e.Select)},
		{"summary", e.Summary},
		{"snippet.rule", e.Snippet.Rule},
		{"snippet.why", e.Snippet.Why},
	}
	for _, r := range required {
		if strings.TrimSpace(r.value) == "" {
			add(r.field, "is required")
		}
	}

	if e.ID != "" {
		switch {
		case !kebabCase.MatchString(e.ID):
			add("id", "%q must be kebab-case (lowercase letters, digits and single hyphens)", e.ID)
		case name != e.ID+".yaml":
			add("id", "%q does not match the file name; rename the file to %s.yaml", e.ID, e.ID)
		}
	}
	if e.Category != "" && !kebabCase.MatchString(e.Category) {
		add("category", "%q must be kebab-case (lowercase letters, digits and single hyphens)", e.Category)
	}
	if e.Topic != "" && !kebabCase.MatchString(e.Topic) {
		add("topic", "%q must be kebab-case (lowercase letters, digits and single hyphens)", e.Topic)
	}
	if e.Select != "" && !slices.Contains(selects, e.Select) {
		add("select", "must be %q or %q, got %q", SelectRadio, SelectCheckbox, e.Select)
	}
	if e.Level != "" && !slices.Contains(levels, e.Level) {
		add("level", "must be %q, %q or %q, got %q", LevelBeginner, LevelIntermediate, LevelAdvanced, e.Level)
	}
	if e.URL != "" {
		if u, err := url.Parse(e.URL); err != nil || u.Scheme != "https" || u.Host == "" {
			add("url", "%q must be an absolute https URL", e.URL)
		}
	}

	errs = append(errs, validateRefs(name, e.ID, "requires", e.Requires)...)
	errs = append(errs, validateRefs(name, e.ID, "conflicts", e.Conflicts)...)
	for i, id := range e.Conflicts {
		if id != "" && slices.Contains(e.Requires, id) {
			add(fmt.Sprintf("conflicts[%d]", i), "%q is also listed in requires", id)
		}
	}

	if e.Enforce != nil && e.Enforce.Claude != nil {
		errs = append(errs, validateNotBlank(name, "enforce.claude.deny", e.Enforce.Claude.Deny)...)
	}
	if e.Detect != nil {
		errs = append(errs, validateNotBlank(name, "detect.files", e.Detect.Files)...)
	}
	return errs
}

// validateRefs checks a list of entry ids for blanks, self-references and duplicates.
// Whether the ids exist is checked by validateCatalog.
func validateRefs(name, self, field string, ids []string) []error {
	errs := validateNotBlank(name, field, ids)
	for i, id := range ids {
		f := fmt.Sprintf("%s[%d]", field, i)
		switch {
		case strings.TrimSpace(id) == "":
			// Reported by validateNotBlank.
		case id == self:
			errs = append(errs, &ValidationError{File: name, Field: f, Msg: "an entry cannot reference itself"})
		case slices.Index(ids, id) < i:
			errs = append(errs, &ValidationError{File: name, Field: f, Msg: fmt.Sprintf("%q is listed twice", id)})
		}
	}
	return errs
}

func validateNotBlank(name, field string, values []string) []error {
	var errs []error
	for i, v := range values {
		if strings.TrimSpace(v) == "" {
			errs = append(errs, &ValidationError{File: name, Field: fmt.Sprintf("%s[%d]", field, i), Msg: "is empty"})
		}
	}
	return errs
}

// validateCatalog checks rules that span entries: unique ids, references to
// existing entries, and conflicts that cross categories and are symmetric.
func validateCatalog(files []file) []error {
	var errs []error
	byID := make(map[string]file, len(files))
	for _, f := range files {
		id := f.entry.ID
		if id == "" {
			continue
		}
		if first, ok := byID[id]; ok {
			errs = append(errs, &ValidationError{File: f.name, Field: "id", Msg: fmt.Sprintf("%q is already used by %s", id, first.name)})
			continue
		}
		byID[id] = f
	}

	for _, f := range files {
		e := f.entry
		for i, id := range e.Requires {
			if _, ok := byID[id]; !ok && isRef(e.Requires, i, e.ID) {
				errs = append(errs, &ValidationError{File: f.name, Field: fmt.Sprintf("requires[%d]", i), Msg: fmt.Sprintf("unknown entry %q", id)})
			}
		}
		for i, id := range e.Conflicts {
			if !isRef(e.Conflicts, i, e.ID) {
				continue
			}
			field := fmt.Sprintf("conflicts[%d]", i)
			other, ok := byID[id]
			switch {
			case !ok:
				errs = append(errs, &ValidationError{File: f.name, Field: field, Msg: fmt.Sprintf("unknown entry %q", id)})
			case sameRadioCategory(e, other.entry):
				errs = append(errs, &ValidationError{File: f.name, Field: field, Msg: fmt.Sprintf(
					"%q is in the same radio category %q; exclusivity inside a radio category is implicit, so remove it", id, e.Category)})
			case !slices.Contains(other.entry.Conflicts, e.ID):
				errs = append(errs, &ValidationError{File: other.name, Field: "conflicts", Msg: fmt.Sprintf(
					"missing %q: %s declares a conflict with %q, and conflicts must be listed on both sides", e.ID, f.name, id)})
			}
		}
	}
	return errs
}

// isRef reports whether ids[i] is a reference validateEntry did not already
// reject as blank, a self-reference or a duplicate.
func isRef(ids []string, i int, self string) bool {
	id := ids[i]
	return strings.TrimSpace(id) != "" && id != self && slices.Index(ids, id) == i
}

func sameRadioCategory(a, b Entry) bool {
	return a.Category == b.Category && (a.Select == SelectRadio || b.Select == SelectRadio)
}
