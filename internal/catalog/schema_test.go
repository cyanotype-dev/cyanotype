package catalog

import (
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/cyanotype-dev/cyanotype/internal/schematest"
)

// TestSchemaMatchesEntry keeps the editor schema in sync with the Go types:
// same fields, same required fields, same enums and the same id pattern.
func TestSchemaMatchesEntry(t *testing.T) {
	root := schematest.Load(t, filepath.Join("..", "..", "schema", "catalog-entry.schema.json"))

	schematest.CheckStruct(t, "entry", reflect.TypeFor[Entry](), root)

	props := root.Properties
	if got, want := props["select"].Enum, schematest.Strings(selects); !slices.Equal(got, want) {
		t.Errorf("select enum = %v, want %v", got, want)
	}
	if got, want := props["level"].Enum, schematest.Strings(levels); !slices.Equal(got, want) {
		t.Errorf("level enum = %v, want %v", got, want)
	}

	kebab := map[string]*schematest.Node{
		"id":                          props["id"],
		"category":                    props["category"],
		"topic":                       props["topic"],
		"requires.items":              props["requires"].Items,
		"requires_one_of.items.items": props["requires_one_of"].Items.Items,
		"conflicts.items":             props["conflicts"].Items,
	}
	for path, node := range kebab {
		if node.Pattern != kebabCase.String() {
			t.Errorf("%s pattern = %q, want %q", path, node.Pattern, kebabCase.String())
		}
	}
}
