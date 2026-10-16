// Package render turns resolved selections into agent instruction files.
//
// There is one renderer per output format (CLAUDE.md, AGENTS.md, Cursor and
// Copilot rules). Renderers produce exact bytes and are tested against golden
// files in testdata/.
package render
