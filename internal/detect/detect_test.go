package detect

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/cyanotype-dev/cyanotype/internal/catalog"
)

// testCatalog loads the small catalog in testdata/catalog.
func testCatalog(t *testing.T) *catalog.Catalog {
	t.Helper()
	c, err := catalog.LoadRoot(os.DirFS(filepath.Join("testdata", "catalog")))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// project builds an in-memory project from file contents.
func project(files map[string]string) fstest.MapFS {
	fsys := fstest.MapFS{}
	for name, content := range files {
		fsys[name] = &fstest.MapFile{Data: []byte(content)}
	}
	return fsys
}

func TestDetect(t *testing.T) {
	c := testCatalog(t)
	tests := []struct {
		name     string
		files    map[string]string
		findings []Finding
		clashes  []Clash
		scripts  []string
		problems []string
	}{
		{
			name:  "empty project",
			files: nil,
		},
		{
			name: "files and dependencies",
			files: map[string]string{
				"package.json":   `{"dependencies": {"next": "15.0.0"}, "devDependencies": {"pg": "8"}, "scripts": {"dev": "next dev", "lint": "eslint ."}}`,
				"pnpm-lock.yaml": "lockfileVersion: '9.0'\n",
				"next.config.ts": "export default {}\n",
			},
			findings: []Finding{
				{ID: "nextjs", Evidence: []Evidence{{File: "next.config.ts"}, {File: "package.json", Dependency: "next"}}},
				{ID: "node", Evidence: []Evidence{{File: "package.json"}}},
				{ID: "pnpm", Evidence: []Evidence{{File: "pnpm-lock.yaml"}}},
				{ID: "postgres", Evidence: []Evidence{{File: "package.json", Dependency: "pg"}}},
			},
			scripts: []string{"dev", "lint"},
		},
		{
			name: "two lockfiles clash in a radio category",
			files: map[string]string{
				"package.json":      `{}`,
				"package-lock.json": "{}",
				"pnpm-lock.yaml":    "",
			},
			findings: []Finding{
				{ID: "node", Evidence: []Evidence{{File: "package.json"}}},
				{ID: "npm", Evidence: []Evidence{{File: "package-lock.json"}}},
				{ID: "pnpm", Evidence: []Evidence{{File: "pnpm-lock.yaml"}}},
			},
			clashes: []Clash{{IDs: []string{"npm", "pnpm"}, Category: "package-manager"}},
		},
		{
			name: "declared conflict across categories",
			files: map[string]string{
				"go.mod":  "module example.com/app\n\nrequire modernc.org/sqlite v1.30.0\n",
				"kv.toml": "",
			},
			findings: []Finding{
				{ID: "cloud-kv", Evidence: []Evidence{{File: "kv.toml"}}},
				{ID: "sqlite", Evidence: []Evidence{{File: "go.mod", Dependency: "modernc.org/sqlite"}}},
			},
			clashes: []Clash{{IDs: []string{"cloud-kv", "sqlite"}}},
		},
		{
			name: "python manifests both count",
			files: map[string]string{
				"pyproject.toml":   "[project]\nname = \"api\"\ndependencies = [\"FastAPI[standard]>=0.115\"]\n",
				"requirements.txt": "psycopg2_binary==2.9\nfastapi\n",
			},
			findings: []Finding{
				{ID: "fastapi", Evidence: []Evidence{{File: "pyproject.toml", Dependency: "fastapi"}, {File: "requirements.txt", Dependency: "fastapi"}}},
				{ID: "postgres", Evidence: []Evidence{{File: "requirements.txt", Dependency: "psycopg2-binary"}}},
			},
		},
		{
			name: "go module major version",
			files: map[string]string{
				"go.mod": "module example.com/app\n\nrequire (\n\tgithub.com/jackc/pgx/v5 v5.7.0\n\tgithub.com/jackc/pgxlisten v0.1.0\n)\n",
			},
			findings: []Finding{
				{ID: "postgres", Evidence: []Evidence{{File: "go.mod", Dependency: "github.com/jackc/pgx/v5"}}},
			},
		},
		{
			name: "unreadable manifest is a problem, the rest still counts",
			files: map[string]string{
				"package.json": "{not json",
				".nvmrc":       "22\n",
			},
			findings: []Finding{
				{ID: "node", Evidence: []Evidence{{File: "package.json"}, {File: ".nvmrc"}}},
			},
			problems: []string{"package.json: invalid JSON"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Detect(c, project(tt.files))
			if !reflect.DeepEqual(r.Findings, tt.findings) {
				t.Errorf("Findings =\n%+v\nwant\n%+v", r.Findings, tt.findings)
			}
			if !reflect.DeepEqual(r.Clashes, tt.clashes) {
				t.Errorf("Clashes = %+v, want %+v", r.Clashes, tt.clashes)
			}
			if !reflect.DeepEqual(r.Scripts, tt.scripts) {
				t.Errorf("Scripts = %q, want %q", r.Scripts, tt.scripts)
			}
			if len(r.Problems) != len(tt.problems) {
				t.Fatalf("Problems = %v, want %d", r.Problems, len(tt.problems))
			}
			for i, p := range r.Problems {
				if !strings.Contains(p.Error(), tt.problems[i]) {
					t.Errorf("problem %d = %q, want it to contain %q", i, p.Error(), tt.problems[i])
				}
			}
			if r.Empty() != (len(tt.findings) == 0) {
				t.Errorf("Empty() = %v with %d findings", r.Empty(), len(tt.findings))
			}
		})
	}
}

func TestEvidenceString(t *testing.T) {
	if got := (Evidence{File: "pnpm-lock.yaml"}).String(); got != "pnpm-lock.yaml" {
		t.Errorf("file evidence = %q", got)
	}
	if got := (Evidence{File: "package.json", Dependency: "next"}).String(); got != "package.json: next" {
		t.Errorf("dependency evidence = %q", got)
	}
}

func TestResultHelpers(t *testing.T) {
	r := Result{Findings: []Finding{{ID: "node"}, {ID: "pnpm", Evidence: []Evidence{{File: "pnpm-lock.yaml"}}}}}
	if got := r.IDs(); !reflect.DeepEqual(got, []string{"node", "pnpm"}) {
		t.Errorf("IDs() = %q", got)
	}
	if f, ok := r.Find("pnpm"); !ok || len(f.Evidence) != 1 {
		t.Errorf(`Find("pnpm") = %+v, %v`, f, ok)
	}
	if _, ok := r.Find("npm"); ok {
		t.Error(`Find("npm") ok = true`)
	}
}
