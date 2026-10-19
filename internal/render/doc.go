// Package render turns resolved selections into agent instruction files.
//
// There is one renderer per output format (CLAUDE.md, AGENTS.md, Cursor and
// Copilot rules). Renderers return files in memory and never write to disk;
// they read the existing project only through an fs.FS, to keep the
// protected sections people wrote by hand. Output is deterministic and
// tested against golden files in testdata/.
package render
