package contract

import (
	"github.com/cyanotype-dev/cyanotype/internal/generate"
	"github.com/cyanotype-dev/cyanotype/internal/spec"
)

// ScopeTeam is the scope of the spec cyanotype reads today: the committed
// cyanotype.yaml, without personal overrides.
const ScopeTeam = "team"

// TeamOnlyNote says what Spec leaves out until cyanotype.local.yaml exists.
const TeamOnlyNote = "cyanotype.local.yaml is not supported yet; this is the team spec only"

// Spec is the output of cyanotype spec --json and of the MCP get_spec tool:
// the project's spec, as written and with its defaults applied.
type Spec struct {
	Head
	// File is the spec's path, relative to the project root.
	File string `json:"file,omitzero"`
	// Scope is ScopeTeam: personal overrides are not merged in yet.
	Scope string `json:"scope,omitzero"`
	// Note says so in words, for readers who do not know the scopes.
	Note string `json:"note,omitzero"`
	// Spec is cyanotype.yaml as written, with the field names of the file.
	Spec *spec.Spec `json:"spec,omitzero"`
	// Effective is the same spec with every default applied, as generate
	// reads it.
	Effective *spec.Spec `json:"effective,omitzero"`
	Diagnostics
}

// NewSpec returns the output for the spec s.
func NewSpec(s spec.Spec) *Spec {
	effective := s.WithDefaults()
	return &Spec{File: generate.SpecFile, Scope: ScopeTeam, Note: TeamOnlyNote, Spec: &s, Effective: &effective}
}
