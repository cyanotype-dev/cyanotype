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
