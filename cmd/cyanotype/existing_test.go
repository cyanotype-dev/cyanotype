package main

import (
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// handWritten is a CLAUDE.md written before cyanotype.
const handWritten = "# Shop\n\nUse pnpm, never npm.\nRun pnpm test before every commit.\n"

// claudeSpec targets Claude Code only, so CLAUDE.md is the main file.
const claudeSpec = `version: 1
targets: [claude]
project:
  name: shop
  goal: An online shop.
selections: [conventional-commits]
`

// TestGenerateNeverReplacesHandWritten checks that --yes does not replace
// an instruction file cyanotype did not generate.
func TestGenerateNeverReplacesHandWritten(t *testing.T) {
	dir := newProject(t, map[string]string{"cyanotype.yaml": claudeSpec, "CLAUDE.md": handWritten})
	r := runCLI(t, nonInteractive, failReader{t}, "generate", "--dir", dir, "--yes")
	if r.code != codeUsage || r.err == nil {
		t.Fatalf("exit code %d, error %v; want a usage error", r.code, r.err)
	}
	for _, s := range []string{
		"CLAUDE.md has no cyanotype header", "not even with --yes",
		"--existing import ", "--existing keep ", "--existing move ", "--existing replace ",
	} {
		if !strings.Contains(r.err.Error(), s) {
			t.Errorf("error is missing %q:\n%s", s, r.err)
		}
	}
	files := tree(t, dir)
	if files["CLAUDE.md"] != handWritten {
		t.Errorf("CLAUDE.md was changed to:\n%s", files["CLAUDE.md"])
	}
	if len(files) != 2 {
		t.Errorf("files = %q; nothing should be written", keys(files))
	}
}

// generateAgain runs generate once more without --existing and fails the
// test unless everything is up to date: once a choice is written, the
// file is cyanotype's and keeps the hand-written text.
func generateAgain(t *testing.T, dir string) {
	t.Helper()
	r := runCLI(t, nonInteractive, failReader{t}, "generate", "--dir", dir, "--dry-run")
	if r.code != codeOK || !strings.Contains(r.stdout, "Everything is up to date") {
		t.Errorf("generating again: exit code %d, error %v\nstdout:\n%s", r.code, r.err, r.stdout)
	}
}

func TestGenerateExistingKeep(t *testing.T) {
	dir := newProject(t, map[string]string{"cyanotype.yaml": claudeSpec, "CLAUDE.md": handWritten})
	r := runCLI(t, nonInteractive, failReader{t}, "generate", "--dir", dir, "--existing", "keep", "--yes")
	if r.code != codeOK || r.err != nil {
		t.Fatalf("exit code %d, error %v\nstderr:\n%s", r.code, r.err, r.stderr)
	}
	if !strings.Contains(r.stdout, "modified   CLAUDE.md (written by hand: kept whole in a cyanotype:custom block)") {
		t.Errorf("summary does not say CLAUDE.md is kept:\n%s", r.stdout)
	}
	got := tree(t, dir)["CLAUDE.md"]
	if !strings.Contains(got, "# shop\n\n<!-- cyanotype:custom -->\n"+handWritten+"<!-- /cyanotype:custom -->\n") {
		t.Errorf("CLAUDE.md does not keep the hand-written file whole:\n%s", got)
	}
	generateAgain(t, dir)
}

func TestGenerateExistingMove(t *testing.T) {
	dir := newProject(t, map[string]string{"cyanotype.yaml": claudeSpec, "CLAUDE.md": handWritten})
	r := runCLI(t, nonInteractive, failReader{t}, "generate", "--dir", dir, "--existing", "move", "--yes")
	if r.code != codeOK || r.err != nil {
		t.Fatalf("exit code %d, error %v\nstderr:\n%s", r.code, r.err, r.stderr)
	}
	files := tree(t, dir)
	if files["docs/agents/original-claude.md"] != handWritten {
		t.Errorf("moved file = %q, want the hand-written CLAUDE.md", files["docs/agents/original-claude.md"])
	}
	if main := files["CLAUDE.md"]; strings.Contains(main, "never npm") || !strings.Contains(main, "read docs/agents/original-claude.md") {
		t.Errorf("CLAUDE.md should point to the moved file:\n%s", main)
	}
	for _, s := range []string{
		"modified   CLAUDE.md (written by hand: its content moves to docs/agents/original-claude.md)",
		"created    docs/agents/original-claude.md (the content of CLAUDE.md, byte for byte)",
	} {
		if !strings.Contains(r.stdout, s) {
			t.Errorf("summary is missing %q:\n%s", s, r.stdout)
		}
	}
	generateAgain(t, dir)
}

// gitCommit makes dir a git repository with everything in it committed.
func gitCommit(t *testing.T, dir string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Fatal("git is not installed; replace needs it")
	}
	for _, args := range [][]string{{"init", "-q"}, {"add", "-A"}, {"commit", "-q", "-m", "initial"}} {
		all := append([]string{"-C", dir, "-c", "user.name=Test", "-c", "user.email=test@example.com", "-c", "commit.gpgsign=false"}, args...)
		if out, err := exec.Command("git", all...).CombinedOutput(); err != nil { //nolint:gosec // test code runs git with fixed arguments
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
}

func TestGenerateExistingReplace(t *testing.T) {
	dir := newProject(t, map[string]string{"cyanotype.yaml": claudeSpec, "CLAUDE.md": handWritten})
	// Keep git from finding a repository above the test folder.
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(dir))

	r := runCLI(t, nonInteractive, failReader{t}, "generate", "--dir", dir, "--existing", "replace", "--yes")
	if r.code != codeError || !strings.Contains(r.stderr, "error: refusing to replace CLAUDE.md: git could not give it back: the project folder is not in a git repository") ||
		!strings.Contains(r.stderr, "--existing keep or --existing move") {
		t.Errorf("outside a repository: exit code %d, stderr:\n%s\nwant a refusal suggesting keep or move", r.code, r.stderr)
	}

	gitCommit(t, dir)
	writeTestFile(t, dir, "CLAUDE.md", handWritten+"- One more rule.\n")
	r = runCLI(t, nonInteractive, failReader{t}, "generate", "--dir", dir, "--existing", "replace", "--yes")
	if r.code != codeError || !strings.Contains(r.stderr, "CLAUDE.md has changes that are not committed") {
		t.Errorf("uncommitted changes: exit code %d, stderr:\n%s\nwant a refusal", r.code, r.stderr)
	}
	if got := tree(t, dir)["CLAUDE.md"]; got != handWritten+"- One more rule.\n" {
		t.Errorf("CLAUDE.md was changed to:\n%s", got)
	}

	writeTestFile(t, dir, "CLAUDE.md", handWritten)
	r = runCLI(t, nonInteractive, failReader{t}, "generate", "--dir", dir, "--existing", "replace", "--yes")
	if r.code != codeOK || r.err != nil {
		t.Fatalf("committed file: exit code %d, error %v\nstderr:\n%s", r.code, r.err, r.stderr)
	}
	if got := tree(t, dir)["CLAUDE.md"]; strings.Contains(got, "never npm") || !strings.HasPrefix(got, "<!-- Generated by cyanotype") {
		t.Errorf("CLAUDE.md was not replaced:\n%s", got)
	}
	if !strings.Contains(r.stdout, "(written by hand: replaced; git show HEAD:CLAUDE.md gives it back)") {
		t.Errorf("summary does not say how to get the file back:\n%s", r.stdout)
	}
}

func TestGenerateExistingImport(t *testing.T) {
	dir := newProject(t, map[string]string{
		"cyanotype.yaml": claudeSpec, "CLAUDE.md": handWritten,
		"package.json": `{"dependencies": {"next": "15"}}`, "pnpm-lock.yaml": "",
	})
	r := runCLI(t, nonInteractive, failReader{t}, "generate", "--dir", dir, "--existing", "import")
	if r.code != codeOK || r.err != nil {
		t.Fatalf("exit code %d, error %v\nstderr:\n%s", r.code, r.err, r.stderr)
	}
	for _, s := range []string{
		"# Convert CLAUDE.md into cyanotype.yaml\n",
		"update cyanotype.yaml, which already exists",
		"## The current cyanotype.yaml\n",
		"- `nextjs` (Next.js): package.json: next\n",
		"- `no-test-tampering`: ",
	} {
		if !strings.Contains(r.stdout, s) {
			t.Errorf("prompt is missing %q", s)
		}
	}
	// The prompt alone is on stdout, so it can be piped to an agent.
	if strings.Contains(r.stdout, "Nothing was written") || !strings.Contains(r.stderr, "Nothing was written.") {
		t.Errorf("what to do with the prompt belongs on stderr:\nstderr:\n%s", r.stderr)
	}
	if files := keys(tree(t, dir)); len(files) != 4 {
		t.Errorf("files = %q; import writes nothing", files)
	}
}

func TestGenerateExistingDryRun(t *testing.T) {
	dir := newProject(t, map[string]string{"cyanotype.yaml": claudeSpec, "CLAUDE.md": handWritten})
	r := runCLI(t, nonInteractive, failReader{t}, "generate", "--dir", dir, "--dry-run")
	if r.code != codeNotWritten {
		t.Fatalf("exit code %d, error %v; want %d", r.code, r.err, codeNotWritten)
	}
	if strings.Contains(r.stdout, "+++ b/CLAUDE.md") || !strings.Contains(r.stdout, "+++ b/.gitignore") {
		t.Errorf("a dry run shows every change except the file waiting for a choice:\n%s", r.stdout)
	}
	for _, s := range []string{
		"modified   CLAUDE.md (written by hand: waiting for a choice)",
		"CLAUDE.md was written by hand: choose what to do with it with --existing import, keep, move or replace.",
	} {
		if !strings.Contains(r.stdout, s) {
			t.Errorf("stdout is missing %q:\n%s", s, r.stdout)
		}
	}
}

// TestGenerateExistingInteractive answers on a terminal: replace is
// refused outside git and offered no more, then keep is chosen.
func TestGenerateExistingInteractive(t *testing.T) {
	dir := newProject(t, map[string]string{"cyanotype.yaml": claudeSpec, "CLAUDE.md": handWritten})
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(dir))
	answers := "4\n2\ny\n" // replace, then keep, then write
	r := runCLI(t, interactive, strings.NewReader(answers), "generate", "--dir", dir)
	if r.code != codeOK || r.err != nil {
		t.Fatalf("exit code %d, error %v\nstderr:\n%s", r.code, r.err, r.stderr)
	}
	for _, s := range []string{
		"CLAUDE.md has no cyanotype header, so it was written by hand. What should cyanotype do with it?",
		"Import it with your coding agent (recommended)",
		"Choose 1-4 [1]",
		"Cannot replace CLAUDE.md: git could not give it back: the project folder is not in a git repository",
		"Choose 1-3 [1]",
		"Write these files? [y/N]",
	} {
		if !strings.Contains(r.stderr, s) {
			t.Errorf("stderr is missing %q:\n%s", s, r.stderr)
		}
	}
	if got := tree(t, dir)["CLAUDE.md"]; !strings.Contains(got, "<!-- cyanotype:custom -->\n"+handWritten) {
		t.Errorf("CLAUDE.md was not kept:\n%s", got)
	}
}

func TestExistingInvalidFlag(t *testing.T) {
	for cmd, files := range map[string]map[string]string{
		"generate": {"cyanotype.yaml": claudeSpec, "CLAUDE.md": handWritten},
		"init":     {"CLAUDE.md": handWritten},
	} {
		dir := newProject(t, files)
		r := runCLI(t, nonInteractive, failReader{t}, cmd, "--dir", dir, "--existing", "overwrite")
		if r.code != codeUsage || r.err == nil || !strings.Contains(r.err.Error(), `--existing "overwrite" is not a choice`) {
			t.Errorf("%s: exit code %d, error %v; want a usage error", cmd, r.code, r.err)
		}
	}
}

// nextWithClaude is an existing Next.js project with a hand-written
// CLAUDE.md and no spec yet.
var nextWithClaude = map[string]string{
	"package.json":   `{"scripts": {"dev": "next dev", "test": "vitest run"}, "dependencies": {"next": "15"}}`,
	"pnpm-lock.yaml": "",
	"CLAUDE.md":      handWritten,
}

func TestInitExistingMissing(t *testing.T) {
	dir := newProject(t, nextWithClaude)
	r := runCLI(t, nonInteractive, failReader{t}, "init", "--dir", dir)
	if r.code != codeUsage || r.err == nil {
		t.Fatalf("exit code %d, error %v; want %d", r.code, r.err, codeUsage)
	}
	for _, s := range []string{"--existing  what to do with CLAUDE.md, written by hand: import (recommended", "--detect "} {
		if !strings.Contains(r.err.Error(), s) {
			t.Errorf("error is missing %q:\n%s", s, r.err)
		}
	}
	assertNoSpec(t, dir)
}

func TestInitExistingImport(t *testing.T) {
	dir := newProject(t, nextWithClaude)
	r := runCLI(t, nonInteractive, failReader{t}, "init", "--dir", dir, "--existing", "import", "--name", "shop")
	if r.code != codeOK || r.err != nil {
		t.Fatalf("exit code %d, error %v\nstderr:\n%s", r.code, r.err, r.stderr)
	}
	for _, s := range []string{
		"# Convert CLAUDE.md into cyanotype.yaml\n",
		"## Draft\n",
		"targets:\n  - claude\n",
		"  name: shop\n  goal: 'TODO: the goal of the project",
		"commands:\n  dev: pnpm dev\n  test: pnpm test\n",
		"selections:\n  - node\n  - pnpm\n  - nextjs\n",
	} {
		if !strings.Contains(r.stdout, s) {
			t.Errorf("prompt is missing %q:\n%s", s, r.stdout)
		}
	}
	if strings.Contains(r.stdout, "Detected in") {
		t.Errorf("import asks no question, so it shows no confirmation:\n%s", r.stdout)
	}
	assertNoSpec(t, dir)
	if files := keys(tree(t, dir)); len(files) != 3 {
		t.Errorf("files = %q; import writes nothing", files)
	}
}

// TestInitExistingImportClash imports with npm and pnpm both detected: the
// draft names no commands, since they depend on the package manager the
// user keeps, and the prompt asks the agent to ask about it.
func TestInitExistingImportClash(t *testing.T) {
	files := maps.Clone(nextWithClaude)
	files["package-lock.json"] = "{}"
	dir := newProject(t, files)
	r := runCLI(t, nonInteractive, failReader{t}, "init", "--dir", dir, "--existing", "import", "--name", "shop")
	if r.code != codeOK || r.err != nil {
		t.Fatalf("exit code %d, error %v\nstderr:\n%s", r.code, r.err, r.stderr)
	}
	_, draft, ok := strings.Cut(r.stdout, "## Draft\n")
	draft, _, _ = strings.Cut(draft, "## The cyanotype.yaml format")
	if !ok {
		t.Fatalf("prompt has no draft:\n%s", r.stdout)
	}
	if strings.Contains(draft, "commands:") {
		t.Errorf("the draft has commands while the package manager is open:\n%s", draft)
	}
	if !strings.Contains(r.stdout, "Only one of `npm` and `pnpm` can be selected. Keep the one the instruction files name; if they name none, ask me.") {
		t.Errorf("prompt does not ask about the clash:\n%s", r.stdout)
	}
	assertNoSpec(t, dir)
}

// TestInitExistingKeepInteractive keeps the file, answers the questions and
// lets init's generate step apply the choice without asking again.
func TestInitExistingKeepInteractive(t *testing.T) {
	dir := newProject(t, nextWithClaude)
	answers := strings.Join([]string{
		"2",          // CLAUDE.md: keep
		"",           // Start from the detected stack: yes
		"typescript", // Known language
		"",           // Autonomy
		"1",          // Translations: no
		"",           // Replies
		"Shop.",      // Goal
		"1",          // Agents: Claude Code
		"y",          // Generate now
		"y",          // Write these files
	}, "\n") + "\n"
	r := runCLI(t, interactive, strings.NewReader(answers), "init", "--dir", dir)
	if r.code != codeOK || r.err != nil {
		t.Fatalf("exit code %d, error %v\nstderr:\n%s", r.code, r.err, r.stderr)
	}
	if strings.Count(r.stderr, "What should cyanotype do with it?") != 1 {
		t.Errorf("the choice should be asked once:\n%s", r.stderr)
	}
	if got := tree(t, dir)["CLAUDE.md"]; !strings.Contains(got, "<!-- cyanotype:custom -->\n"+handWritten) {
		t.Errorf("CLAUDE.md was not kept:\n%s", got)
	}
}

func TestInitExistingFlagsThroughGenerate(t *testing.T) {
	dir := newProject(t, nextWithClaude)
	r := runCLI(t, nonInteractive, failReader{t}, append([]string{"init", "--dir", dir, "--existing", "move", "--detect", "yes", "--i18n", "no"}, answered...)...)
	if r.code != codeOK || r.err != nil {
		t.Fatalf("exit code %d, error %v\nstderr:\n%s", r.code, r.err, r.stderr)
	}
	if !strings.Contains(r.stdout, `Next: run "cyanotype generate --dir `) || !strings.Contains(r.stdout, ` --existing move" to write the instruction files.`) {
		t.Errorf("init does not say to repeat the choice to generate:\n%s", r.stdout)
	}
}

func writeTestFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
