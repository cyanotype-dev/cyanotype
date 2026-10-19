package spec

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"regexp"
	"slices"
	"strings"
)

var (
	// kebabCase matches catalog entry ids: lowercase letters and digits in
	// groups joined by single hyphens. Same pattern as internal/catalog.
	kebabCase = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	// snakeCase matches answer keys such as domain_notes.
	snakeCase = regexp.MustCompile(`^[a-z][a-z0-9]*(_[a-z0-9]+)*$`)
	// languageName matches human language names such as english or
	// brazilian-portuguese.
	languageName = regexp.MustCompile(`^[a-z]+(-[a-z]+)*$`)
	// catalogVersion matches date-based catalog versions such as 2026.10
	// or 2026.10.1.
	catalogVersion = regexp.MustCompile(`^[0-9]{4}\.(0[1-9]|1[0-2])(\.[1-9][0-9]*)?$`)
	// localeTag matches BCP 47 tags in canonical case with a language and
	// optional script and region: en, pt-BR, zh-Hant-TW, es-419.
	localeTag = regexp.MustCompile(`^[a-z]{2,3}(-[A-Z][a-z]{3})?(-([A-Z]{2}|[0-9]{3}))?$`)
)

// Allowed values of each enum, in the order error messages list them.
var (
	targets         = []Target{TargetClaude, TargetAgentsMD, TargetGemini, TargetCursor, TargetCopilot}
	modes           = []Mode{ModeSimple, ModeAdvanced}
	translationsBys = []TranslationsBy{TranslationsAgentDraft, TranslationsAgentDefaultOnly, TranslationsHuman}
	splits          = []Split{SplitAuto, SplitAlways, SplitNever}
)

// Validate checks the structure of s: required fields, enums, formats and
// consistency inside the spec. It does not need the catalog, so it cannot
// tell whether selected entries exist; internal/resolve checks that.
// All problems are returned together, joined with errors.Join; each one is
// a *ValidationError without a line, since s has no file.
func Validate(s Spec) error {
	return errors.Join(validate(s)...)
}

// validator collects problems for one spec.
type validator struct {
	errs []error
}

func (v *validator) add(field, format string, args ...any) {
	v.errs = append(v.errs, &ValidationError{Field: field, Msg: fmt.Sprintf(format, args...)})
}

// required reports a blank value.
func (v *validator) required(field, value string) bool {
	if strings.TrimSpace(value) == "" {
		v.add(field, "is required")
		return false
	}
	return true
}

// enum reports a set value outside allowed.
func enum[T ~string](v *validator, field string, value T, allowed []T) {
	if value != "" && !slices.Contains(allowed, value) {
		v.add(field, "must be %s, got %q", quoteList(allowed), value)
	}
}

// list reports blank and duplicate items, then runs check on the others.
func (v *validator) list(field string, items []string, check func(field, item string)) {
	for i, item := range items {
		f := fmt.Sprintf("%s[%d]", field, i)
		switch {
		case strings.TrimSpace(item) == "":
			v.add(f, "is empty")
		case slices.Index(items, item) < i:
			v.add(f, "%q is listed twice", item)
		case check != nil:
			check(f, item)
		}
	}
}

// path reports a value that is not a relative path inside the project.
func (v *validator) path(field, value string) {
	// fs.ValidPath rejects absolute paths, "." and ".." elements and empty
	// elements; backslashes and drive letters are rejected so the spec
	// means the same on every OS.
	if value != "" && (!fs.ValidPath(value) || value == "." || strings.ContainsAny(value, `\:`)) {
		v.add(field, "%q must be a relative path inside the project, with forward slashes", value)
	}
}

func validate(s Spec) []error {
	v := &validator{}

	switch s.Version {
	case 0:
		v.add("version", "is required")
	case CurrentVersion:
	default:
		v.add("version", "must be %d, got %d", CurrentVersion, s.Version)
	}

	if s.CatalogVersion != "" && !catalogVersion.MatchString(s.CatalogVersion) {
		v.add("catalog_version", "%q must be a catalog version such as \"2026.10\"", s.CatalogVersion)
	}

	if len(s.Targets) == 0 {
		v.add("targets", "is required; list at least one agent")
	}
	for i, t := range s.Targets {
		f := fmt.Sprintf("targets[%d]", i)
		if slices.Index(s.Targets, t) < i {
			v.add(f, "%q is listed twice", t)
			continue
		}
		enum(v, f, t, targets)
	}

	enum(v, "mode", s.Mode, modes)
	validateProject(v, s.Project)
	validateLanguages(v, s.Languages)
	validateCommands(v, s.Commands)

	v.list("selections", s.Selections, func(f, id string) {
		if !kebabCase.MatchString(id) {
			v.add(f, "%q must be a catalog entry id: kebab-case (lowercase letters, digits and single hyphens)", id)
		}
	})
	v.list("locked", s.Locked, func(f, id string) {
		if !slices.Contains(s.Selections, id) {
			v.add(f, "%q is not in selections; only selected entries can be locked", id)
		}
	})

	if s.I18n != nil {
		validateI18n(v, *s.I18n)
	}

	for _, key := range slices.Sorted(maps.Keys(s.Answers)) {
		f := "answers." + key
		if !snakeCase.MatchString(key) {
			v.add(f, "key must be snake_case (lowercase letters, digits and single underscores)")
		}
		if strings.TrimSpace(s.Answers[key]) == "" {
			v.add(f, "is empty")
		}
	}

	for i, r := range s.CustomRules {
		f := fmt.Sprintf("custom_rules[%d]", i)
		v.required(f+".rule", r.Rule)
		v.required(f+".why", r.Why)
	}

	enum(v, "output.split", s.Output.Split, splits)
	if s.Output.MaxLines < 0 {
		v.add("output.max_lines", "must be positive, got %d", s.Output.MaxLines)
	}
	v.path("output.topic_dir", s.Output.TopicDir)

	return v.errs
}

func validateProject(v *validator, p Project) {
	if v.required("project.name", p.Name) && strings.Contains(p.Name, "\n") {
		v.add("project.name", "must be a single line")
	}
	v.required("project.goal", p.Goal)
	v.list("project.core_features", p.CoreFeatures, nil)
	v.list("project.out_of_scope", p.OutOfScope, nil)
	v.path("project.brief", p.Brief)
}

func validateLanguages(v *validator, l Languages) {
	fields := []struct{ field, value string }{
		{"languages.code", l.Code},
		{"languages.comments", l.Comments},
		{"languages.docs", l.Docs},
		{"languages.commits", l.Commits},
		{"languages.agent_replies", l.AgentReplies},
	}
	for _, f := range fields {
		if f.value != "" && !languageName.MatchString(f.value) {
			v.add(f.field, "%q must be a lowercase language name such as \"english\"", f.value)
		}
	}
}

// validateCommands reports commands that are set but blank, or span several
// lines: the main file lists each command as inline code on one line.
func validateCommands(v *validator, c Commands) {
	fields := []struct{ field, value string }{
		{"commands.dev", c.Dev},
		{"commands.build", c.Build},
		{"commands.test", c.Test},
		{"commands.lint", c.Lint},
	}
	for _, f := range fields {
		switch {
		case f.value == "":
		case strings.TrimSpace(f.value) == "":
			v.add(f.field, "is empty")
		case strings.Contains(f.value, "\n"):
			v.add(f.field, "must be a single line")
		}
	}
}

func validateI18n(v *validator, i I18n) {
	checkLocale := func(field, tag string) {
		if localeTag.MatchString(tag) {
			return
		}
		hint := ""
		if strings.Contains(tag, "_") {
			hint = `; use "-" instead of "_"`
		}
		v.add(field, "%q must be a BCP 47 tag such as \"en\" or \"pt-BR\"%s", tag, hint)
	}

	if v.required("i18n.default_locale", i.DefaultLocale) {
		checkLocale("i18n.default_locale", i.DefaultLocale)
	}
	v.list("i18n.locales", i.Locales, checkLocale)
	if len(i.Locales) > 0 && i.DefaultLocale != "" && !slices.Contains(i.Locales, i.DefaultLocale) {
		v.add("i18n.locales", "must include the default locale %q", i.DefaultLocale)
	}
	enum(v, "i18n.translations_by", i.TranslationsBy, translationsBys)
}

// quoteList formats values as `"a", "b" or "c"` for error messages.
func quoteList[T ~string](values []T) string {
	quoted := make([]string, len(values))
	for i, val := range values {
		quoted[i] = fmt.Sprintf("%q", val)
	}
	if len(quoted) == 1 {
		return quoted[0]
	}
	return strings.Join(quoted[:len(quoted)-1], ", ") + " or " + quoted[len(quoted)-1]
}
