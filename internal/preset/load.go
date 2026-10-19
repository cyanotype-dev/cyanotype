package preset

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/cyanotype-dev/cyanotype/internal/catalog"
	"github.com/cyanotype-dev/cyanotype/internal/resolve"
	"github.com/cyanotype-dev/cyanotype/internal/spec"
)

// Dir is the folder of a catalog that holds the presets, next to the
// entries folder.
const Dir = "presets"

// ValidationError describes one problem in one preset file.
type ValidationError struct {
	File  string // file name inside the presets folder
	Field string // YAML path such as "selections[2]"; empty for file-level problems
	Msg   string
}

func (e *ValidationError) Error() string {
	if e.Field == "" {
		return e.File + ": " + e.Msg
	}
	return e.File + ": " + e.Field + ": " + e.Msg
}

// kebabCase matches preset ids, like catalog entry ids.
var kebabCase = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// LoadRoot loads the presets of a catalog folder laid out like catalog/ in
// the repository, and validates them against c, the catalog loaded from the
// same folder. A catalog without a presets folder has no presets.
//
// Presets come sorted by id. All problems are returned together, joined
// with errors.Join; each one is a *ValidationError unless reading the
// files failed.
func LoadRoot(catalogFS fs.FS, c *catalog.Catalog) ([]Preset, error) {
	dirEntries, err := fs.ReadDir(catalogFS, Dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s folder: %w", Dir, err)
	}

	var (
		presets []Preset
		errs    []error
	)
	for _, d := range dirEntries {
		name := d.Name()
		switch {
		case strings.HasPrefix(name, "."):
			continue
		case d.IsDir():
			errs = append(errs, &ValidationError{File: name, Msg: "subfolders are not allowed; presets live in one flat folder"})
			continue
		case path.Ext(name) != ".yaml":
			errs = append(errs, &ValidationError{File: name, Msg: "only .yaml files are allowed"})
			continue
		}

		data, err := fs.ReadFile(catalogFS, path.Join(Dir, name))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}
		p, decodeErrs := decode(name, data)
		if len(decodeErrs) > 0 {
			errs = append(errs, decodeErrs...)
			continue
		}
		if fileErrs := validate(name, p, c); len(fileErrs) > 0 {
			errs = append(errs, fileErrs...)
			continue
		}
		presets = append(presets, p)
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	slices.SortFunc(presets, func(a, b Preset) int { return strings.Compare(a.ID, b.ID) })
	return presets, nil
}

var unknownField = regexp.MustCompile(`^(line \d+): field (\S+) not found in type `)

// decode parses exactly one YAML document into a Preset, rejecting unknown
// fields. Keys that describe one project, such as project or i18n, are
// unknown fields too: a preset cannot know them.
func decode(name string, data []byte) (Preset, []error) {
	var p Preset
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)

	err := dec.Decode(&p)
	if errors.Is(err, io.EOF) {
		return Preset{}, []error{&ValidationError{File: name, Msg: "file is empty"}}
	}
	if err != nil {
		return Preset{}, decodeErrors(name, err)
	}

	var extra yaml.Node
	switch err := dec.Decode(&extra); {
	case errors.Is(err, io.EOF):
		return p, nil
	case err != nil:
		return Preset{}, decodeErrors(name, err)
	default:
		return Preset{}, []error{&ValidationError{File: name, Msg: "a file must hold exactly one YAML document"}}
	}
}

// decodeErrors turns a YAML error into one ValidationError per problem,
// rewording messages that mention Go types.
func decodeErrors(name string, err error) []error {
	var typeErr *yaml.TypeError
	if !errors.As(err, &typeErr) {
		return []error{&ValidationError{File: name, Msg: "invalid YAML: " + strings.TrimPrefix(err.Error(), "yaml: ")}}
	}
	errs := make([]error, 0, len(typeErr.Errors))
	for _, msg := range typeErr.Errors {
		if m := unknownField.FindStringSubmatch(msg); m != nil {
			msg = fmt.Sprintf("%s: unknown field %q; a preset holds only selections, i18n_selections, commands and output, "+
				"since the other spec fields describe one project", m[1], m[2])
		}
		errs = append(errs, &ValidationError{File: name, Msg: msg})
	}
	return errs
}

// validate checks the metadata of one preset, then resolves it against c
// as simple mode will use it: on its own, and with the entries a
// multilingual project adds. Resolving reuses every check of a spec, so a
// preset that loads always gives a spec that generates.
func validate(name string, p Preset, c *catalog.Catalog) []error {
	var errs []error
	add := func(field, format string, args ...any) {
		errs = append(errs, &ValidationError{File: name, Field: field, Msg: fmt.Sprintf(format, args...)})
	}

	for _, r := range []struct{ field, value string }{
		{"id", p.ID}, {"name", p.Name}, {"summary", p.Summary}, {"project_type", string(p.ProjectType)},
	} {
		switch {
		case strings.TrimSpace(r.value) == "":
			add(r.field, "is required")
		case strings.Contains(r.value, "\n"):
			add(r.field, "must be a single line")
		}
	}
	if p.ID != "" {
		switch {
		case !kebabCase.MatchString(p.ID):
			add("id", "%q must be kebab-case (lowercase letters, digits and single hyphens)", p.ID)
		case name != p.ID+".yaml":
			add("id", "%q does not match the file name; rename the file to %s.yaml", p.ID, p.ID)
		}
	}
	if p.ProjectType != "" && !slices.Contains(projectTypes, p.ProjectType) {
		add("project_type", "must be %s, got %q", quoteList(projectTypes), p.ProjectType)
	}
	if len(p.Selections) == 0 {
		add("selections", "is required; list at least one catalog entry")
	}
	if len(errs) > 0 {
		return errs
	}

	// Placeholders for the fields a preset cannot know; resolving needs them.
	project := spec.Project{Name: p.ID, Goal: p.Summary}
	targets := []spec.Target{spec.TargetAgentsMD}

	plain := p.Spec(project, targets)
	seen := map[string]bool{}
	for _, e := range resolveErrors(c, plain) {
		ve := &ValidationError{File: name, Field: e.Field, Msg: e.Msg}
		seen[ve.Error()] = true
		errs = append(errs, ve)
	}

	multilingual := p.Spec(project, targets)
	multilingual.Selections = append(multilingual.Selections, p.MultilingualSelections()...)
	multilingual.I18n = &spec.I18n{DefaultLocale: "en"}
	for _, e := range resolveErrors(c, multilingual) {
		field, msg := multilingualField(e.Field, len(p.Selections)), e.Msg
		if field == "" || !strings.HasPrefix(field, "i18n_selections") {
			msg = fmt.Sprintf("%s (when the app is multilingual and %q and i18n_selections are added)", msg, GenericI18n)
		}
		ve := &ValidationError{File: name, Field: field, Msg: msg}
		if !seen[(&ValidationError{File: name, Field: e.Field, Msg: e.Msg}).Error()] {
			errs = append(errs, ve)
		}
	}
	return errs
}

// resolveErrors resolves s against c and returns its problems.
func resolveErrors(c *catalog.Catalog, s spec.Spec) []*spec.ValidationError {
	_, _, err := resolve.Resolve(c, spec.New(s))
	if err == nil {
		return nil
	}
	errs := []error{err}
	var joined interface{ Unwrap() []error }
	if errors.As(err, &joined) {
		errs = joined.Unwrap()
	}
	out := make([]*spec.ValidationError, 0, len(errs))
	for _, e := range errs {
		var ve *spec.ValidationError
		if !errors.As(e, &ve) {
			ve = &spec.ValidationError{Msg: e.Error()}
		}
		out = append(out, ve)
	}
	return out
}

var selectionIndex = regexp.MustCompile(`^selections\[(\d+)\]$`)

// multilingualField maps a field of the multilingual spec, whose selections
// are the preset's n selections, then GenericI18n, then i18n_selections,
// back to the field of the preset file.
func multilingualField(field string, n int) string {
	m := selectionIndex.FindStringSubmatch(field)
	if m == nil {
		return field
	}
	i, _ := strconv.Atoi(m[1])
	switch {
	case i < n:
		return field
	case i == n:
		return "" // GenericI18n itself, which the preset does not list
	default:
		return fmt.Sprintf("i18n_selections[%d]", i-n-1)
	}
}

// quoteList formats values as `"a", "b" or "c"` for error messages.
func quoteList[T ~string](values []T) string {
	quoted := make([]string, len(values))
	for i, v := range values {
		quoted[i] = strconv.Quote(string(v))
	}
	if len(quoted) == 1 {
		return quoted[0]
	}
	return strings.Join(quoted[:len(quoted)-1], ", ") + " or " + quoted[len(quoted)-1]
}
