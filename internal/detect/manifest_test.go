package detect

import (
	"reflect"
	"strings"
	"testing"
)

func TestParsePackageJSON(t *testing.T) {
	p, err := parsePackageJSON([]byte(`{
		"name": "app",
		"dependencies": {"next": "15", "react": "19"},
		"devDependencies": {"typescript": "5", "react": "19"},
		"optionalDependencies": {"fsevents": "2"},
		"peerDependencies": {"@types/node": "*"},
		"scripts": {"test": "vitest", "dev": "next dev", "build": 3}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"@types/node", "fsevents", "next", "react", "typescript"}; !reflect.DeepEqual(p.dependencies, want) {
		t.Errorf("dependencies = %q, want %q", p.dependencies, want)
	}
	if want := []string{"build", "dev", "test"}; !reflect.DeepEqual(p.scripts, want) {
		t.Errorf("scripts = %q, want %q", p.scripts, want)
	}

	if _, err := parsePackageJSON([]byte(`{"dependencies": [1, 2]}`)); err == nil {
		t.Error("dependencies as an array: want an error")
	}
}

func TestParseRequirements(t *testing.T) {
	got := parseRequirements([]byte(strings.Join([]string{
		"# comment",
		"FastAPI[standard]>=0.115  # inline comment",
		"psycopg2_binary==2.9 \\",
		"    --hash=sha256:abc",
		"-r dev.txt",
		"-e ./lib",
		"./wheels/thing.whl",
		"https://example.com/pkg.tar.gz",
		"requests @ https://example.com/requests.whl",
		"uvicorn ; python_version >= '3.9'",
		"fastapi",
		"",
	}, "\r\n")))
	want := []string{"fastapi", "psycopg2-binary", "requests", "uvicorn"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseRequirements = %q, want %q", got, want)
	}
}

func TestParsePyProject(t *testing.T) {
	tests := []struct {
		name string
		toml string
		want []string
	}{
		{
			name: "PEP 621 dependencies and optional dependencies",
			toml: `[project]
name = "api"
version = "0.1.0"
requires-python = ">=3.12"
dependencies = [
    "fastapi[standard]>=0.115",  # web
    'SQLAlchemy>=2',
    "psycopg[binary]; python_version >= '3.9'",
]

[project.optional-dependencies]
test = ["pytest>=8", "pytest-cov"]
`,
			want: []string{"fastapi", "psycopg", "pytest", "pytest-cov", "sqlalchemy"},
		},
		{
			name: "dependency groups skip include-group tables",
			toml: `[dependency-groups]
dev = [
    { include-group = "test" },
    "ruff>=0.6",
]
test = ["pytest"]
`,
			want: []string{"pytest", "ruff"},
		},
		{
			name: "uv dev-dependencies",
			toml: "[tool.uv]\ndev-dependencies = [\"mypy\"]\npackage = true\n",
			want: []string{"mypy"},
		},
		{
			name: "poetry keys, python excluded, quoted keys with dots",
			toml: `[tool.poetry]
name = "app"

[tool.poetry.dependencies]
python = "^3.12"
FastAPI = { version = "^0.115", extras = ["standard"] }
"zope.interface" = "^6"

[tool.poetry.group.dev.dependencies]
pytest = "^8"

[tool.poetry.dev-dependencies]
black = "*"
`,
			want: []string{"black", "fastapi", "pytest", "zope-interface"},
		},
		{
			name: "dotted keys at the top level",
			toml: "project.dependencies = [\"httpx\"]\n",
			want: []string{"httpx"},
		},
		{
			name: "other tables, values and arrays of tables are skipped",
			toml: `[build-system]
requires = ["hatchling"]
build-backend = "hatchling.build"

[tool.ruff]
line-length = 100
exclude = ["build"]
"quoted key" = true
date = 1979-05-27T07:32:00Z

[[tool.mypy.overrides]]
module = "x.*"

[tool.example]
text = """
dependencies = ["not-a-dependency"]
"""
literal = '''raw "quotes"'''
escaped = "a \" b"
`,
			want: []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parsePyProject([]byte(tt.toml))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parsePyProject = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParsePyProjectErrors(t *testing.T) {
	tests := []struct{ name, toml, msg string }{
		{"unclosed array", "[project]\ndependencies = [\n  \"fastapi\",\n", "line 2: array is never closed"},
		{"unclosed string", "[project]\nname = \"api\n", "line 2: string is never closed"},
		{"unclosed table header", "[project\n", "expected ']'"},
		{"missing equals", "[project]\nname \"api\"\n", "expected '='"},
		{"junk after value", "[project]\nname = \"api\" x\n", "unexpected 'x' after a value"},
		{"unclosed inline table", "[x]\na = { b = 1\n", "line 2: inline table is never closed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parsePyProject([]byte(tt.toml))
			if err == nil || !strings.Contains(err.Error(), tt.msg) {
				t.Errorf("error = %v, want it to contain %q", err, tt.msg)
			}
		})
	}
}

func TestParseGoMod(t *testing.T) {
	got, err := parseGoMod([]byte("module example.com/app\r\n\r\ngo 1.27\r\n\r\n" +
		"require github.com/spf13/cobra v1.10.2\r\n" +
		"require (\r\n" +
		"\tgithub.com/jackc/pgx/v5 v5.7.0 // database\r\n" +
		"\tgolang.org/x/sys v0.49.0 // indirect\r\n" +
		"\t\"example.com/quoted\" v1.0.0\r\n" +
		")\r\n" +
		"require(\n\tmodernc.org/sqlite v1.30.0\n)\n" +
		"replace example.com/quoted => ../quoted\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"github.com/spf13/cobra", "github.com/jackc/pgx/v5", "example.com/quoted", "modernc.org/sqlite"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseGoMod = %q, want %q", got, want)
	}

	if _, err := parseGoMod([]byte("module x\n\nrequire (\n\ta v1\n")); err == nil || !strings.Contains(err.Error(), "line 3: require block is never closed") {
		t.Errorf("unclosed block: error = %v", err)
	}
}

func TestModulePathMatches(t *testing.T) {
	tests := []struct {
		module, path string
		want         bool
	}{
		{"github.com/jackc/pgx", "github.com/jackc/pgx", true},
		{"github.com/jackc/pgx/v5", "github.com/jackc/pgx", true},
		{"github.com/jackc/pgxpool", "github.com/jackc/pgx", false},
		{"github.com/jackc", "github.com/jackc/pgx", false},
	}
	for _, tt := range tests {
		if got := modulePathMatches(tt.module, tt.path); got != tt.want {
			t.Errorf("modulePathMatches(%q, %q) = %v, want %v", tt.module, tt.path, got, tt.want)
		}
	}
}
