package main

import (
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	bundled "github.com/cyanotype-dev/cyanotype/catalog"
	"github.com/cyanotype-dev/cyanotype/internal/catalog"
	"github.com/cyanotype-dev/cyanotype/internal/lock"
)

// catalogVersion returns the version of the bundled catalog, which init
// pins in every spec it writes.
func catalogVersion(t *testing.T) string {
	t.Helper()
	c, err := catalog.LoadRoot(bundled.FS())
	if err != nil {
		t.Fatal(err)
	}
	return c.Version()
}

// readSpec returns the cyanotype.yaml in dir.
func readSpec(t *testing.T, dir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "cyanotype.yaml")) //nolint:gosec // test temp dir
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// projectDir returns a new empty folder with the given name, so the
// default project name is predictable.
func projectDir(t *testing.T, name string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	if err := os.Mkdir(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestInitFlags runs init without a terminal, with every answer given as
// a flag, then generate on the spec it wrote.
func TestInitFlags(t *testing.T) {
	dir := projectDir(t, "recipe-box")
	r := runCLI(t, nonInteractive, failReader{t}, "init", "--dir", dir,
		"--preset", "nextjs-supabase", "--autonomy", "asks-often", "--i18n", "yes", "--locales", "en,IT",
		"--replies", "Italian", "--agents", "codex,claude", "--goal", "Web app where families save and share recipes.")
	if r.code != codeOK || r.err != nil {
		t.Fatalf("exit code %d, error %v\nstderr:\n%s", r.code, r.err, r.stderr)
	}

	want := `version: 1

catalog_version: "` + catalogVersion(t) + `"

targets:
  - claude
  - agents-md

mode: simple

project:
  name: recipe-box
  goal: Web app where families save and share recipes.

languages:
  agent_replies: italian

commands:
  dev: pnpm dev
  build: pnpm build
  test: pnpm test
  lint: pnpm lint

selections:
  - typescript
  - node
  - pnpm
  - nextjs
  - tailwind
  - shadcn-ui
  - supabase
  - vitest
  - eslint-prettier
  - i18n-rules
  - i18n-next-intl
  - conventional-commits
  - small-commits
  - plan-before-code
  - definition-of-done
  - no-destructive-commands
  - no-test-tampering
  - secrets-in-env
  - no-unrequested-changes
  - no-duplicate-files
  - rule-maintenance-propose
  - dependency-approval

i18n:
  default_locale: en
  locales:
    - en
    - it
`
	// --locales en,IT is written in canonical case.
	if got := readSpec(t, dir); got != want {
		t.Errorf("cyanotype.yaml:\n%s\nwant:\n%s", got, want)
	}
	for _, s := range []string{
		"Starter: Next.js + Supabase\n",
		"\nStack\n  - TypeScript: ",
		"\nTranslations (en, it; en is the default)\n",
		"\nAgent rules (autonomy: asks often)\n",
		"Project name: recipe-box (the folder name; to change it, pass --name or edit project.name in cyanotype.yaml)\n",
		"Goal: Web app where families save and share recipes.\n",
		"Agents: Codex, Claude Code\n",
		"Instruction files: CLAUDE.md, AGENTS.md\n",
		"Wrote cyanotype.yaml",
		`Next: run "cyanotype generate --dir `,
	} {
		if !strings.Contains(r.stdout, s) {
			t.Errorf("stdout is missing %q:\n%s", s, r.stdout)
		}
	}

	g := runCLI(t, nonInteractive, failReader{t}, "generate", "--dir", dir, "--yes")
	if g.code != codeOK || g.err != nil || g.stderr != "" {
		t.Errorf("generate after init: exit code %d, error %v, stderr:\n%s", g.code, g.err, g.stderr)
	}
	files := keys(tree(t, dir))
	for _, f := range []string{"AGENTS.md", "CLAUDE.md", ".claude/settings.json", ".gitignore"} {
		if !slices.Contains(files, f) {
			t.Errorf("generate did not write %s; files: %q", f, files)
		}
	}
}

// TestInitPresetImpliedByType checks that a type and a language with one
// preset need no --preset, and that answers left out use their defaults.
func TestInitPresetImpliedByType(t *testing.T) {
	dir := projectDir(t, "renamer")
	r := runCLI(t, nonInteractive, failReader{t}, "init", "--dir", dir, "--type", "script", "--language", "go",
		"--autonomy", "autonomous", "--agents", "claude", "--goal", "Renames photos by date.", "--name", "photo-renamer")
	if r.code != codeOK || r.err != nil {
		t.Fatalf("exit code %d, error %v\nstderr:\n%s", r.code, r.err, r.stderr)
	}
	got := readSpec(t, dir)
	for _, s := range []string{
		"targets:\n  - claude\n\n", "  name: photo-renamer\n", "  - go\n  - golangci-lint\n", "  - branch-per-task\n",
	} {
		if !strings.Contains(got, s) {
			t.Errorf("cyanotype.yaml is missing %q:\n%s", s, got)
		}
	}
	for _, s := range []string{"languages:", "i18n:", "learning-", "dependency-approval"} {
		if strings.Contains(got, s) {
			t.Errorf("cyanotype.yaml has %q, which the answers did not ask for:\n%s", s, got)
		}
	}
	// A name given with --name is shown as it is, without the hint about
	// the folder name.
	if !strings.Contains(r.stdout, "Project name: photo-renamer\n") || strings.Contains(r.stdout, "the folder name") {
		t.Errorf("summary does not show the --name project name plainly:\n%s", r.stdout)
	}
}

func TestInitMissingFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
		not  []string
	}{
		{"nothing", nil,
			[]string{"--type ", "--autonomy ", "--goal ", "--agents "},
			[]string{"--preset", "--i18n", "--name"}},
		{"website in TypeScript", []string{"--type", "website", "--language", "typescript"},
			[]string{"--preset ", "nextjs-supabase or vite-react-app", "--autonomy ", "--i18n ", "--goal ", "--agents "},
			[]string{"--type "}},
		{"multilingual without locales", []string{"--preset", "expo-app", "--i18n", "yes", "--autonomy", "balanced",
			"--goal", "x", "--agents", "claude"},
			[]string{"--locales "},
			[]string{"--i18n ", "--goal "}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := projectDir(t, "demo")
			r := runCLI(t, nonInteractive, failReader{t}, append([]string{"init", "--dir", dir}, tt.args...)...)
			if r.code != codeUsage || r.err == nil {
				t.Fatalf("exit code %d, error %v; want %d and an error", r.code, r.err, codeUsage)
			}
			msg := r.err.Error()
			if !strings.Contains(msg, "stdin is not a terminal") {
				t.Errorf("error does not say why it cannot ask:\n%s", msg)
			}
			for _, s := range tt.want {
				if !strings.Contains(msg, s) {
					t.Errorf("error is missing %q:\n%s", s, msg)
				}
			}
			for _, s := range tt.not {
				if strings.Contains(msg, s) {
					t.Errorf("error mentions %q, which was answered or does not apply:\n%s", s, msg)
				}
			}
			assertNoSpec(t, dir)
		})
	}
}

func TestInitInvalidFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"coming later", []string{"--type", "game"}, "--type game: game is coming in a later version"},
		{"unknown value", []string{"--autonomy", "wild"}, `--autonomy "wild": must be asks-often, balanced or autonomous`},
		{"unknown preset", []string{"--preset", "rails"}, `--preset "rails": unknown preset`},
		{"preset and type disagree", []string{"--preset", "go-cli", "--type", "api"}, "--preset go-cli builds a script, but --type is api"},
		{"bad locale", []string{"--locales", "english"}, `"english" is not a locale tag`},
		{"locales without i18n", []string{"--i18n", "no", "--locales", "en"}, "--locales needs --i18n later or yes"},
		{"one locale for yes", []string{"--i18n", "yes", "--locales", "en"}, "at least two --locales"},
		{"unknown agent", []string{"--agents", "claude,robot"}, `--agents "robot"`},
		{"empty goal", []string{"--goal", " "}, "--goal is empty"},
		{"i18n for a script", []string{"--preset", "go-cli", "--i18n", "later", "--autonomy", "balanced",
			"--goal", "x", "--agents", "claude"}, "--i18n and --locales apply only to websites and mobile apps"},
		{"argument", []string{"extra"}, `"cyanotype init" takes no arguments, got "extra"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := projectDir(t, "demo")
			r := runCLI(t, nonInteractive, failReader{t}, append([]string{"init", "--dir", dir}, tt.args...)...)
			if r.code != codeUsage || r.err == nil || !strings.Contains(r.err.Error(), tt.want) {
				t.Errorf("exit code %d, error %v; want %d and an error containing %q", r.code, r.err, codeUsage, tt.want)
			}
			assertNoSpec(t, dir)
		})
	}
}

func TestInitRefusesExistingSpec(t *testing.T) {
	const existing = "# mine\n"
	dir := newProject(t, map[string]string{"cyanotype.yaml": existing})
	r := runCLI(t, interactive, failReader{t}, "init", "--dir", dir)
	if r.code != codeError || r.err == nil || !strings.Contains(r.err.Error(), "init never overwrites it") {
		t.Errorf("exit code %d, error %v; want %d and a refusal", r.code, r.err, codeError)
	}
	if got := readSpec(t, dir); got != existing {
		t.Errorf("the existing spec was changed to %q", got)
	}
}

// TestInitInteractive answers every question on a terminal, with a wrong
// answer on the way, and declines to generate at the end.
func TestInitInteractive(t *testing.T) {
	dir := projectDir(t, "family-recipes")
	answers := strings.Join([]string{
		"1",        // What to build: website
		"none",     // Known language: not yet, by value
		"",         // Starter: the default, nextjs-supabase
		"9",        // Learning mode: out of range, asked again
		"3",        // Learning mode: guided
		"",         // Autonomy: the default, balanced
		"2",        // Translations: later
		"",         // First locale: the default, en
		"Italian",  // Replies
		"",         // Goal: required, asked again
		"Recipes.", // Goal
		"1, 2",     // Agents: Claude Code and Codex
		"n",        // Generate now: no
	}, "\n") + "\n"
	r := runCLI(t, interactive, strings.NewReader(answers), "init", "--dir", dir)
	if r.code != codeOK || r.err != nil {
		t.Fatalf("exit code %d, error %v\nstderr:\n%s", r.code, r.err, r.stderr)
	}

	got := readSpec(t, dir)
	for _, s := range []string{
		"  name: family-recipes\n", "  goal: Recipes.\n", "  agent_replies: italian\n",
		"  - i18n-rules\n  - i18n-next-intl\n", "  - learning-guided\n", "  default_locale: en\n",
	} {
		if !strings.Contains(got, s) {
			t.Errorf("cyanotype.yaml is missing %q:\n%s", s, got)
		}
	}
	for _, s := range []string{
		"What do you want to build?",
		"   -  Desktop app (coming later)",
		"Which starter fits best?",
		"Turn on learning mode, to learn TypeScript while the agent works?",
		"Please answer with a number from 1 to 4.",
		"An answer is required.",
		"Generate the instruction files now? [Y/n]",
	} {
		if !strings.Contains(r.stderr, s) {
			t.Errorf("stderr is missing %q:\n%s", s, r.stderr)
		}
	}
	if strings.Contains(r.stdout, "What do you want to build?") {
		t.Errorf("questions went to stdout:\n%s", r.stdout)
	}
	if !strings.Contains(r.stdout, `Run "cyanotype generate --dir `) {
		t.Errorf("stdout does not say how to generate later:\n%s", r.stdout)
	}
	if files := keys(tree(t, dir)); !slices.Equal(files, []string{"cyanotype.yaml"}) {
		t.Errorf("declining to generate still wrote %q", files)
	}
}

// TestInitInteractiveGenerates accepts the offer to generate, and checks
// that generate asks for its own confirmation on the same input.
func TestInitInteractiveGenerates(t *testing.T) {
	dir := projectDir(t, "renamer")
	answers := "y\ny\n" // generate now, then write these files
	r := runCLI(t, interactive, strings.NewReader(answers), "init", "--dir", dir, "--preset", "go-cli",
		"--language", "go", "--autonomy", "balanced", "--replies", "english", "--agents", "claude", "--goal", "Renames photos.")
	if r.code != codeOK || r.err != nil {
		t.Fatalf("exit code %d, error %v\nstderr:\n%s", r.code, r.err, r.stderr)
	}
	if !strings.Contains(r.stderr, "Write these files? [y/N]") || !strings.Contains(r.stdout, "Wrote 4 files.") {
		t.Errorf("generate did not run through its confirmation:\nstdout:\n%s\nstderr:\n%s", r.stdout, r.stderr)
	}
	if files := keys(tree(t, dir)); !slices.Equal(files, []string{".claude/settings.json", ".gitignore", "CLAUDE.md", "cyanotype.lock", "cyanotype.yaml"}) {
		t.Errorf("files = %q", files)
	}
	// The lock records the generated files it can hash, and the rules
	// written to the new settings file, all of which cyanotype added.
	l, err := lock.Parse([]byte(tree(t, dir)["cyanotype.lock"]))
	if err != nil {
		t.Fatal(err)
	}
	if got := slices.Sorted(maps.Keys(l.Files)); !slices.Equal(got, []string{".gitignore", "CLAUDE.md"}) {
		t.Errorf("lock records %q", got)
	}
	if l.Claude.Empty() {
		t.Error("lock records no permission rules, though settings.json was created with some")
	}
}

func TestInitInputEnds(t *testing.T) {
	dir := projectDir(t, "demo")
	r := runCLI(t, interactive, strings.NewReader("1\n"), "init", "--dir", dir)
	if r.code != codeError || !errors.Is(r.err, errInputEnded) {
		t.Errorf("exit code %d, error %v; want %d and errInputEnded", r.code, r.err, codeError)
	}
	assertNoSpec(t, dir)
}

func assertNoSpec(t *testing.T, dir string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(dir, "cyanotype.yaml")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("cyanotype.yaml was written: %v", err)
	}
}
