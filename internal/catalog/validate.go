package catalog

import (
	"fmt"
	"maps"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"unicode"
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
	for i, group := range e.RequiresOneOf {
		field := fmt.Sprintf("requires_one_of[%d]", i)
		if len(group) < 2 {
			add(field, "needs at least two alternatives; move a single id to requires")
		}
		errs = append(errs, validateRefs(name, e.ID, field, group)...)
		for j, id := range group {
			if !isRef(group, j, e.ID) {
				continue
			}
			// An alternative that is also required, or forbidden, makes the
			// group meaningless.
			f := fmt.Sprintf("%s[%d]", field, j)
			if slices.Contains(e.Requires, id) {
				add(f, "%q is also listed in requires", id)
			}
			if slices.Contains(e.Conflicts, id) {
				add(f, "%q is also listed in conflicts", id)
			}
		}
	}

	for _, s := range []struct{ field, text string }{
		{"snippet.rule", e.Snippet.Rule},
		{"snippet.why", e.Snippet.Why},
	} {
		for _, name := range UsedPlaceholders(s.text) {
			if !slices.Contains(placeholderNames, name) {
				add(s.field, "unknown placeholder {%s}; known placeholders: {%s}. Wrap literal braces in backticks",
					name, strings.Join(placeholderNames, "}, {"))
			}
		}
	}

	if e.Enforce != nil && e.Enforce.Claude != nil {
		c := e.Enforce.Claude
		errs = append(errs, validateNotBlank(name, "enforce.claude.deny", c.Deny)...)
		errs = append(errs, validateNotBlank(name, "enforce.claude.ask", c.Ask)...)
		// Deny wins over ask in Claude Code, so a rule in both lists would
		// never ask: the entry must say which one it means.
		for i, rule := range c.Ask {
			if strings.TrimSpace(rule) != "" && slices.Contains(c.Deny, rule) {
				add(fmt.Sprintf("enforce.claude.ask[%d]", i), "%q is also listed in enforce.claude.deny, which wins; keep it in one list", rule)
			}
		}
		// Keys are sorted so errors come out in the same order every run.
		for _, key := range slices.Sorted(maps.Keys(c.With)) {
			field := "enforce.claude.with." + key
			switch {
			case !kebabCase.MatchString(key):
				add(field, "%q must be an entry id in kebab-case", key)
			case key == e.ID:
				add(field, "an entry cannot reference itself")
			}
			rules := c.With[key].Ask
			if len(rules) == 0 {
				add(field, "has no ask rules; add some or remove the key")
			}
			errs = append(errs, validateNotBlank(name, field+".ask", rules)...)
			for i, rule := range rules {
				if strings.TrimSpace(rule) != "" && slices.Contains(c.Deny, rule) {
					add(fmt.Sprintf("%s.ask[%d]", field, i), "%q is also listed in enforce.claude.deny, which wins; keep it in one list", rule)
				}
			}
		}
	}
	if e.Enforce != nil {
		errs = append(errs, validateGitignore(name, e.Enforce.Gitignore)...)
	}
	if e.Detect != nil {
		errs = append(errs, validateDetect(name, *e.Detect)...)
	}
	return errs
}

// validateDetect checks the detect field of an entry.
func validateDetect(name string, d Detect) []error {
	errs := validateNotBlank(name, "detect.files", d.Files)
	add := func(field, format string, args ...any) {
		errs = append(errs, &ValidationError{File: name, Field: field, Msg: fmt.Sprintf(format, args...)})
	}

	if deps := d.Dependencies; deps != nil {
		if len(deps.NPM)+len(deps.PyPI)+len(deps.Go) == 0 {
			add("detect.dependencies", "lists no dependencies; add some or remove it")
		}
		errs = append(errs, validatePackageNames(name, "detect.dependencies.npm", deps.NPM, nil)...)
		errs = append(errs, validatePackageNames(name, "detect.dependencies.pypi", deps.PyPI, func(n string) string {
			if want := NormalizePyPI(n); want != n {
				return fmt.Sprintf("%q must be written in its normalized form, %q", n, want)
			}
			return ""
		})...)
		errs = append(errs, validatePackageNames(name, "detect.dependencies.go", deps.Go, func(n string) string {
			if strings.HasSuffix(n, "/") || strings.HasPrefix(n, "/") {
				return fmt.Sprintf("%q must be a module path without leading or trailing slashes", n)
			}
			return ""
		})...)
	}

	if d.RunScript != "" {
		switch {
		case strings.TrimSpace(d.RunScript) == "" || strings.ContainsAny(d.RunScript, "\r\n"):
			add("detect.run_script", "must be one line of text")
		case strings.Count(d.RunScript, ScriptPlaceholder) != 1:
			add("detect.run_script", "%q must contain %s exactly once, where the script's name goes", d.RunScript, ScriptPlaceholder)
		}
	}
	return errs
}

// validatePackageNames checks one ecosystem's list of names: none blank,
// none with spaces, none listed twice. check, when not nil, returns a
// message for a name the ecosystem does not accept, or "".
func validatePackageNames(name, field string, names []string, check func(string) string) []error {
	errs := validateNotBlank(name, field, names)
	for i, n := range names {
		f := fmt.Sprintf("%s[%d]", field, i)
		add := func(msg string) { errs = append(errs, &ValidationError{File: name, Field: f, Msg: msg}) }
		switch {
		case strings.TrimSpace(n) == "":
			// Reported by validateNotBlank.
		case strings.IndexFunc(n, unicode.IsSpace) >= 0:
			add(fmt.Sprintf("%q contains spaces; a package name never does", n))
		case slices.Index(names, n) < i:
			add(fmt.Sprintf("%q is listed twice", n))
		case check != nil:
			if msg := check(n); msg != "" {
				add(msg)
			}
		}
	}
	return errs
}

// pypiSeparators matches the runs of characters PEP 503 treats as one.
var pypiSeparators = regexp.MustCompile(`[-_.]+`)

// NormalizePyPI returns the normalized form of a Python project name, as
// PEP 503 defines it: lowercase, with every run of "-", "_" and "."
// replaced by one hyphen. pip treats "Flask_SQLAlchemy" and
// "flask-sqlalchemy" as the same project, so detection compares names in
// this form.
func NormalizePyPI(name string) string {
	return pypiSeparators.ReplaceAllString(strings.ToLower(name), "-")
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

// validateGitignore checks the lines an entry adds to the .gitignore block.
// Each must be one pattern on one line: no leading or trailing spaces, which
// git reads as part of the pattern, and no leading #, which makes a comment
// and could imitate the block's markers.
func validateGitignore(name string, lines []string) []error {
	const field = "enforce.gitignore"
	errs := validateNotBlank(name, field, lines)
	for i, line := range lines {
		f := fmt.Sprintf("%s[%d]", field, i)
		add := func(msg string) { errs = append(errs, &ValidationError{File: name, Field: f, Msg: msg}) }
		switch {
		case strings.TrimSpace(line) == "":
			// Reported by validateNotBlank.
		case strings.ContainsAny(line, "\r\n"):
			add("must be a single line")
		case strings.TrimSpace(line) != line:
			add(fmt.Sprintf("%q has leading or trailing spaces, which git reads as part of the pattern", line))
		case strings.HasPrefix(line, "#"):
			add(fmt.Sprintf("%q starts with #, which makes it a comment", line))
		case slices.Index(lines, line) < i:
			add(fmt.Sprintf("%q is listed twice", line))
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

// validateCatalog checks rules that span entries: unique ids, one select value
// per category, references to existing entries (in requires, requires_one_of,
// conflicts and the keys of enforce.claude.with), with keys that can be
// selected together with their entry, and conflicts that cross categories
// and are symmetric.
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

	// The UI draws a category either as radio buttons or as checkboxes, never
	// a mix, so the first file of each category sets the value for the rest.
	firstOfCategory := make(map[string]file)
	for _, f := range files {
		e := f.entry
		if e.Category == "" || !slices.Contains(selects, e.Select) {
			continue // reported by validateEntry
		}
		first, ok := firstOfCategory[e.Category]
		if !ok {
			firstOfCategory[e.Category] = f
			continue
		}
		if e.Select != first.entry.Select {
			errs = append(errs, &ValidationError{File: f.name, Field: "select", Msg: fmt.Sprintf(
				"%q differs from %q used by %s in category %q; all entries in a category must share the same select",
				e.Select, first.entry.Select, first.name, e.Category)})
		}
	}

	for _, f := range files {
		e := f.entry
		for i, id := range e.Requires {
			if _, ok := byID[id]; !ok && isRef(e.Requires, i, e.ID) {
				errs = append(errs, &ValidationError{File: f.name, Field: fmt.Sprintf("requires[%d]", i), Msg: fmt.Sprintf("unknown entry %q", id)})
			}
		}
		for i, group := range e.RequiresOneOf {
			for j, id := range group {
				if _, ok := byID[id]; !ok && isRef(group, j, e.ID) {
					errs = append(errs, &ValidationError{File: f.name, Field: fmt.Sprintf("requires_one_of[%d][%d]", i, j), Msg: fmt.Sprintf("unknown entry %q", id)})
				}
			}
		}
		if e.Enforce != nil && e.Enforce.Claude != nil {
			for _, key := range slices.Sorted(maps.Keys(e.Enforce.Claude.With)) {
				if key == e.ID || !kebabCase.MatchString(key) {
					continue // reported by validateEntry
				}
				field := "enforce.claude.with." + key
				switch other, ok := byID[key]; {
				case !ok:
					errs = append(errs, &ValidationError{File: f.name, Field: field, Msg: fmt.Sprintf("unknown entry %q", key)})
				case sameRadioCategory(e, other.entry):
					errs = append(errs, &ValidationError{File: f.name, Field: field, Msg: fmt.Sprintf(
						"%q is in the same radio category %q, so the two are never selected together and these rules would never apply", key, e.Category)})
				}
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
