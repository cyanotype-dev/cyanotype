package main

import (
	"slices"
	"strings"
	"testing"
)

// nextProject is an existing Next.js project with two lockfiles, which
// detection cannot choose between.
var nextProject = map[string]string{
	"package.json": `{"name": "shop",
		"scripts": {"dev": "next dev", "build": "next build", "lint": "next lint", "start": "next start"},
		"dependencies": {"next": "15", "react": "19"},
		"devDependencies": {"typescript": "5", "tailwindcss": "4"}}`,
	"pnpm-lock.yaml":    "lockfileVersion: '9.0'\n",
	"package-lock.json": "{}\n",
}

// answered are the flags that answer every question detection leaves.
var answered = []string{"--autonomy", "balanced", "--goal", "An online shop.", "--agents", "claude", "--name", "shop"}

func TestInitDetectNeedsConfirmation(t *testing.T) {
	dir := newProject(t, nextProject)
	r := runCLI(t, nonInteractive, failReader{t}, "init", "--dir", dir)
	if r.code != codeUsage || r.err == nil {
		t.Fatalf("exit code %d, error %v; want %d and an error", r.code, r.err, codeUsage)
	}
	// Only --detect is missing: the other questions depend on its answer.
	msg := r.err.Error()
	if !strings.Contains(msg, "--detect  yes to start from the stack detected above") || strings.Contains(msg, "--type") {
		t.Errorf("error should ask for --detect only:\n%s", msg)
	}
	for _, s := range []string{
		"Detected in " + dir + ":\n",
		"  - Next.js (package.json: next)\n",
		"  - Node.js (package.json)\n",
		"  - npm (package-lock.json)\n",
		"  - pnpm (pnpm-lock.yaml)\n",
		"Only one of npm and pnpm can be selected",
		"Scripts in package.json: build, dev, lint, start\n",
	} {
		if !strings.Contains(r.stdout, s) {
			t.Errorf("stdout is missing %q:\n%s", s, r.stdout)
		}
	}
	assertNoSpec(t, dir)
}

func TestInitDetectFlags(t *testing.T) {
	dir := newProject(t, nextProject)

	// The clash between the lockfiles is a missing answer, listed with the
	// others.
	r := runCLI(t, nonInteractive, failReader{t}, append([]string{"init", "--dir", dir, "--detect", "yes"}, answered...)...)
	if r.code != codeUsage || r.err == nil {
		t.Fatalf("exit code %d, error %v; want %d", r.code, r.err, codeUsage)
	}
	for _, s := range []string{
		"--with  npm or pnpm: the one the project uses (npm found package-lock.json, pnpm found pnpm-lock.yaml; only one can be selected)",
		"--i18n ",
	} {
		if !strings.Contains(r.err.Error(), s) {
			t.Errorf("error is missing %q:\n%s", s, r.err)
		}
	}
	assertNoSpec(t, dir)

	r = runCLI(t, nonInteractive, failReader{t}, append([]string{"init", "--dir", dir, "--detect", "yes", "--with", "pnpm", "--i18n", "no"}, answered...)...)
	if r.code != codeOK || r.err != nil {
		t.Fatalf("exit code %d, error %v\nstderr:\n%s", r.code, r.err, r.stderr)
	}
	got := readSpec(t, dir)
	want := `commands:
  dev: pnpm dev
  build: pnpm build
  lint: pnpm lint

selections:
  - typescript
  - node
  - pnpm
  - nextjs
  - tailwind
  - conventional-commits
`
	if !strings.Contains(got, want) {
		t.Errorf("cyanotype.yaml:\n%s\nwant it to contain:\n%s", got, want)
	}
	// Nothing the project does not use: no npm, and none of the preset's
	// Supabase, shadcn/ui or Vitest.
	for _, s := range []string{"  - npm\n", "supabase", "shadcn-ui", "vitest"} {
		if strings.Contains(got, s) {
			t.Errorf("cyanotype.yaml has %q:\n%s", s, got)
		}
	}
	for _, s := range []string{
		"Starting from: the stack detected in the project\nProject type: Website\n",
		"  - Next.js: React framework for full-stack web apps (found package.json: next)\n",
		"Commands (from package.json scripts)\n  - dev: pnpm dev\n  - build: pnpm build\n  - lint: pnpm lint\n",
	} {
		if !strings.Contains(r.stdout, s) {
			t.Errorf("stdout is missing %q:\n%s", s, r.stdout)
		}
	}
	if strings.Contains(r.stdout, "Starter:") {
		t.Errorf("a detected stack is shown as a starter:\n%s", r.stdout)
	}
}

func TestInitDetectNo(t *testing.T) {
	dir := newProject(t, nextProject)
	// --detect no is simple mode as in an empty folder: two website
	// presets fit, so the starter is missing.
	r := runCLI(t, nonInteractive, failReader{t}, append([]string{"init", "--dir", dir, "--detect", "no",
		"--type", "website", "--language", "typescript", "--i18n", "no"}, answered...)...)
	if r.code != codeUsage || r.err == nil || !strings.Contains(r.err.Error(), "--preset ") {
		t.Fatalf("exit code %d, error %v; want a usage error asking for --preset", r.code, r.err)
	}
	if strings.Contains(r.stdout, "Detected in") {
		t.Errorf("--detect no still shows the detected stack:\n%s", r.stdout)
	}

	// A preset alone ignores detection and starts from the starter.
	r = runCLI(t, nonInteractive, failReader{t}, append([]string{"init", "--dir", dir, "--preset", "vite-react-app", "--i18n", "no"}, answered...)...)
	if r.code != codeOK || r.err != nil {
		t.Fatalf("exit code %d, error %v\nstderr:\n%s", r.code, r.err, r.stderr)
	}
	if strings.Contains(r.stdout, "Detected in") || !strings.Contains(r.stdout, "Starter: Vite + React\n") {
		t.Errorf("--preset should start from the starter without detecting:\n%s", r.stdout)
	}
	if got := readSpec(t, dir); !strings.Contains(got, "  - vite-react\n") || strings.Contains(got, "  - nextjs\n") {
		t.Errorf("cyanotype.yaml is not the preset's:\n%s", got)
	}
}

func TestInitDetectInvalidFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"unknown value", []string{"--detect", "maybe"}, `--detect "maybe": must be yes or no`},
		{"preset and detect yes", []string{"--preset", "go-cli", "--detect", "yes"}, "drop --detect yes or --preset"},
		{"preset and with", []string{"--preset", "go-cli", "--with", "pnpm"}, "which --preset does not use"},
		{"detect no and without", []string{"--detect", "no", "--without", "npm"}, "which --detect no ignores"},
		{"unknown entry", []string{"--detect", "yes", "--with", "pnpn"}, `with: unknown entry "pnpn"; did you mean "pnpm"?`},
		{"removing what was not detected", []string{"--detect", "yes", "--without", "expo"}, `without: "expo" was not detected`},
		{"two package managers", []string{"--detect", "yes", "--with", "npm,pnpm"}, `"npm" and "pnpm" are both in radio category "package-manager"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := newProject(t, nextProject)
			r := runCLI(t, nonInteractive, failReader{t}, append([]string{"init", "--dir", dir}, tt.args...)...)
			if r.code != codeUsage || r.err == nil || !strings.Contains(r.err.Error(), tt.want) {
				t.Errorf("exit code %d, error %v; want %d and an error containing %q", r.code, r.err, codeUsage, tt.want)
			}
			assertNoSpec(t, dir)
		})
	}
}

// TestInitDetectTranslated checks that a detected translation library
// skips the translation question and leaves only the locales to give.
func TestInitDetectTranslated(t *testing.T) {
	dir := newProject(t, map[string]string{
		"package.json":   `{"dependencies": {"next": "15", "next-intl": "4"}}`,
		"pnpm-lock.yaml": "",
	})
	r := runCLI(t, nonInteractive, failReader{t}, append([]string{"init", "--dir", dir, "--detect", "yes"}, answered...)...)
	if r.code != codeUsage || r.err == nil {
		t.Fatalf("exit code %d, error %v; want %d", r.code, r.err, codeUsage)
	}
	if msg := r.err.Error(); !strings.Contains(msg, "--locales ") || strings.Contains(msg, "--i18n ") {
		t.Errorf("error should ask for --locales only:\n%s", msg)
	}

	r = runCLI(t, nonInteractive, failReader{t}, append([]string{"init", "--dir", dir, "--detect", "yes", "--locales", "it,en"}, answered...)...)
	if r.code != codeOK || r.err != nil {
		t.Fatalf("exit code %d, error %v\nstderr:\n%s", r.code, r.err, r.stderr)
	}
	got := readSpec(t, dir)
	for _, s := range []string{"  - i18n-next-intl\n  - i18n-rules\n", "  default_locale: it\n  locales:\n    - it\n    - en\n"} {
		if !strings.Contains(got, s) {
			t.Errorf("cyanotype.yaml is missing %q:\n%s", s, got)
		}
	}
	if strings.Count(got, "i18n-next-intl") != 1 {
		t.Errorf("i18n-next-intl is listed more than once:\n%s", got)
	}
}

// TestInitDetectInteractive answers the questions of a detected stack on a
// terminal: it removes a false positive, picks the package manager detection
// could not find, and skips what to build and the starter.
func TestInitDetectInteractive(t *testing.T) {
	dir := newProject(t, map[string]string{
		"package.json": `{"dependencies": {"next": "15"}, "devDependencies": {"typescript": "5"}, "scripts": {"test": "vitest"}}`,
		// Expo's config file, which this Next.js project has for another reason.
		"app.json": "{}",
	})
	answers := strings.Join([]string{
		"2",          // Start from the detected stack: yes, without some of it
		"1",          // Which does the project not use: Expo
		"",           // Package manager: the default, pnpm
		"typescript", // Known language
		"",           // Autonomy: balanced
		"1",          // Translations: no
		"",           // Replies: English
		"Shop.",      // Goal
		"1",          // Agents: Claude Code
		"n",          // Generate now: no
	}, "\n") + "\n"
	r := runCLI(t, interactive, strings.NewReader(answers), "init", "--dir", dir)
	if r.code != codeOK || r.err != nil {
		t.Fatalf("exit code %d, error %v\nstderr:\n%s", r.code, r.err, r.stderr)
	}
	for _, s := range []string{
		"Start from the detected stack?",
		"Which of these does the project not use?",
		"Node.js needs npm or pnpm, and detection found neither. Which one does the project use?",
		"Choose 1-2 [2]",
	} {
		if !strings.Contains(r.stderr, s) {
			t.Errorf("stderr is missing %q:\n%s", s, r.stderr)
		}
	}
	for _, s := range []string{"What do you want to build?", "Which starter fits best?", "learning mode"} {
		if strings.Contains(r.stderr, s) {
			t.Errorf("stderr asks %q, which detection answered:\n%s", s, r.stderr)
		}
	}
	got := readSpec(t, dir)
	if !strings.Contains(got, "commands:\n  test: pnpm test\n") ||
		!strings.Contains(got, "selections:\n  - typescript\n  - node\n  - pnpm\n  - nextjs\n  - conventional-commits\n") ||
		strings.Contains(got, "expo") {
		t.Errorf("cyanotype.yaml:\n%s", got)
	}
	if !strings.Contains(r.stdout, "  - pnpm: Fast, disk-efficient package manager for Node.js (your choice)\n") {
		t.Errorf("summary does not say pnpm was chosen:\n%s", r.stdout)
	}
	if files := keys(tree(t, dir)); !slices.Equal(files, []string{"app.json", "cyanotype.yaml", "package.json"}) {
		t.Errorf("files = %q; detection must not write anything", files)
	}
}

// TestInitDetectAsksType checks that a stack whose closest presets build
// different things asks what to build, then skips the starter.
func TestInitDetectAsksType(t *testing.T) {
	dir := newProject(t, map[string]string{"go.mod": "module example.com/tool\n\ngo 1.27\n"})
	r := runCLI(t, nonInteractive, failReader{t}, append([]string{"init", "--dir", dir, "--detect", "yes"}, answered...)...)
	if r.code != codeUsage || r.err == nil || !strings.Contains(r.err.Error(), "--type ") || strings.Contains(r.err.Error(), "--preset") {
		t.Fatalf("exit code %d, error %v; want a usage error asking for --type only", r.code, r.err)
	}

	r = runCLI(t, nonInteractive, failReader{t}, append([]string{"init", "--dir", dir, "--detect", "yes", "--type", "script"}, answered...)...)
	if r.code != codeOK || r.err != nil {
		t.Fatalf("exit code %d, error %v\nstderr:\n%s", r.code, r.err, r.stderr)
	}
	got := readSpec(t, dir)
	if !strings.Contains(got, "selections:\n  - go\n  - conventional-commits\n") || strings.Contains(got, "golangci-lint") || strings.Contains(got, "commands:") {
		t.Errorf("cyanotype.yaml:\n%s", got)
	}
	if !strings.Contains(r.stdout, "Commands: none detected; add them under commands in cyanotype.yaml.\n") {
		t.Errorf("summary does not say there are no commands:\n%s", r.stdout)
	}
}
