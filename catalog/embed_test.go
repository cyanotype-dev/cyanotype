package catalog

import (
	"io/fs"
	"os"
	"slices"
	"testing"

	loader "github.com/cyanotype-dev/cyanotype/internal/catalog"
	"github.com/cyanotype-dev/cyanotype/internal/preset"
)

// TestEmbedded loads the embedded catalog and compares it with this folder
// on disk, so the binary always ships the catalog the repository holds.
func TestEmbedded(t *testing.T) {
	embedded, err := loader.LoadRoot(FS())
	if err != nil {
		t.Fatalf("the embedded catalog is invalid:\n%v", err)
	}
	onDisk, err := loader.LoadRoot(os.DirFS("."))
	if err != nil {
		t.Fatalf("catalog/ is invalid:\n%v", err)
	}

	if embedded.Version() != onDisk.Version() {
		t.Errorf("embedded version = %q, want %q from catalog.yaml", embedded.Version(), onDisk.Version())
	}
	if got, want := ids(embedded), ids(onDisk); !slices.Equal(got, want) {
		t.Errorf("embedded entries = %q, want %q", got, want)
	}

	embeddedPresets, err := preset.LoadRoot(FS(), embedded)
	if err != nil {
		t.Fatalf("the embedded presets are invalid:\n%v", err)
	}
	presetsOnDisk, err := preset.LoadRoot(os.DirFS("."), onDisk)
	if err != nil {
		t.Fatalf("catalog/presets is invalid:\n%v", err)
	}
	if got, want := presetIDs(embeddedPresets), presetIDs(presetsOnDisk); !slices.Equal(got, want) {
		t.Errorf("embedded presets = %q, want %q", got, want)
	}
}

// TestEmbeddedLayout checks that only the manifest, the entries folder and
// the presets folder are embedded: Go files, the README and the license stay
// out of the binary.
func TestEmbeddedLayout(t *testing.T) {
	top, err := fs.ReadDir(FS(), ".")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, d := range top {
		names = append(names, d.Name())
	}
	if want := []string{"catalog.yaml", "entries", "presets"}; !slices.Equal(names, want) {
		t.Errorf("embedded top level = %q, want %q", names, want)
	}
}

func ids(c *loader.Catalog) []string {
	var out []string
	for _, e := range c.Entries() {
		out = append(out, e.ID)
	}
	return out
}

func presetIDs(presets []preset.Preset) []string {
	var out []string
	for _, p := range presets {
		out = append(out, p.ID)
	}
	return out
}
