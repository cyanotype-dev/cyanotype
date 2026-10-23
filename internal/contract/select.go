package contract

// CodeLocked is the code of the error about removing a selection the spec
// lists in locked: locked rules are never removed by a change of choices.
const CodeLocked = "locked"

// Select is the output of cyanotype select --json and of the MCP
// set_selections tool: the change of selections, as a diff of
// cyanotype.yaml, and whether it was written.
type Select struct {
	Head
	// Added are the entries the change adds to selections, in order;
	// entries already selected are not listed.
	Added []string `json:"added,omitzero"`
	// Removed are the entries the change removes from selections.
	Removed []string `json:"removed,omitzero"`
	// SpecDiff is the change as a unified diff of cyanotype.yaml; omitted
	// when the spec already has those selections.
	SpecDiff string `json:"spec_diff,omitzero"`
	// Written reports that cyanotype.yaml was written.
	Written *bool `json:"written,omitzero"`
	Diagnostics
}
