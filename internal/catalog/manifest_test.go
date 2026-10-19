package catalog

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadRootValid(t *testing.T) {
	c, err := LoadRoot(os.DirFS(filepath.Join("testdata", "root", "valid")))
	if err != nil {
		t.Fatalf("LoadRoot() error:\n%v", err)
	}
	if got, want := c.Version(), "2026.10.2"; got != want {
		t.Errorf("Version() = %q, want %q", got, want)
	}
	if _, ok := c.Get("alpha"); !ok {
		t.Error(`Get("alpha") found nothing; entries were not loaded`)
	}
}

func TestLoadRootInvalid(t *testing.T) {
	type wantErr struct{ file, field, msg string }
	tests := []struct {
		dir  string
		want []wantErr
	}{
		{"missing-manifest", []wantErr{
			{"catalog.yaml", "", "is missing"},
		}},
		{"unknown-field", []wantErr{
			{"catalog.yaml", "", `line 2: unknown field "name"`},
		}},
		// Problems in the manifest and in the entries come back together.
		{"bad-version", []wantErr{
			{"catalog.yaml", "version", `"2026.1" must be a date-based version`},
			{"beta.yaml", "category", "is required"},
			{"beta.yaml", "select", "is required"},
			{"beta.yaml", "summary", "is required"},
			{"beta.yaml", "snippet.rule", "is required"},
			{"beta.yaml", "snippet.why", "is required"},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.dir, func(t *testing.T) {
			c, err := LoadRoot(os.DirFS(filepath.Join("testdata", "root", tt.dir)))
			if err == nil {
				t.Fatalf("LoadRoot() = catalog %q, want error", c.Version())
			}
			got := validationErrors(t, err)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d errors, want %d:\n%v", len(got), len(tt.want), err)
			}
			for i, w := range tt.want {
				g := got[i]
				if g.File != w.file || g.Field != w.field || !strings.Contains(g.Msg, w.msg) {
					t.Errorf("error %d = %q\nwant file %q, field %q, message containing %q", i, g, w.file, w.field, w.msg)
				}
			}
		})
	}
}

func TestLoadRootMissingEntries(t *testing.T) {
	_, err := LoadRoot(os.DirFS(filepath.Join("testdata", "root", "no-entries")))
	if err == nil {
		t.Fatal("LoadRoot() error = nil, want error")
	}
	var ve *ValidationError
	if errors.As(err, &ve) {
		t.Errorf("LoadRoot() error = %v, want an I/O error, not a validation error", err)
	}
}
