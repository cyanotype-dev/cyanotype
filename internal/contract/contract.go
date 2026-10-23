// Package contract is the JSON output of cyanotype: what the CLI prints
// with --json and what the MCP tools return. It is one public contract, so
// both interfaces build the same Go types from what the core returns and
// encode them with Marshal, and a script that reads the CLI's output reads
// an MCP tool's result the same way. See docs/DESIGN.md, JSON output.
//
// Every output is an object that starts with the contract version and ends
// with its warnings and errors, always present as arrays. Field names are
// lowercase snake_case; the codes inside diagnostics are kebab-case, like
// the kinds of cyanotype check. Paths are relative to the project root,
// with forward slashes.
//
// Like every internal package it never prints: Marshal returns bytes, and
// each interface decides where they go.
package contract

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Version is the version of the contract. Adding fields, codes or commands
// keeps it; renaming or removing a field, or changing what one means,
// increments it. Readers ignore fields they do not know.
const Version = 1

// Head opens every output with the contract version. Output types embed
// it first, so the version is the first field of the JSON object.
type Head struct {
	Version int `json:"version"`
}

func (h *Head) head() *Head { return h }

// Diagnostics closes every output with what went wrong. Output types embed
// it last. Errors is not empty when the command could not do its work; the
// output then holds only the version, the warnings and the errors.
type Diagnostics struct {
	Warnings []Diagnostic `json:"warnings"`
	Errors   []Diagnostic `json:"errors"`
}

func (d *Diagnostics) diagnostics() *Diagnostics { return d }

// Output is any of the output types of this package: each embeds Head and
// Diagnostics.
type Output interface {
	head() *Head
	diagnostics() *Diagnostics
}

// Marshal encodes o as the JSON every interface returns: indented by two
// spaces, with a final newline, and without HTML escaping, since diffs hold
// "<!--" and "&". It sets the contract version, and turns nil warnings and
// errors into empty arrays, so readers never see null there.
func Marshal(o Output) ([]byte, error) {
	o.head().Version = Version
	d := o.diagnostics()
	if d.Warnings == nil {
		d.Warnings = []Diagnostic{}
	}
	if d.Errors == nil {
		d.Errors = []Diagnostic{}
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(o); err != nil {
		return nil, fmt.Errorf("encode JSON output: %w", err)
	}
	return buf.Bytes(), nil
}
