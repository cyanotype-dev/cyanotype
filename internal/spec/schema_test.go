package spec

import (
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"testing"

	"github.com/cyanotype-dev/cyanotype/internal/schematest"
)

// TestSchemaMatchesSpec keeps the editor schema in sync with the Go types:
// same fields, same required fields, same enums and the same patterns.
func TestSchemaMatchesSpec(t *testing.T) {
	root := schematest.Load(t, filepath.Join("..", "..", "schema", "spec.schema.json"))

	schematest.CheckStruct(t, "spec", reflect.TypeFor[Spec](), root)

	props := root.Properties
	i18n := props["i18n"].Properties
	output := props["output"].Properties

	enums := []struct {
		path string
		got  []string
		want []string
	}{
		{"targets.items", props["targets"].Items.Enum, schematest.Strings(targets)},
		{"mode", props["mode"].Enum, schematest.Strings(modes)},
		{"i18n.translations_by", i18n["translations_by"].Enum, schematest.Strings(translationsBys)},
		{"output.split", output["split"].Enum, schematest.Strings(splits)},
	}
	for _, e := range enums {
		if !slices.Equal(e.got, e.want) {
			t.Errorf("%s enum = %v, want %v", e.path, e.got, e.want)
		}
	}

	patterns := []struct {
		path string
		node *schematest.Node
		want *regexp.Regexp
	}{
		{"catalog_version", props["catalog_version"], catalogVersion},
		{"selections.items", props["selections"].Items, kebabCase},
		{"locked.items", props["locked"].Items, kebabCase},
		{"i18n.default_locale", i18n["default_locale"], localeTag},
		{"i18n.locales.items", i18n["locales"].Items, localeTag},
		{"answers.propertyNames", props["answers"].PropertyNames, snakeCase},
	}
	for name, lang := range props["languages"].Properties {
		patterns = append(patterns, struct {
			path string
			node *schematest.Node
			want *regexp.Regexp
		}{"languages." + name, lang, languageName})
	}
	for _, p := range patterns {
		if p.node == nil || p.node.Pattern != p.want.String() {
			t.Errorf("%s pattern does not match %q", p.path, p.want.String())
		}
	}
}
