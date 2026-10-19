// Package preset loads the presets that ship inside the catalog: ready
// combinations of entries, such as "Next.js + Supabase", that simple mode
// starts a project from.
//
// A preset is the stack part of a cyanotype.yaml, with the same keys and
// meaning (selections, commands, output), plus metadata of its own. It
// cannot be a whole spec, because the fields about one project, such as
// project.name and project.goal, are unknown until someone uses it. Presets
// live in the presets folder of the catalog, one file per preset, and are
// validated against the entries they ship with, so a preset that does not
// resolve never loads. See the Presets section of docs/DESIGN.md.
package preset

import (
	"slices"

	"github.com/cyanotype-dev/cyanotype/internal/catalog"
	"github.com/cyanotype-dev/cyanotype/internal/spec"
)

// ProjectType is what a preset builds. Simple mode asks for it first, and
// offers the presets of the chosen type.
type ProjectType string

// Allowed values for ProjectType. Desktop apps and games will join them once
// the catalog has stacks for them.
const (
	Website   ProjectType = "website"
	MobileApp ProjectType = "mobile-app"
	API       ProjectType = "api"
	Script    ProjectType = "script"
)

// ProjectTypes lists every allowed ProjectType, in the order simple mode
// shows them.
func ProjectTypes() []ProjectType {
	return slices.Clone(projectTypes)
}

var projectTypes = []ProjectType{Website, MobileApp, API, Script}

// GenericI18n is the catalog entry holding the internationalization rules
// that apply to any stack. A project that is multilingual, or will be, gets
// it together with the preset's I18nSelections.
const GenericI18n = "i18n-rules"

// languageCategory is the catalog category of programming languages.
const languageCategory = "language"

// Preset is one ready combination of catalog entries, decoded from a single
// YAML file. Fields whose yaml tag has no omitempty are required.
type Preset struct {
	// ID is the stable identifier, equal to the file name without .yaml.
	ID string `yaml:"id"`
	// Name is the display name, such as "Next.js + Supabase".
	Name string `yaml:"name"`
	// Summary says in one line what the preset is good for.
	Summary     string      `yaml:"summary"`
	ProjectType ProjectType `yaml:"project_type"`
	// Selections holds catalog entry ids in the order they go into the spec,
	// which is the order of their text in generated files.
	Selections []string `yaml:"selections"`
	// I18nSelections holds the entries added when the app is multilingual,
	// such as the i18n library that fits the stack. They live in the preset
	// because which library goes with which framework is stack knowledge,
	// which belongs in the catalog rather than in code.
	I18nSelections []string      `yaml:"i18n_selections,omitempty"`
	Commands       spec.Commands `yaml:"commands,omitempty"`
	Output         spec.Output   `yaml:"output,omitempty"`
}

// Spec returns a spec built from the preset for the given project and
// targets. The result does not share slices with p, so callers may append
// to its selections.
func (p Preset) Spec(project spec.Project, targets []spec.Target) spec.Spec {
	return spec.Spec{
		Version:    spec.CurrentVersion,
		Targets:    slices.Clone(targets),
		Project:    project,
		Commands:   p.Commands,
		Selections: slices.Clone(p.Selections),
		Output:     p.Output,
	}
}

// MultilingualSelections returns the entries a multilingual project adds to
// the preset's selections: the generic i18n rules, then the preset's own
// i18n entries.
func (p Preset) MultilingualSelections() []string {
	return append([]string{GenericI18n}, p.I18nSelections...)
}

// Languages returns the ids of the selected entries of the "language"
// category, in selection order, such as ["typescript"]. Simple mode uses
// them to tell whether the user already knows the preset's languages.
func (p Preset) Languages(c *catalog.Catalog) []string {
	var ids []string
	for _, id := range p.Selections {
		if e, ok := c.Get(id); ok && e.Category == languageCategory {
			ids = append(ids, id)
		}
	}
	return ids
}
