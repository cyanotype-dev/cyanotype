package check

import (
	"strings"
	"testing"
	"testing/fstest"

	bundled "github.com/cyanotype-dev/cyanotype/catalog"
	"github.com/cyanotype-dev/cyanotype/internal/catalog"
	"github.com/cyanotype-dev/cyanotype/internal/generate"
	"github.com/cyanotype-dev/cyanotype/internal/lock"
	"github.com/cyanotype-dev/cyanotype/internal/spec"
)

// baseSpec targets both main files and selects entries with permission
// rules and .gitignore lines, so every kind of generated file exists.
const baseSpec = `version: 1
targets: [agents-md, claude]
project:
  name: tiny
  goal: A tiny project for tests.
selections: [go, conventional-commits, secrets-in-env, no-destructive-commands]
`

func loadCatalog(t *testing.T) *catalog.Catalog {
	t.Helper()
	c, err := catalog.LoadRoot(bundled.FS())
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func loadSpec(t *testing.T, text string) *spec.File {
	t.Helper()
	f, err := spec.Load(strings.NewReader(text))
	if err != nil {
		t.Fatalf("test spec does not load:\n%v", err)
	}
	return f
}

// generated returns a project holding text as its spec and the files
// cyanotype generate --yes writes for it.
func generated(t *testing.T, text string) fstest.MapFS {
	t.Helper()
	project := fstest.MapFS{generate.SpecFile: {Data: []byte(text)}}
	regenerate(t, text, project)
	return project
}

// regenerate writes into project what generate would write for text.
func regenerate(t *testing.T, text string, project fstest.MapFS) {
	t.Helper()
	project[generate.SpecFile] = &fstest.MapFile{Data: []byte(text)}
	changes, _, err := generate.Preview(loadCatalog(t), loadSpec(t, text), project)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range changes {
		switch c.Status {
		case generate.Deleted:
			delete(project, c.Path)
		case generate.Created, generate.Modified:
			project[c.Path] = &fstest.MapFile{Data: c.New}
		}
	}
}

func run(t *testing.T, text string, project fstest.MapFS) Report {
	t.Helper()
	r, err := Run(loadCatalog(t), loadSpec(t, text), project, "", nil)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	return r
}

// edit replaces old with replacement in the file at path, which must
// contain old.
func edit(t *testing.T, project fstest.MapFS, path, old, replacement string) {
	t.Helper()
	data := string(project[path].Data)
	if !strings.Contains(data, old) {
		t.Fatalf("%s does not contain %q:\n%s", path, old, data)
	}
	project[path] = &fstest.MapFile{Data: []byte(strings.Replace(data, old, replacement, 1))}
}

// want describes a finding a test expects.
type want struct {
	kind     Kind
	severity Severity
	path     string
	msg      string // a part of the message
}

func checkFindings(t *testing.T, got Report, wants ...want) {
	t.Helper()
	if len(got.Findings) != len(wants) {
		t.Errorf("got %d findings, want %d:\n%s", len(got.Findings), len(wants), describe(got))
		return
	}
	for i, w := range wants {
		f := got.Findings[i]
		if f.Kind != w.kind || f.Severity != w.severity || f.Path != w.path || !strings.Contains(f.Msg, w.msg) {
			t.Errorf("finding %d = %+v\nwant %s %s %s containing %q", i, f, w.kind, w.severity, w.path, w.msg)
		}
	}
}

func describe(r Report) string {
	var b strings.Builder
	for _, f := range r.Findings {
		b.WriteString("  " + f.Path + ": " + f.Severity.String() + " " + string(f.Kind) + ": " + f.Msg + "\n")
	}
	return b.String()
}

func TestCleanProject(t *testing.T) {
	checkFindings(t, run(t, baseSpec, generated(t, baseSpec)))
}

// TestChangesThatLoseNothing are differences generate would rewrite but
// that are not drift: CRLF endings, and text inside a protected block.
func TestChangesThatLoseNothing(t *testing.T) {
	project := generated(t, baseSpec)
	for path, f := range project {
		if path != generate.SpecFile {
			project[path] = &fstest.MapFile{Data: []byte(strings.ReplaceAll(string(f.Data), "\n", "\r\n"))}
		}
	}
	edit(t, project, "AGENTS.md", "## Git workflow", "<!-- cyanotype:custom -->\r\nMy note.\r\n<!-- /cyanotype:custom -->\r\n\r\n## Git workflow")
	checkFindings(t, run(t, baseSpec, project))
}

func TestGeneratedFiles(t *testing.T) {
	moreSpec := strings.Replace(baseSpec, "secrets-in-env,", "secrets-in-env, small-commits,", 1)
	tests := []struct {
		name  string
		spec  string // the spec check runs with; baseSpec when empty
		setup func(t *testing.T, p fstest.MapFS)
		want  []want
	}{
		{
			name: "edited by hand",
			setup: func(t *testing.T, p fstest.MapFS) {
				edit(t, p, "AGENTS.md", "## Git workflow\n", "## Git workflow\n\n- My own rule.\n")
			},
			want: []want{{Edited, Error, "AGENTS.md", "was edited by hand since cyanotype generated it"}},
		},
		{
			name: "spec changed",
			spec: moreSpec,
			want: []want{{OutOfSync, Error, "AGENTS.md", "is out of sync with cyanotype.yaml or the catalog"}},
		},
		{
			name: "edited and spec changed",
			spec: moreSpec,
			setup: func(t *testing.T, p fstest.MapFS) {
				edit(t, p, "AGENTS.md", "## Git workflow\n", "## Git workflow\n\n- My own rule.\n")
			},
			want: []want{{Edited, Error, "AGENTS.md", "and cyanotype.yaml or the catalog changed too"}},
		},
		{
			name:  "missing file",
			setup: func(_ *testing.T, p fstest.MapFS) { delete(p, "CLAUDE.md") },
			want:  []want{{OutOfSync, Error, "CLAUDE.md", "is missing"}},
		},
		{
			name:  "written by hand",
			setup: func(_ *testing.T, p fstest.MapFS) { p["CLAUDE.md"] = &fstest.MapFile{Data: []byte("# My rules\n")} },
			want:  []want{{HandWritten, Error, "CLAUDE.md", "was written by hand"}},
		},
		{
			name: "rule removed from settings",
			setup: func(t *testing.T, p fstest.MapFS) {
				edit(t, p, ".claude/settings.json", "\"Bash(rm -rf *)\",\n", "")
			},
			want: []want{{OutOfSync, Error, ".claude/settings.json", "does not hold the permission rules cyanotype generates"}},
		},
		{
			name:  "gitignore block edited",
			setup: func(t *testing.T, p fstest.MapFS) { edit(t, p, ".gitignore", "!.env.example\n", "") },
			want:  []want{{Edited, Error, ".gitignore", "was edited by hand"}},
		},
		{
			name:  "gitignore block removed",
			setup: func(_ *testing.T, p fstest.MapFS) { p[".gitignore"] = &fstest.MapFile{Data: []byte("node_modules/\n")} },
			want:  []want{{OutOfSync, Error, ".gitignore", "has no cyanotype block"}},
		},
		{
			name:  "no lock",
			setup: func(_ *testing.T, p fstest.MapFS) { delete(p, lock.Path) },
			want: []want{
				{UnownedRules, Warning, ".claude/settings.json", "treats the rules already in this file as yours"},
				{OutOfSync, Error, lock.Path, "is missing, so check cannot tell hand edits"},
			},
		},
		{
			name: "no lock and an edit",
			setup: func(t *testing.T, p fstest.MapFS) {
				delete(p, lock.Path)
				edit(t, p, "AGENTS.md", "## Git workflow\n", "## Git workflow\n\n- My own rule.\n")
			},
			want: []want{
				{UnownedRules, Warning, ".claude/settings.json", "treats the rules already in this file as yours"},
				{OutOfSync, Error, "AGENTS.md", "no lock records it, so check cannot tell whether it was edited by hand"},
				{OutOfSync, Error, lock.Path, "is missing"},
			},
		},
		{
			name: "lock out of date alone",
			setup: func(t *testing.T, p fstest.MapFS) {
				l, err := lock.Parse(p[lock.Path].Data)
				if err != nil {
					t.Fatal(err)
				}
				l.Entries["go"] = lock.Hash([]byte("an older go entry"))
				p[lock.Path] = &fstest.MapFile{Data: l.Marshal()}
			},
			want: []want{{OutOfSync, Error, lock.Path, "is out of date, though every generated file is current"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			project := generated(t, baseSpec)
			if tt.setup != nil {
				tt.setup(t, project)
			}
			text := baseSpec
			if tt.spec != "" {
				text = tt.spec
				project[generate.SpecFile] = &fstest.MapFile{Data: []byte(text)}
			}
			checkFindings(t, run(t, text, project), tt.want...)
		})
	}
}

func TestStaleFiles(t *testing.T) {
	split := baseSpec + "output:\n  split: always\n"
	project := generated(t, split)
	if _, ok := project["docs/agents/git.md"]; !ok {
		t.Fatal("the split output has no docs/agents/git.md")
	}
	checkFindings(t, run(t, baseSpec, project),
		want{OutOfSync, Error, "AGENTS.md", "out of sync"},
		want{OutOfSync, Error, "docs/agents/git.md", "is no longer generated and nobody changed it since"},
		want{OutOfSync, Error, "docs/agents/style.md", "is no longer generated and nobody changed it since"},
	)

	edit(t, project, "docs/agents/git.md", "# Git workflow\n", "# Git workflow\n\n- Mine.\n")
	regenerate(t, baseSpec, project)
	checkFindings(t, run(t, baseSpec, project),
		want{Stale, Warning, "docs/agents/git.md", "is no longer generated, but it changed since cyanotype generated it"})
}

func TestStack(t *testing.T) {
	nodeSpec := "version: 1\ntargets: [agents-md]\nproject:\n  name: web\n  goal: A web app.\nselections: [typescript, node, pnpm]\n"
	project := generated(t, nodeSpec)
	project["package.json"] = &fstest.MapFile{Data: []byte(`{"devDependencies": {"vitest": "^3", "typescript": "^5"}}`)}
	project["pnpm-lock.yaml"] = &fstest.MapFile{Data: []byte("lockfileVersion: '9.0'\n")}
	project["package-lock.json"] = &fstest.MapFile{Data: []byte("{}")}
	project["requirements.txt"] = &fstest.MapFile{Data: []byte("fastapi==0.1\n")}

	checkFindings(t, run(t, nodeSpec, project),
		want{Undeclared, Warning, "package-lock.json", "suggests npm, but cyanotype.yaml selects pnpm; delete the file if it is a leftover"},
		want{Undeclared, Warning, "package.json", "lists vitest, which suggests Vitest, and cyanotype.yaml does not select it; add vitest to selections"},
		want{Undeclared, Warning, "requirements.txt", "FastAPI"},
		want{Undeclared, Warning, "requirements.txt", "Python"},
	)
}

func TestCatalogOlder(t *testing.T) {
	pinned := strings.Replace(baseSpec, "version: 1\n", "version: 1\ncatalog_version: \"2026.09\"\n", 1)
	project := generated(t, pinned)
	checkFindings(t, run(t, pinned, project),
		want{CatalogOlder, Warning, "cyanotype.yaml", "no selected entry changed since the files were generated"})

	l, err := lock.Parse(project[lock.Path].Data)
	if err != nil {
		t.Fatal(err)
	}
	l.Entries["secrets-in-env"] = lock.Hash([]byte("as an older catalog wrote it"))
	project[lock.Path] = &fstest.MapFile{Data: l.Marshal()}
	// The lock is out of date too, since its entry hash no longer matches.
	r := run(t, pinned, project)
	checkFindings(t, r,
		want{OutOfSync, Error, lock.Path, "is out of date"},
		want{CatalogOlder, Warning, "cyanotype.yaml", "selected entries changed since the files were generated: secrets-in-env"})
	if len(r.Findings) == 2 {
		if f := r.Findings[1]; f.Line != 2 || f.Field != "catalog_version" {
			t.Errorf("catalog-older finding at line %d, field %q; want line 2, catalog_version", f.Line, f.Field)
		}
	}

	delete(project, lock.Path)
	checkFindings(t, run(t, pinned, project),
		want{UnownedRules, Warning, ".claude/settings.json", "as yours"},
		want{OutOfSync, Error, lock.Path, "is missing"},
		want{CatalogOlder, Warning, "cyanotype.yaml", "cyanotype.lock is missing, so check cannot tell which selected entries changed"})
}

func TestOtherWarnings(t *testing.T) {
	text := strings.Replace(baseSpec, "targets: [agents-md, claude]", "targets: [agents-md, claude, cursor]", 1) +
		"output:\n  split: never\n  max_lines: 10\n"
	text = strings.Replace(text, "  goal: A tiny project for tests.\n", "  goal: A tiny project for tests.\n  brief: docs/PRODUCT.md\n", 1)
	checkFindings(t, run(t, text, generated(t, text)),
		want{MissingReference, Warning, "AGENTS.md", "points to docs/PRODUCT.md, which does not exist"},
		want{SpecWarning, Warning, "cyanotype.yaml", `targets[2]: cyanotype cannot generate files for "cursor" yet`},
		want{TooLong, Warning, "cyanotype.yaml", "output.max_lines: AGENTS.md has"},
	)
}

func TestSpecErrorsStopTheCheck(t *testing.T) {
	text := "version: 1\ntargets: [agents-md]\nproject:\n  name: x\n  goal: y\nselections: [pnpn]\n"
	_, err := Run(loadCatalog(t), loadSpec(t, text), fstest.MapFS{}, "", nil)
	if err == nil || !strings.Contains(err.Error(), `unknown entry "pnpn"`) {
		t.Errorf("Run() error = %v", err)
	}
}

func TestCount(t *testing.T) {
	r := Report{Findings: []Finding{{Severity: Error}, {Severity: Warning}, {Severity: Warning}}}
	if r.Count(Error) != 1 || r.Count(Warning) != 2 {
		t.Errorf("Count() = %d errors, %d warnings", r.Count(Error), r.Count(Warning))
	}
}
