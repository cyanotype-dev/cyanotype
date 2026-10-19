package preset

import (
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/cyanotype-dev/cyanotype/internal/schematest"
)

// TestSchemaMatchesPreset keeps the editor schema in sync with the Go type:
// same fields, same required fields, the project_type enum and the id
// pattern.
func TestSchemaMatchesPreset(t *testing.T) {
	root := schematest.Load(t, filepath.Join("..", "..", "schema", "preset.schema.json"))

	schematest.CheckStruct(t, "preset", reflect.TypeFor[Preset](), root)

	props := root.Properties
	if got, want := props["project_type"].Enum, schematest.Strings(projectTypes); !slices.Equal(got, want) {
		t.Errorf("project_type enum = %v, want %v", got, want)
	}
	kebab := map[string]*schematest.Node{
		"id":                    props["id"],
		"selections.items":      props["selections"].Items,
		"i18n_selections.items": props["i18n_selections"].Items,
	}
	for path, node := range kebab {
		if node.Pattern != kebabCase.String() {
			t.Errorf("%s pattern = %q, want %q", path, node.Pattern, kebabCase.String())
		}
	}
}
