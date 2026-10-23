// Package schema embeds the JSON Schemas next to this file into every
// cyanotype binary that imports it.
//
// The MCP server needs them at run time: each tool declares the shape of
// its result with the definition in output.schema.json, the schema the
// tests check the contract types against, so what a client is told and
// what the server returns cannot drift apart. The package holds data only.
package schema

import _ "embed"

// Output is output.schema.json, the JSON output of the CLI's --json and of
// the MCP tools, one definition per command under $defs.
//
//go:embed output.schema.json
var Output []byte

// Spec is spec.schema.json, the schema of cyanotype.yaml, which the
// output of cyanotype spec refers to.
//
//go:embed spec.schema.json
var Spec []byte
