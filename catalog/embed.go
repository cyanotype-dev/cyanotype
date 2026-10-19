// Package catalog embeds the bundled catalog, the catalog.yaml manifest and
// the entries folder next to this file, into every cyanotype binary.
//
// The binary then needs no files next to it and works offline: this is the
// "bundled snapshot" of the Distribution section in docs/DESIGN.md. Load it
// with internal/catalog.LoadRoot(FS()). The package holds data only; loading
// and validation live in internal/catalog, which every interface shares.
package catalog

import (
	"embed"
	"io/fs"
)

// files holds the whole entries folder, not only its .yaml files, so a stray
// file is embedded too and LoadRoot rejects it exactly as it would on disk.
//
//go:embed catalog.yaml entries
var files embed.FS

// FS returns the bundled catalog, laid out like this folder: catalog.yaml at
// the root and one file per entry under entries/.
func FS() fs.FS {
	return files
}
