package contract

import (
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/cyanotype-dev/cyanotype/internal/adopt"
	"github.com/cyanotype-dev/cyanotype/internal/generate"
	"github.com/cyanotype-dev/cyanotype/internal/schematest"
)

// TestSchemaMatchesContract keeps schema/output.schema.json in sync with
// the output types: same fields, same fields always present, same enums.
func TestSchemaMatchesContract(t *testing.T) {
	root := schematest.Load(t, filepath.Join("..", "..", "schema", "output.schema.json"))

	outputs := map[string]reflect.Type{
		"generate": reflect.TypeFor[Generate](),
		"check":    reflect.TypeFor[Check](),
	}
	for name, typ := range outputs {
		schematest.CheckJSONStruct(t, root, name, typ)
	}

	defs := root.Defs
	enums := []struct {
		path string
		got  []string
		want []string
	}{
		{"diagnostic.severity", defs["diagnostic"].Properties["severity"].Enum, []string{SeverityError, SeverityWarning}},
		{"file.status", defs["file"].Properties["status"].Enum, statuses()},
		{"file.choice", defs["file"].Properties["choice"].Enum, choicesThatWrite()},
	}
	for _, e := range enums {
		if !slices.Equal(e.got, e.want) {
			t.Errorf("%s enum = %v, want %v", e.path, e.got, e.want)
		}
	}
}

func statuses() []string {
	var out []string
	for _, s := range []generate.Status{generate.Created, generate.Modified, generate.Deleted, generate.Unchanged} {
		out = append(out, s.String())
	}
	return out
}

// choicesThatWrite returns the choices a generated file can carry: import
// writes nothing, so no file ever has it.
func choicesThatWrite() []string {
	var out []string
	for _, c := range adopt.Choices() {
		if c.Writes() {
			out = append(out, string(c))
		}
	}
	return out
}
