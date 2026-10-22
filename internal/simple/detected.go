package simple

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/cyanotype-dev/cyanotype/internal/catalog"
	"github.com/cyanotype-dev/cyanotype/internal/detect"
	"github.com/cyanotype-dev/cyanotype/internal/preset"
	"github.com/cyanotype-dev/cyanotype/internal/resolve"
	"github.com/cyanotype-dev/cyanotype/internal/spec"
)

// On an existing project the stack comes from detection, not from a
// preset: detection says what the project uses, a preset what a new
// project would use, so simple mode never adds a technology detection did
// not find, such as Supabase to a plain Next.js app. Presets still help
// where the project cannot answer by itself: the project type, the order of
// the entries, the default of an open choice and the i18n library.

// Requirement is an entry added to a detected stack because a selected
// entry requires it.
type Requirement struct {
	// ID is the added entry.
	ID string
	// By is the selected entry that requires it.
	By string
}

// ChoiceKind says why a detected stack needs the user to choose.
type ChoiceKind int

// The kinds of Choice.
const (
	// ChoiceClash means several entries were detected where only one can
	// be selected, such as npm and pnpm from two lockfiles.
	ChoiceClash ChoiceKind = iota
	// ChoiceGroup means a selected entry needs one of several entries and
	// none was detected, such as Node.js with neither npm nor pnpm.
	ChoiceGroup
)

// Choice is a question only the user can answer before a detected stack
// can become a spec. Interfaces ask it, or take the answer from a flag,
// then call DetectStack again with the chosen id in with.
type Choice struct {
	Kind ChoiceKind
	// IDs are the entries to choose among, in catalog order.
	IDs []string
	// Category is the radio category of a clash, or "" for a clash through
	// conflicts and for a group.
	Category string
	// By is the entry that needs one of IDs, for a group.
	By string
	// Default is the suggested answer, or "" when there is none: for a
	// group, the alternative the closest preset uses.
	Default string
}

// Detected is a stack found by detection, adjusted by the user's
// additions and removals.
type Detected struct {
	// Selections are the entries of the stack, in spec order: the order of
	// the closest preset, then the catalog's.
	Selections []string
	// Added are entries added because a selected entry requires them.
	Added []Requirement
	// Choices are questions to answer before the stack is complete. Build
	// refuses a Detected with choices left.
	Choices []Choice
	// Commands are the spec commands found in package.json scripts.
	Commands spec.Commands
	// CommandScripts names the scripts the commands come from, in the
	// order dev, build, test, lint.
	CommandScripts []string
	// ProjectType is the type of the presets closest to the stack, or ""
	// when they disagree, so the user must say.
	ProjectType preset.ProjectType
	// Multilingual reports that the stack already has an entry some preset
	// adds for translations, such as next-intl: the app is translated, so
	// only its locales are left to ask.
	Multilingual bool
}

// DetectStack turns what detection found into a stack. with adds entries,
// and answers choices: an entry in with replaces the other entries of its
// radio category and the entries it conflicts with. without removes
// detected entries, for a file that revealed something the project does
// not use. Unknown ids in with or without, an id in both, and two ids of
// with that exclude each other are errors.
func DetectStack(c *catalog.Catalog, presets []preset.Preset, r detect.Result, with, without []string) (Detected, error) {
	if err := checkAdjustments(c, r, with, without); err != nil {
		return Detected{}, err
	}

	var sel []string
	for _, id := range r.IDs() {
		if !slices.Contains(without, id) {
			sel = append(sel, id)
		}
	}
	for _, id := range with {
		e, _ := c.Get(id)
		sel = slices.DeleteFunc(sel, func(other string) bool {
			o, _ := c.Get(other)
			return other != id && (o.Category == e.Category && e.Select == catalog.SelectRadio ||
				slices.Contains(e.Conflicts, other))
		})
		if !slices.Contains(sel, id) {
			sel = append(sel, id)
		}
	}

	d := Detected{}
	// Add what the selection requires, until nothing is missing: an added
	// entry may require more.
	for i := 0; i < len(sel); i++ {
		e, _ := c.Get(sel[i])
		for _, req := range e.Requires {
			if !slices.Contains(sel, req) {
				sel = append(sel, req)
				d.Added = append(d.Added, Requirement{ID: req, By: e.ID})
			}
		}
	}

	closest := closestPresets(presets, sel)
	for _, clash := range detect.Clashes(c, sel) {
		d.Choices = append(d.Choices, Choice{Kind: ChoiceClash, IDs: clash.IDs, Category: clash.Category})
	}
	for _, id := range sel {
		e, _ := c.Get(id)
		for _, group := range e.RequiresOneOf {
			if slices.ContainsFunc(group, func(alt string) bool { return slices.Contains(sel, alt) }) ||
				slices.ContainsFunc(d.Choices, func(ch Choice) bool { return slices.Equal(ch.IDs, group) }) {
				continue
			}
			ch := Choice{Kind: ChoiceGroup, IDs: slices.Clone(group), By: id}
			if len(closest) > 0 {
				if i := slices.IndexFunc(group, func(alt string) bool { return slices.Contains(closest[0].Selections, alt) }); i >= 0 {
					ch.Default = group[i]
				}
			}
			d.Choices = append(d.Choices, ch)
		}
	}

	d.Selections = order(c, sel, closest)
	d.ProjectType = projectType(closest)
	d.Commands, d.CommandScripts = commands(c, sel, r.Scripts)
	for _, p := range presets {
		if slices.ContainsFunc(p.I18nSelections, func(id string) bool { return slices.Contains(sel, id) }) {
			d.Multilingual = true
		}
	}
	return d, nil
}

// checkAdjustments rejects with and without lists that name unknown
// entries, name an entry twice over, exclude each other, or remove an entry
// detection did not find.
func checkAdjustments(c *catalog.Catalog, r detect.Result, with, without []string) error {
	all := c.Entries() // sorted by id, as resolve.Suggest expects
	ids := make([]string, len(all))
	for i, e := range all {
		ids[i] = e.ID
	}
	var errs []error
	known := func(flag, id string) bool {
		if _, ok := c.Get(id); ok {
			return true
		}
		msg := fmt.Sprintf("%s: unknown entry %q", flag, id)
		if s, ok := resolve.Suggest(id, ids); ok {
			msg += fmt.Sprintf("; did you mean %q?", s)
		}
		errs = append(errs, errors.New(msg))
		return false
	}
	for i, id := range with {
		if !known("with", id) {
			continue
		}
		e, _ := c.Get(id)
		if slices.Contains(without, id) {
			errs = append(errs, fmt.Errorf("%q is both added and removed; keep it in one list", id))
		}
		for _, other := range with[:i] {
			o, ok := c.Get(other)
			switch {
			case !ok:
			case o.Category == e.Category && e.Select == catalog.SelectRadio:
				errs = append(errs, fmt.Errorf("%q and %q are both in radio category %q; add only one", other, id, e.Category))
			case slices.Contains(e.Conflicts, other):
				errs = append(errs, fmt.Errorf("%q conflicts with %q; add only one", id, other))
			}
		}
	}
	for _, id := range without {
		if known("without", id) {
			if _, found := r.Find(id); !found {
				errs = append(errs, fmt.Errorf("without: %q was not detected, so there is nothing to remove", id))
			}
		}
	}
	return errors.Join(errs...)
}

// closestPresets returns the presets that share the most entries with sel,
// in preset order, or none when no preset shares any.
func closestPresets(presets []preset.Preset, sel []string) []preset.Preset {
	best, bestScore := []preset.Preset(nil), 0
	for _, p := range presets {
		score := 0
		for _, id := range p.Selections {
			if slices.Contains(sel, id) {
				score++
			}
		}
		switch {
		case score == 0 || score < bestScore:
		case score > bestScore:
			best, bestScore = []preset.Preset{p}, score
		default:
			best = append(best, p)
		}
	}
	return best
}

// projectType returns the project type all the presets share, or "" when
// they disagree or there are none.
func projectType(presets []preset.Preset) preset.ProjectType {
	if len(presets) == 0 {
		return ""
	}
	t := presets[0].ProjectType
	for _, p := range presets[1:] {
		if p.ProjectType != t {
			return ""
		}
	}
	return t
}

// order sorts sel the way the closest preset does, so a detected stack
// reads like a starter one; entries the preset does not have follow, in
// catalog order.
func order(c *catalog.Catalog, sel []string, closest []preset.Preset) []string {
	rank := map[string]int{}
	for i, e := range c.Entries() {
		rank[e.ID] = len(c.Entries()) + i
	}
	if len(closest) > 0 {
		for i, id := range closest[0].Selections {
			rank[id] = i
		}
	}
	out := slices.Clone(sel)
	slices.SortStableFunc(out, func(a, b string) int { return rank[a] - rank[b] })
	return out
}

// commandScripts lists the spec commands, which package.json scripts of
// the same name fill.
var commandScripts = []string{"dev", "build", "test", "lint"}

// commands turns the package.json scripts named like spec commands into
// commands, run with the selected package manager's detect.run_script.
// Without exactly one selected entry that has one, there are no commands:
// guessing the tool, such as npm while the choice between npm and pnpm is
// still open, would give commands that do not work.
func commands(c *catalog.Catalog, sel, scripts []string) (spec.Commands, []string) {
	run := ""
	for _, id := range sel {
		if e, _ := c.Get(id); e.Detect != nil && e.Detect.RunScript != "" {
			if run != "" {
				return spec.Commands{}, nil
			}
			run = e.Detect.RunScript
		}
	}
	var cmds spec.Commands
	var used []string
	if run == "" {
		return cmds, nil
	}
	fields := map[string]*string{"dev": &cmds.Dev, "build": &cmds.Build, "test": &cmds.Test, "lint": &cmds.Lint}
	for _, name := range commandScripts {
		if slices.Contains(scripts, name) {
			*fields[name] = strings.Replace(run, catalog.ScriptPlaceholder, name, 1)
			used = append(used, name)
		}
	}
	return cmds, used
}

// Stack returns the stack d gives a project of type t: its selections and
// commands, and the i18n entries of the closest preset of that type that
// fit the stack. It is an error to call it while choices are left.
func (d Detected) Stack(c *catalog.Catalog, presets []preset.Preset, t preset.ProjectType) (Stack, error) {
	if len(d.Choices) > 0 {
		return Stack{}, fmt.Errorf("the detected stack has %d choices left to make", len(d.Choices))
	}
	st := Stack{Selections: slices.Clone(d.Selections), Commands: d.Commands}

	var ofType []preset.Preset
	for _, p := range presets {
		if p.ProjectType == t {
			ofType = append(ofType, p)
		}
	}
	if closest := closestPresets(ofType, d.Selections); len(closest) > 0 {
		for _, id := range closest[0].I18nSelections {
			if fits(c, d.Selections, id) {
				st.I18nSelections = append(st.I18nSelections, id)
			}
		}
	}
	return st, nil
}

// fits reports whether entry id can join sel: it is not selected yet, no
// selected entry shares its radio category or conflicts with it, and sel
// meets its requirements. A preset's i18n library that needs another
// framework is left out, and the generic i18n rules then ask the agent to
// propose one.
func fits(c *catalog.Catalog, sel []string, id string) bool {
	e, ok := c.Get(id)
	if !ok || slices.Contains(sel, id) {
		return false
	}
	for _, other := range sel {
		o, _ := c.Get(other)
		if o.Category == e.Category && e.Select == catalog.SelectRadio || slices.Contains(e.Conflicts, other) {
			return false
		}
	}
	for _, req := range e.Requires {
		if !slices.Contains(sel, req) {
			return false
		}
	}
	for _, group := range e.RequiresOneOf {
		if !slices.ContainsFunc(group, func(alt string) bool { return slices.Contains(sel, alt) }) {
			return false
		}
	}
	return true
}

// Languages returns the ids of the entries of category "language" among
// ids, in order. Simple mode uses them to offer learning mode, and fills
// {languages} the same way.
func Languages(c *catalog.Catalog, ids []string) []string {
	var out []string
	for _, id := range ids {
		if e, ok := c.Get(id); ok && e.Category == languageCategory {
			out = append(out, id)
		}
	}
	return out
}

// OffersLearningFor is OffersLearning for any list of entries, such as a
// detected stack.
func OffersLearningFor(c *catalog.Catalog, ids []string, language string) bool {
	return !slices.Contains(Languages(c, ids), language)
}
