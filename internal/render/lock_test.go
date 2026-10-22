package render

import (
	"maps"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/cyanotype-dev/cyanotype/internal/lock"
	"github.com/cyanotype-dev/cyanotype/internal/resolve"
)

// lockSpec returns a spec targeting claude, with selections and extra
// lines appended.
func lockSpec(selections string, extra ...string) string {
	return "version: 1\ntargets: [claude]\nproject:\n  name: tiny\n  goal: A tiny project for tests.\nselections: [" +
		selections + "]\n" + strings.Join(extra, "")
}

// renderInto renders text against project and returns the files and
// warnings.
func renderInto(t *testing.T, text string, project fstest.MapFS) ([]File, []resolve.Warning) {
	t.Helper()
	files, warnings, err := Render(resolveSpec(t, text), project)
	if err != nil {
		t.Fatalf("Render() error: %v", err)
	}
	return files, warnings
}

// apply does to project what generate.Write does with files.
func apply(project fstest.MapFS, files []File) {
	for _, f := range files {
		if f.Delete {
			delete(project, f.Path)
		} else {
			project[f.Path] = &fstest.MapFile{Data: f.Content}
		}
	}
}

// generated renders text into project and writes the result, as a run of
// cyanotype generate --yes would.
func generated(t *testing.T, text string, project fstest.MapFS) []resolve.Warning {
	t.Helper()
	files, warnings := renderInto(t, text, project)
	apply(project, files)
	return warnings
}

func readLock(t *testing.T, project fstest.MapFS) *lock.Lock {
	t.Helper()
	l, err := lock.Read(project)
	if err != nil || l == nil {
		t.Fatalf("lock.Read() = %v, %v", l, err)
	}
	return l
}

// settingsLists returns the deny and ask lists of the settings file in
// project.
func settingsLists(t *testing.T, project fstest.MapFS) (deny, ask []string) {
	t.Helper()
	f, ok := project[settingsPath]
	if !ok {
		t.Fatal("no .claude/settings.json")
	}
	deny, ask, err := permissionLists(f.Data)
	if err != nil {
		t.Fatal(err)
	}
	return deny, ask
}

func TestLockRecordsWhatWasGenerated(t *testing.T) {
	project := fstest.MapFS{}
	generated(t, lockSpec("secrets-in-env"), project)
	l := readLock(t, project)

	if want := loadCatalog(t).Version(); l.CatalogVersion != want {
		t.Errorf("catalog_version = %q, want %q", l.CatalogVersion, want)
	}
	if got := slices.Sorted(maps.Keys(l.Entries)); !slices.Equal(got, []string{"secrets-in-env"}) {
		t.Errorf("entries = %q", got)
	}
	if got := slices.Sorted(maps.Keys(l.Files)); !slices.Equal(got, []string{".gitignore", "CLAUDE.md"}) {
		t.Errorf("files = %q", got)
	}
	for p, h := range l.Files {
		owned, _, err := Owned(p, project[p].Data)
		if err != nil || h != lock.Hash(owned) {
			t.Errorf("%s recorded as %s, want the hash of its owned content (error %v)", p, h, err)
		}
	}
	deny, _ := settingsLists(t, project)
	if !slices.Equal(l.Claude.Deny, slices.Sorted(slices.Values(deny))) {
		t.Errorf("lock deny = %q, want every rule of the new file %q", l.Claude.Deny, deny)
	}

	// Regenerating an unchanged project changes nothing, the lock included.
	files, _ := renderInto(t, lockSpec("secrets-in-env"), project)
	for _, f := range files {
		if f.Delete || string(project[f.Path].Data) != string(f.Content) {
			t.Errorf("regenerating changed %s", f.Path)
		}
	}
}

// TestLockEntryHashIgnoresAnswers checks that the lock hashes entries as
// the catalog writes them: a change to the spec that fills a placeholder
// differently leaves the entry hash alone.
func TestLockEntryHashIgnoresAnswers(t *testing.T) {
	hash := func(languages string) string {
		project := fstest.MapFS{}
		generated(t, lockSpec(languages+"learning-guided"), project)
		return readLock(t, project).Entries["learning-guided"]
	}
	if a, b := hash("go, "), hash("go, typescript, "); a == "" || a != b {
		t.Errorf("learning-guided hashes %q with Go and %q with Go and TypeScript; want the same", a, b)
	}
}

func TestDeselectRemovesOwnedRules(t *testing.T) {
	project := fstest.MapFS{}
	generated(t, lockSpec("no-destructive-commands, secrets-in-env"), project)
	// A rule the user adds after cyanotype's stays whatever happens.
	deny, _ := settingsLists(t, project)
	project[settingsPath] = &fstest.MapFile{Data: []byte(`{"permissions": {"deny": ` +
		`["` + strings.Join(append(deny, "Bash(curl *)"), `", "`) + `"]}}`)}

	files, _ := renderInto(t, lockSpec("secrets-in-env"), project)
	apply(project, files)
	deny, _ = settingsLists(t, project)
	for _, rule := range deny {
		if strings.HasPrefix(rule, "Bash(rm ") || strings.HasPrefix(rule, "Bash(git ") || strings.HasPrefix(rule, "PowerShell(") {
			t.Errorf("rule %q of the deselected entry is still in the file: %q", rule, deny)
		}
	}
	for _, want := range []string{"Bash(curl *)", "Read(.env)", "Edit(!.env.example)"} {
		if !slices.Contains(deny, want) {
			t.Errorf("rule %q was removed: %q", want, deny)
		}
	}
	if l := readLock(t, project); slices.Contains(l.Claude.Deny, "Bash(rm -rf *)") || slices.Contains(l.Claude.Deny, "Bash(curl *)") {
		t.Errorf("lock deny = %q", l.Claude.Deny)
	}
}

// TestUserRuleBeforeCyanotypeStays covers a rule the user wrote before
// cyanotype generated the same one: it is never cyanotype's, so
// deselecting the entry keeps it.
func TestUserRuleBeforeCyanotypeStays(t *testing.T) {
	for _, withLock := range []bool{false, true} {
		project := fstest.MapFS{settingsPath: {Data: []byte(`{"permissions": {"deny": ["Bash(rm -rf *)"]}}` + "\n")}}
		if withLock {
			project[lock.Path] = &fstest.MapFile{Data: (&lock.Lock{}).Marshal()}
		}
		generated(t, lockSpec("no-destructive-commands"), project)
		if l := readLock(t, project); slices.Contains(l.Claude.Deny, "Bash(rm -rf *)") || !slices.Contains(l.Claude.Deny, "Bash(rm -fr *)") {
			t.Errorf("lock %v: deny = %q, want every rule but the user's", withLock, l.Claude.Deny)
		}
		generated(t, lockSpec(""), project)
		if deny, _ := settingsLists(t, project); !slices.Equal(deny, []string{"Bash(rm -rf *)"}) {
			t.Errorf("lock %v: after deselecting, deny = %q, want only the user's rule", withLock, deny)
		}
	}
}

// TestDenyMovedToAsk is a rule a newer catalog moved from deny to ask: the
// old deny rule cyanotype owns goes, so the ask rule can fire.
func TestDenyMovedToAsk(t *testing.T) {
	project := fstest.MapFS{
		settingsPath: {Data: []byte(`{"permissions": {"deny": ["Bash(curl *)", "Edit(/cyanotype.yaml)"]}}` + "\n")},
		lock.Path:    {Data: (&lock.Lock{Claude: lock.Rules{Deny: []string{"Edit(/cyanotype.yaml)"}}}).Marshal()},
	}
	generated(t, lockSpec("rule-maintenance-propose"), project)
	deny, ask := settingsLists(t, project)
	if slices.Contains(deny, "Edit(/cyanotype.yaml)") || !slices.Contains(deny, "Bash(curl *)") {
		t.Errorf("deny = %q", deny)
	}
	if !slices.Equal(ask, []string{"Edit(/cyanotype.yaml)"}) {
		t.Errorf("ask = %q", ask)
	}
	if l := readLock(t, project); !slices.Equal(l.Claude.Ask, []string{"Edit(/cyanotype.yaml)"}) || slices.Contains(l.Claude.Deny, "Edit(/cyanotype.yaml)") {
		t.Errorf("lock = %+v", l.Claude)
	}
}

// TestClaudeTargetRemoved drops the claude target: the rules cyanotype
// added go, and the lists they leave empty go with them.
func TestClaudeTargetRemoved(t *testing.T) {
	project := fstest.MapFS{}
	generated(t, lockSpec("secrets-in-env"), project)
	generated(t, strings.Replace(lockSpec("secrets-in-env"), "[claude]", "[agents-md]", 1), project)

	want := "{\n  \"$schema\": \"" + settingsSchema + "\"\n}\n"
	if got := string(project[settingsPath].Data); got != want {
		t.Errorf("settings.json =\n%s\nwant:\n%s", got, want)
	}
	if l := readLock(t, project); !l.Claude.Empty() {
		t.Errorf("lock still owns %+v", l.Claude)
	}
}

func TestStaleFiles(t *testing.T) {
	split := lockSpec("go, plan-before-code, conventional-commits", "output:\n  split: always\n")
	whole := lockSpec("go, plan-before-code, conventional-commits")
	const stale = "docs/agents/git.md"

	tests := []struct {
		name string
		// edit changes the stale file before regenerating; nil leaves it.
		edit       func(old []byte) []byte
		wantDelete bool
		wantLock   bool
		wantWarn   string
	}{
		{name: "unchanged", wantDelete: true},
		{name: "CRLF checkout", edit: func(b []byte) []byte { return []byte(strings.ReplaceAll(string(b), "\n", "\r\n")) }, wantDelete: true},
		{name: "edited", edit: func(b []byte) []byte { return append(b, "- My own rule.\n"...) },
			wantLock: true, wantWarn: "it changed since cyanotype generated it"},
		{name: "custom block", edit: func(b []byte) []byte {
			return append(b, "\n<!-- cyanotype:custom -->\nMine.\n<!-- /cyanotype:custom -->\n"...)
		}, wantLock: true, wantWarn: "it holds a cyanotype:custom block"},
		{name: "broken markers", edit: func(b []byte) []byte { return append(b, "<!-- cyanotype:custom -->\n"...) },
			wantLock: true, wantWarn: "markers do not match"},
		{name: "header removed", edit: func([]byte) []byte { return []byte("# Git workflow\n- Mine now.\n") }},
		{name: "deleted by hand", edit: func([]byte) []byte { return nil }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			project := fstest.MapFS{}
			generated(t, split, project)
			if _, ok := project[stale]; !ok {
				t.Fatalf("the split output has no %s", stale)
			}
			if tt.edit != nil {
				if data := tt.edit(project[stale].Data); data == nil {
					delete(project, stale)
				} else {
					project[stale] = &fstest.MapFile{Data: data}
				}
			}

			files, warnings := renderInto(t, whole, project)
			deleted := slices.ContainsFunc(files, func(f File) bool { return f.Delete && f.Path == stale })
			if deleted != tt.wantDelete {
				t.Errorf("deleted = %v, want %v", deleted, tt.wantDelete)
			}
			var warned []string
			for _, w := range warnings {
				if w.File == stale {
					warned = append(warned, w.Msg)
				}
			}
			switch {
			case tt.wantWarn == "" && len(warned) > 0:
				t.Errorf("unexpected warnings %q", warned)
			case tt.wantWarn != "" && (len(warned) != 1 || !strings.Contains(warned[0], tt.wantWarn)):
				t.Errorf("warnings = %q, want one containing %q", warned, tt.wantWarn)
			}
			apply(project, files)
			if _, recorded := readLock(t, project).Files[stale]; recorded != tt.wantLock {
				t.Errorf("lock records %s: %v, want %v", stale, recorded, tt.wantLock)
			}
		})
	}
}

// TestUnrecordedHeaderFile is a topic file left by a split made before
// v0.3: it has the header, but no lock records it, so it is only reported.
func TestUnrecordedHeaderFile(t *testing.T) {
	project := fstest.MapFS{
		"docs/agents/git.md":               {Data: []byte(strings.Join(header, "\n") + "\n\n# Git workflow\n")},
		"docs/agents/original-claude.md":   {Data: []byte("# Rules written before cyanotype\n")},
		"docs/agents/notes.txt":            {Data: []byte(strings.Join(header, "\n") + "\n")},
		"docs/agents/sub/deeper.md":        {Data: []byte(strings.Join(header, "\n") + "\n")},
		"docs/agents/decisions-by-hand.md": {Data: []byte("# Decisions\n")},
	}
	files, warnings := renderInto(t, lockSpec("go"), project)
	if slices.ContainsFunc(files, func(f File) bool { return f.Delete }) {
		t.Error("an unrecorded file was deleted")
	}
	var warned []string
	for _, w := range warnings {
		warned = append(warned, w.String())
	}
	want := []string{"docs/agents/git.md: has the cyanotype header but is not generated, and cyanotype.lock does not record it, " +
		"so cyanotype cannot tell whether someone edited it; delete it if it is no longer needed"}
	if !slices.Equal(warned, want) {
		t.Errorf("warnings = %q\nwant %q", warned, want)
	}
}

// TestUnownedRulesWarning is the first generation of a project an older
// cyanotype generated: no rule is taken as cyanotype's, and the warning
// names those that come from entries no longer selected.
func TestUnownedRulesWarning(t *testing.T) {
	project := fstest.MapFS{settingsPath: {Data: []byte(`{"permissions": {"deny": ` +
		`["Read(.env)", "Bash(pnpm *)", "Bash(curl *)"], "ask": ["Bash(npm i *)"]}}` + "\n")}}
	_, warnings := renderInto(t, lockSpec("secrets-in-env"), project)
	if len(warnings) != 1 || warnings[0].File != settingsPath {
		t.Fatalf("warnings = %v, want one about %s", warnings, settingsPath)
	}
	msg := warnings[0].Msg
	for _, want := range []string{
		"cyanotype treats the rules already in this file as yours",
		"1 match rules it generates",
		`2 come from catalog entries that are not selected ("Bash(pnpm *)", "Bash(npm i *)")`,
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("warning is missing %q:\n%s", want, msg)
		}
	}
	if strings.Contains(msg, "curl") {
		t.Errorf("warning names a rule no catalog entry has:\n%s", msg)
	}

	// Once the lock exists, the warning is gone.
	generated(t, lockSpec("secrets-in-env"), project)
	if _, warnings := renderInto(t, lockSpec("secrets-in-env"), project); len(warnings) != 0 {
		t.Errorf("warnings after the first generation = %v", warnings)
	}
}

func TestLockFromNewerCatalog(t *testing.T) {
	project := fstest.MapFS{lock.Path: {Data: (&lock.Lock{CatalogVersion: "2099.01"}).Marshal()}}
	_, _, err := Render(resolveSpec(t, lockSpec("go")), project)
	if err == nil || !strings.Contains(err.Error(), "generated with catalog 2099.01, newer than this cyanotype's catalog") {
		t.Errorf("Render() error = %v, want a refusal", err)
	}
}

func TestInvalidLock(t *testing.T) {
	project := fstest.MapFS{lock.Path: {Data: []byte("version: 1\nfiles: [oops]\n")}}
	_, _, err := Render(resolveSpec(t, lockSpec("go")), project)
	if err == nil || !strings.Contains(err.Error(), "cyanotype.lock is invalid") {
		t.Errorf("Render() error = %v, want the lock's error", err)
	}
}

// TestOwned checks the normalization behind every hash: CRLF endings, a
// byte order mark, protected blocks anywhere and the blank lines around
// them do not count; any other change does.
func TestOwned(t *testing.T) {
	const generatedFile = "<!-- Generated by cyanotype -->\n\n# tiny\n\n## Testing\n\n- Rule one.\n\n## Git workflow\n\n- Rule two.\n"
	base, ok, err := Owned("AGENTS.md", []byte(generatedFile))
	if err != nil || !ok {
		t.Fatalf("Owned() = %v, %v", ok, err)
	}
	same := map[string]string{
		"CRLF":        strings.ReplaceAll(generatedFile, "\n", "\r\n"),
		"BOM":         "\xef\xbb\xbf" + generatedFile,
		"block":       strings.Replace(generatedFile, "- Rule one.\n", "- Rule one.\n\n<!-- cyanotype:custom -->\nMine.\n\n## Mine\n<!-- /cyanotype:custom -->\n", 1),
		"tight block": strings.Replace(generatedFile, "- Rule one.\n", "- Rule one.\n<!-- cyanotype:custom -->\nMine.\n<!-- /cyanotype:custom -->\n", 1),
		"extra blank": strings.Replace(generatedFile, "\n\n## Git", "\n\n\n\n## Git", 1) + "\n\n",
	}
	for name, content := range same {
		got, _, err := Owned("AGENTS.md", []byte(content))
		if err != nil || string(got) != string(base) {
			t.Errorf("%s: Owned() =\n%s\nwant:\n%s\n(error %v)", name, got, base, err)
		}
	}
	for name, content := range map[string]string{
		"edited rule": strings.Replace(generatedFile, "Rule one", "Rule 1", 1),
		"added line":  generatedFile + "- Rule three.\n",
	} {
		if got, _, _ := Owned("AGENTS.md", []byte(content)); string(got) == string(base) {
			t.Errorf("%s: Owned() did not change", name)
		}
	}

	if _, _, err := Owned("AGENTS.md", []byte("<!-- cyanotype:custom -->\n")); err == nil {
		t.Error("Owned() accepted a block that is never closed")
	}
	for _, p := range []string{settingsPath, lock.Path} {
		if _, ok, err := Owned(p, []byte("{}")); ok || err != nil {
			t.Errorf("Owned(%s) = %v, %v; want no owned content", p, ok, err)
		}
	}
	gitignore := "node_modules/\r\n" + gitignoreBeginLine + "\r\n.env\r\n" + gitignoreEnd + "\r\nmine/\r\n"
	if got, ok, err := Owned(gitignorePath, []byte(gitignore)); err != nil || !ok || string(got) != gitignoreBeginLine+"\n.env\n"+gitignoreEnd+"\n" {
		t.Errorf("Owned(.gitignore) = %q, %v, %v; want the block only, with LF", got, ok, err)
	}
	if _, ok, err := Owned(gitignorePath, []byte("mine/\n")); ok || err != nil {
		t.Errorf("Owned(.gitignore without a block) = %v, %v", ok, err)
	}
}
