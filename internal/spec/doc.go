// Package spec reads and writes cyanotype.yaml, the project spec file.
//
// The spec holds the user's choices, never the generated text, so every
// interface (CLI, TUI, desktop app, MCP server) can read and update the same
// file and regenerate the instruction files from it.
package spec
