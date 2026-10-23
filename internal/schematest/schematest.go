// Package schematest checks that a JSON Schema in schema/ describes the same
// fields as a Go type. The file schemas exist for editor autocompletion
// while the Go code does the real validation, and the output schema
// documents the JSON contract the Go types produce, so without this check
// the two would drift apart silently. It is meant to be used from tests
// only, like net/http/httptest.
package schematest

import (
	"bytes"
	"encoding/json"
	"maps"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// Node is the subset of JSON Schema used by the schemas in schema/.
type Node struct {
	Type       string           `json:"type"`
	Required   []string         `json:"required"`
	Properties map[string]*Node `json:"properties"`
	Items      *Node            `json:"items"`
	Enum       []string         `json:"enum"`
	Pattern    string           `json:"pattern"`
	// PropertyNames constrains the keys of a map, such as their pattern.
	PropertyNames *Node `json:"propertyNames"`
	// AdditionalProperties is either false, for a closed object, or the
	// schema of every value, for a map. It stays raw because its JSON type varies.
	AdditionalProperties json.RawMessage `json:"additionalProperties"`
	// Ref points to a definition of the same file, "#/$defs/<name>".
	Ref string `json:"$ref"`
	// Defs holds the definitions a Ref can point to, at the root.
	Defs map[string]*Node `json:"$defs"`
}

// Load reads and parses the schema file at path, failing the test on error.
func Load(t testing.TB, path string) *Node {
	t.Helper()
	data, err := os.ReadFile(path) //nolint:gosec // path is a constant in test code, never user input

	if err != nil {
		t.Fatal(err)
	}
	var root Node
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return &root
}

// CheckStruct compares the yaml fields of typ with the properties of node,
// recursing into nested structs. A field is required exactly when its yaml
// tag has no omitempty, which is the convention of every Cyanotype file
// format. path names the struct in error messages.
func CheckStruct(t testing.TB, path string, typ reflect.Type, node *Node) {
	t.Helper()
	checker{tag: "yaml"}.checkStruct(t, path, typ, node)
}

// CheckJSONStruct compares the json fields of typ with the definition name
// in the $defs of root, following $ref, recursing into nested structs and
// flattening embedded ones as encoding/json does. A field is required
// exactly when its json tag has neither omitempty nor omitzero, since only
// those fields appear in every output.
func CheckJSONStruct(t testing.TB, root *Node, name string, typ reflect.Type) {
	t.Helper()
	node, ok := root.Defs[name]
	if !ok {
		t.Errorf("schema has no $defs.%s", name)
		return
	}
	checker{tag: "json", root: root}.checkStruct(t, name, typ, node)
}

// checker compares Go types with schema nodes through one struct tag.
type checker struct {
	tag  string
	root *Node // for $ref; nil when the schema has no definitions
}

// field is a struct field as its tag names it.
type field struct {
	name     string
	required bool
	typ      reflect.Type
}

// fields returns the fields of typ as the checker's tag names them,
// flattening embedded structs for json.
func (c checker) fields(typ reflect.Type) []field {
	var out []field
	for f := range typ.Fields() {
		tag, ok := f.Tag.Lookup(c.tag)
		if c.tag == "json" && f.Anonymous && !ok {
			out = append(out, c.fields(f.Type)...)
			continue
		}
		name, opts, _ := strings.Cut(tag, ",")
		if name == "-" {
			continue
		}
		optional := strings.Contains(opts, "omitempty") || strings.Contains(opts, "omitzero")
		out = append(out, field{name: name, required: !optional, typ: f.Type})
	}
	return out
}

// resolve follows a $ref to its definition.
func (c checker) resolve(t testing.TB, path string, node *Node) *Node {
	t.Helper()
	if node.Ref == "" {
		return node
	}
	name, ok := strings.CutPrefix(node.Ref, "#/$defs/")
	if !ok || c.root == nil || c.root.Defs[name] == nil {
		t.Errorf("%s: $ref %q points to no definition", path, node.Ref)
		return &Node{}
	}
	return c.root.Defs[name]
}

func (c checker) checkStruct(t testing.TB, path string, typ reflect.Type, node *Node) {
	t.Helper()
	node = c.resolve(t, path, node)
	if node.Type != "object" || !bytes.Equal(node.AdditionalProperties, []byte("false")) {
		t.Errorf("%s: schema must be an object with additionalProperties: false", path)
	}

	var names, required []string
	for _, f := range c.fields(typ) {
		names = append(names, f.name)
		if f.required {
			required = append(required, f.name)
		}
		prop, ok := node.Properties[f.name]
		if !ok {
			continue // reported below
		}
		c.checkField(t, path+"."+f.name, f.typ, prop)
	}

	if got, want := slices.Sorted(maps.Keys(node.Properties)), sorted(names); !slices.Equal(got, want) {
		t.Errorf("%s: schema properties = %v, Go fields = %v", path, got, want)
	}
	if got, want := sorted(node.Required), sorted(required); !slices.Equal(got, want) {
		t.Errorf("%s: schema required = %v, Go fields that are never omitted = %v", path, got, want)
	}
}

func (c checker) checkField(t testing.TB, path string, typ reflect.Type, prop *Node) {
	t.Helper()
	prop = c.resolve(t, path, prop)
	if typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	switch typ.Kind() {
	case reflect.Struct:
		c.checkStruct(t, path, typ, prop)
	case reflect.Slice:
		if prop.Type != "array" || prop.Items == nil {
			t.Errorf("%s: schema must be an array with items", path)
			return
		}
		c.checkField(t, path+"[]", typ.Elem(), prop.Items)
	case reflect.Map:
		// A map's values are described by additionalProperties, which is
		// checked like any other field: a string, or a nested struct.
		var values Node
		if typ.Key().Kind() != reflect.String || prop.Type != "object" ||
			json.Unmarshal(prop.AdditionalProperties, &values) != nil || values.Type == "" {
			t.Errorf("%s: schema must be an object with additionalProperties describing its values", path)
			return
		}
		c.checkField(t, path+"{}", typ.Elem(), &values)
	case reflect.String:
		if prop.Type != "string" && prop.Enum == nil {
			t.Errorf("%s: schema must be a string or an enum", path)
		}
	case reflect.Int:
		if prop.Type != "integer" {
			t.Errorf("%s: schema must be an integer", path)
		}
	case reflect.Bool:
		if prop.Type != "boolean" {
			t.Errorf("%s: schema must be a boolean", path)
		}
	default:
		t.Errorf("%s: unexpected Go kind %s; extend schematest", path, typ.Kind())
	}
}

// Strings converts a list of named string values, such as enum constants,
// to plain strings for comparison with a schema enum.
func Strings[S ~string](values []S) []string {
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
