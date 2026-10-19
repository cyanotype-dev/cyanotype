package resolve

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/cyanotype-dev/cyanotype/internal/designdoctest"
	"github.com/cyanotype-dev/cyanotype/internal/spec"
)

// TestDesignExample resolves the spec example from docs/DESIGN.md against a
// test catalog holding the entries it selects, so the documented example
// stays consistent with the resolver's rules.
func TestDesignExample(t *testing.T) {
	c := loadCatalog(t, "design")
	example := designdoctest.SpecExample(t, filepath.Join("..", ".."))
	f, err := spec.Load(strings.NewReader(example))
	if err != nil {
		t.Fatalf("the DESIGN.md example does not load:\n%v", err)
	}

	res, warnings, err := Resolve(c, f)
	if err != nil || len(warnings) > 0 {
		t.Fatalf("Resolve() warnings %q, error:\n%v", warningStrings(warnings), err)
	}
	var ids, locked []string
	for _, e := range res.Entries {
		ids = append(ids, e.ID)
		if e.Locked {
			locked = append(locked, e.ID)
		}
	}
	if !slices.Equal(ids, f.Spec.Selections) {
		t.Errorf("entries = %q, want the selections %q", ids, f.Spec.Selections)
	}
	if !slices.Equal(locked, f.Spec.Locked) {
		t.Errorf("locked entries = %q, want %q", locked, f.Spec.Locked)
	}
	if res.CatalogVersion != f.Spec.CatalogVersion {
		t.Errorf("CatalogVersion = %q, want the pinned %q", res.CatalogVersion, f.Spec.CatalogVersion)
	}
}

// TestDesignExampleErrorLine checks that errors in the example point to
// the lines of the selections involved.
func TestDesignExampleErrorLine(t *testing.T) {
	c := loadCatalog(t, "design")
	example := designdoctest.SpecExample(t, filepath.Join("..", ".."))
	const item = "  - nextjs\n"
	i := strings.Index(example, item)
	if i < 0 {
		t.Fatalf("the DESIGN.md example has no %q line", strings.TrimSpace(item))
	}
	line := strings.Count(example[:i], "\n") + 1
	example = strings.Replace(example, item, "  - nextjss\n", 1)
	intl := line + strings.Count(example[i:strings.Index(example, "  - i18n-next-intl\n")], "\n")

	f, err := spec.Load(strings.NewReader(example))
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = Resolve(c, f)
	want := []string{
		fmt.Sprintf(`line %d: selections[1]: unknown entry "nextjss"; did you mean "nextjs"?`, line),
		// The typo also leaves the entry that requires nextjs without it.
		fmt.Sprintf(`line %d: selections[6]: "i18n-next-intl" requires "nextjs", which is not selected`, intl),
	}
	if got := messages(err); !slices.Equal(got, want) {
		t.Errorf("errors = %q, want %q", got, want)
	}
}
