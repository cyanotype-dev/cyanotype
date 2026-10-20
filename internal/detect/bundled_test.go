package detect_test

import (
	"slices"
	"testing"
	"testing/fstest"

	bundled "github.com/cyanotype-dev/cyanotype/catalog"
	"github.com/cyanotype-dev/cyanotype/internal/catalog"
	"github.com/cyanotype-dev/cyanotype/internal/detect"
	"github.com/cyanotype-dev/cyanotype/internal/preset"
)

// TestBundledPresetsAreDetected builds a typical project for every bundled
// preset and checks that detection finds the preset's whole stack with the
// catalog's detect fields, and nothing it should not.
func TestBundledPresetsAreDetected(t *testing.T) {
	c, err := catalog.LoadRoot(bundled.FS())
	if err != nil {
		t.Fatal(err)
	}
	presets, err := preset.LoadRoot(bundled.FS(), c)
	if err != nil {
		t.Fatal(err)
	}

	// undetectable lists entries no file or dependency reveals: the Go
	// standard library is the absence of a framework.
	undetectable := []string{"go-stdlib"}

	projects := map[string]map[string]string{
		"nextjs-supabase": {
			"package.json": `{"dependencies": {"next": "15", "@supabase/supabase-js": "2"},
				"devDependencies": {"typescript": "5", "tailwindcss": "4", "vitest": "3", "eslint": "9", "prettier": "3"}}`,
			"pnpm-lock.yaml":  "",
			"components.json": "{}",
		},
		"vite-react-app": {
			"package.json": `{"devDependencies": {"@vitejs/plugin-react": "4", "typescript": "5", "tailwindcss": "4",
				"vitest": "3", "eslint": "9"}}`,
			"pnpm-lock.yaml":  "",
			"components.json": "{}",
		},
		"expo-app": {
			"package.json":      `{"dependencies": {"expo": "52"}, "devDependencies": {"typescript": "5", "eslint": "9"}}`,
			"package-lock.json": "{}",
		},
		"fastapi-postgres": {
			"pyproject.toml": "[project]\nname = \"api\"\ndependencies = [\"fastapi[standard]\", \"psycopg[binary]\"]\n\n[dependency-groups]\ndev = [\"pytest\", \"ruff\"]\n",
			"uv.lock":        "",
		},
		"go-api": {
			"go.mod":        "module example.com/api\n\ngo 1.27\n\nrequire github.com/jackc/pgx/v5 v5.7.0\n",
			".golangci.yml": "version: \"2\"\n",
		},
		"python-script": {
			"pyproject.toml": "[project]\nname = \"tool\"\ndependencies = []\n\n[dependency-groups]\ndev = [\"pytest\", \"ruff\"]\n",
			"uv.lock":        "",
		},
		"go-cli": {
			"go.mod":        "module example.com/cli\n\ngo 1.27\n",
			".golangci.yml": "version: \"2\"\n",
		},
	}

	for _, p := range presets {
		t.Run(p.ID, func(t *testing.T) {
			files, ok := projects[p.ID]
			if !ok {
				t.Fatalf("no test project for preset %q; add one", p.ID)
			}
			fsys := fstest.MapFS{}
			for name, content := range files {
				fsys[name] = &fstest.MapFile{Data: []byte(content)}
			}
			r := detect.Detect(c, fsys)

			var want []string
			for _, id := range p.Selections {
				if !slices.Contains(undetectable, id) {
					want = append(want, id)
				}
			}
			if got := r.IDs(); !slices.Equal(slices.Sorted(slices.Values(got)), slices.Sorted(slices.Values(want))) {
				t.Errorf("detected %q, want %q", got, want)
			}
			if len(r.Clashes) > 0 || len(r.Problems) > 0 {
				t.Errorf("clashes %v, problems %v; want none", r.Clashes, r.Problems)
			}
		})
	}
}
