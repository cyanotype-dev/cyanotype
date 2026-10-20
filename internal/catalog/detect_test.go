package catalog

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/cyanotype-dev/cyanotype/internal/schematest"
)

// TestLoadDetect checks that every field of detect is decoded.
func TestLoadDetect(t *testing.T) {
	c, err := Load(os.DirFS(filepath.Join("testdata", "detect")))
	if err != nil {
		t.Fatalf("Load() error:\n%v", err)
	}
	e, _ := c.Get("alpha")
	want := &Detect{
		Files: []string{"alpha.mod"},
		Dependencies: &Dependencies{
			NPM:  []string{"@alpha/core"},
			PyPI: []string{"alpha-tools"},
			Go:   []string{"example.com/alpha"},
		},
		RunScript: "alpha run {script}",
	}
	if !reflect.DeepEqual(e.Detect, want) {
		t.Errorf("Detect = %+v, want %+v", e.Detect, want)
	}
}

func TestNormalizePyPI(t *testing.T) {
	tests := []struct{ in, want string }{
		{"fastapi", "fastapi"},
		{"Flask_SQLAlchemy", "flask-sqlalchemy"},
		{"zope.interface", "zope-interface"},
		{"a-_.b", "a-b"},
		{"PyYAML", "pyyaml"},
	}
	for _, tt := range tests {
		if got := NormalizePyPI(tt.in); got != tt.want {
			t.Errorf("NormalizePyPI(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// TestSchemaPyPIPattern keeps the schema's pattern for Python names equal
// to what validation accepts: a normalized name is always kebab-case.
func TestSchemaPyPIPattern(t *testing.T) {
	root := schematest.Load(t, filepath.Join("..", "..", "schema", "catalog-entry.schema.json"))
	node := root.Properties["detect"].Properties["dependencies"].Properties["pypi"].Items
	if node.Pattern != kebabCase.String() {
		t.Errorf("detect.dependencies.pypi items pattern = %q, want %q", node.Pattern, kebabCase.String())
	}
}
