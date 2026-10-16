package catalog

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// schemaNode is the subset of JSON Schema used by schema/catalog-entry.schema.json.
type schemaNode struct {
	Type                 string                 `json:"type"`
	AdditionalProperties *bool                  `json:"additionalProperties"`
	Required             []string               `json:"required"`
	Properties           map[string]*schemaNode `json:"properties"`
	Items                *schemaNode            `json:"items"`
	Enum                 []string               `json:"enum"`
	Pattern              string                 `json:"pattern"`
}

// TestSchemaMatchesEntry keeps the editor schema in sync with the Go types:
// same fields, same required fields, same enums and the same id pattern.
func TestSchemaMatchesEntry(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "schema", "catalog-entry.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var root schemaNode
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatalf("parse schema: %v", err)
	}

	checkStruct(t, "entry", reflect.TypeFor[Entry](), &root)

	props := root.Properties
	if got, want := props["select"].Enum, toStrings(selects); !slices.Equal(got, want) {
		t.Errorf("select enum = %v, want %v", got, want)
	}
	if got, want := props["level"].Enum, toStrings(levels); !slices.Equal(got, want) {
		t.Errorf("level enum = %v, want %v", got, want)
	}

	kebab := map[string]*schemaNode{
		"id":              props["id"],
		"category":        props["category"],
		"topic":           props["topic"],
		"requires.items":  props["requires"].Items,
		"conflicts.items": props["conflicts"].Items,
	}
	for path, node := range kebab {
		if node.Pattern != kebabCase.String() {
			t.Errorf("%s pattern = %q, want %q", path, node.Pattern, kebabCase.String())
		}
	}
}

// checkStruct compares the yaml fields of typ with the properties of node,
// recursing into nested structs.
func checkStruct(t *testing.T, path string, typ reflect.Type, node *schemaNode) {
	t.Helper()
	if node.Type != "object" || node.AdditionalProperties == nil || *node.AdditionalProperties {
		t.Errorf("%s: schema must be an object with additionalProperties: false", path)
	}

	var names, required []string
	for field := range typ.Fields() {
		name, opts, _ := strings.Cut(field.Tag.Get("yaml"), ",")
		names = append(names, name)
		if !strings.Contains(opts, "omitempty") {
			required = append(required, name)
		}

		prop, ok := node.Properties[name]
		if !ok {
			continue // reported below
		}
		checkField(t, path+"."+name, field.Type, prop)
	}

	if got, want := sortedKeys(node.Properties), sorted(names); !slices.Equal(got, want) {
		t.Errorf("%s: schema properties = %v, Go fields = %v", path, got, want)
	}
	if got, want := sorted(node.Required), sorted(required); !slices.Equal(got, want) {
		t.Errorf("%s: schema required = %v, Go fields without omitempty = %v", path, got, want)
	}
}

func checkField(t *testing.T, path string, typ reflect.Type, prop *schemaNode) {
	t.Helper()
	if typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	switch typ.Kind() {
	case reflect.Struct:
		checkStruct(t, path, typ, prop)
	case reflect.Slice:
		if prop.Type != "array" || prop.Items == nil || prop.Items.Type != "string" {
			t.Errorf("%s: schema must be an array of strings", path)
		}
	case reflect.String:
		if prop.Type != "string" && prop.Enum == nil {
			t.Errorf("%s: schema must be a string or an enum", path)
		}
	default:
		t.Errorf("%s: unexpected Go kind %s; extend this test", path, typ.Kind())
	}
}

func toStrings[S ~string](values []S) []string {
	out := make([]string, len(values))
	for i, v := range values {
		out[i] = string(v)
	}
	return out
}

func sorted(s []string) []string {
	s = slices.Clone(s)
	slices.Sort(s)
	return s
}

func sortedKeys[V any](m map[string]V) []string {
	return slices.Sorted(maps.Keys(m))
}
