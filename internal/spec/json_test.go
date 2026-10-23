package spec

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// TestJSONTagsMatchYAML checks that every field has the same name in JSON
// as in YAML, and is left out in JSON exactly when it is left out in YAML,
// so the spec reads the same in the JSON output as in cyanotype.yaml.
func TestJSONTagsMatchYAML(t *testing.T) {
	checkTags(t, "spec", reflect.TypeFor[Spec]())
}

func checkTags(t *testing.T, path string, typ reflect.Type) {
	t.Helper()
	for f := range typ.Fields() {
		yamlName, yamlOpts, _ := strings.Cut(f.Tag.Get("yaml"), ",")
		jsonName, jsonOpts, _ := strings.Cut(f.Tag.Get("json"), ",")
		p := path + "." + f.Name
		if jsonName != yamlName {
			t.Errorf("%s: json name %q, yaml name %q", p, jsonName, yamlName)
		}
		yamlOmits := strings.Contains(yamlOpts, "omitempty")
		jsonOmits := strings.Contains(jsonOpts, "omitempty") || strings.Contains(jsonOpts, "omitzero")
		if yamlOmits != jsonOmits {
			t.Errorf("%s: omitted in yaml %v, in json %v", p, yamlOmits, jsonOmits)
		}
		elem := f.Type
		for elem.Kind() == reflect.Pointer || elem.Kind() == reflect.Slice {
			elem = elem.Elem()
		}
		if elem.Kind() == reflect.Struct {
			checkTags(t, p, elem)
		}
	}
}

func TestJSONLeavesOutUnsetFields(t *testing.T) {
	s := Spec{Version: 1, Targets: []Target{TargetClaude}, Project: Project{Name: "a", Goal: "b"}}
	got, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"version":1,"targets":["claude"],"project":{"name":"a","goal":"b"}}`; string(got) != want {
		t.Errorf("json.Marshal() = %s, want %s", got, want)
	}
}
