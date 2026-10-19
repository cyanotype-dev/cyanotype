// Package schematest checks that a JSON Schema in schema/ describes the same
// fields as a Go type. The schemas exist for editor autocompletion while the
// Go code does the real validation, so without this check the two would drift
// apart silently. It is meant to be used from tests only, like net/http/httptest.
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
	if node.Type != "object" || !bytes.Equal(node.AdditionalProperties, []byte("false")) {
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

	if got, want := slices.Sorted(maps.Keys(node.Properties)), sorted(names); !slices.Equal(got, want) {
		t.Errorf("%s: schema properties = %v, Go fields = %v", path, got, want)
	}
	if got, want := sorted(node.Required), sorted(required); !slices.Equal(got, want) {
		t.Errorf("%s: schema required = %v, Go fields without omitempty = %v", path, got, want)
	}
}

func checkField(t testing.TB, path string, typ reflect.Type, prop *Node) {
	t.Helper()
	if typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	switch typ.Kind() {
	case reflect.Struct:
		CheckStruct(t, path, typ, prop)
	case reflect.Slice:
		if prop.Type != "array" || prop.Items == nil {
			t.Errorf("%s: schema must be an array with items", path)
			return
		}
		checkField(t, path+"[]", typ.Elem(), prop.Items)
	case reflect.Map:
		var values Node
		if typ.Key().Kind() != reflect.String || typ.Elem().Kind() != reflect.String ||
			prop.Type != "object" || json.Unmarshal(prop.AdditionalProperties, &values) != nil || values.Type != "string" {
			t.Errorf("%s: schema must be an object with additionalProperties of type string", path)
		}
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
