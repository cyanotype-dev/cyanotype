package catalog

import (
	"os"
	"path/filepath"
	"testing"
)

// TestRealCatalog loads the catalog shipped in the repository and fails on
// any invalid entry, so a broken entry never reaches main.
func TestRealCatalog(t *testing.T) {
	c, err := Load(os.DirFS(filepath.Join("..", "..", "catalog", "entries")))
	if err != nil {
		t.Fatalf("catalog/entries is invalid:\n%v", err)
	}
	if len(c.Entries()) == 0 {
		t.Fatal("catalog/entries holds no entries")
	}
}

// TestRealCatalogRoot loads the whole catalog folder, manifest included, so
// the bundled catalog always carries a valid version.
func TestRealCatalogRoot(t *testing.T) {
	c, err := LoadRoot(os.DirFS(filepath.Join("..", "..", "catalog")))
	if err != nil {
		t.Fatalf("catalog/ is invalid:\n%v", err)
	}
	if !ValidVersion(c.Version()) {
		t.Errorf("catalog/catalog.yaml version = %q, want a valid version", c.Version())
	}
}
