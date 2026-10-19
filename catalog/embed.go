// Package catalog embeds the bundled catalog, the catalog.yaml manifest, the
// entries folder and the presets folder next to this file, into every
// cyanotype binary.
//
// The binary then needs no files next to it and works offline: this is the
// "bundled snapshot" of the Distribution section in docs/DESIGN.md. Presets
// ship in the same snapshot, so they are always versioned with the entries
// they select. Load it with internal/catalog.LoadRoot(FS()), then
// internal/preset.LoadRoot(FS(), c). The package holds data only; loading
// and validation live in internal/catalog and internal/preset, which every
// interface shares.
package catalog

import (
	"embed"
	"io/fs"
)

// files holds the whole entries and presets folders, not only their .yaml
// files, so a stray file is embedded too and the loaders reject it exactly
// as they would on disk.
//
//go:embed catalog.yaml entries presets
var files embed.FS

// FS returns the bundled catalog, laid out like this folder: catalog.yaml at
// the root, one file per entry under entries/ and one per preset under
// presets/.
func FS() fs.FS {
	return files
}
