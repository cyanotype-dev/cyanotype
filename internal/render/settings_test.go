package render

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/cyanotype-dev/cyanotype/internal/catalog"
	"github.com/cyanotype-dev/cyanotype/internal/resolve"
	"github.com/cyanotype-dev/cyanotype/internal/spec"
)

// secretsRules are the deny rules of secrets-in-env, whose negations only
// narrow the rules listed before them.
var secretsRules = []string{
	"Read(.env)", "Read(.env.*)", "Read(!.env.example)",
	"Edit(.env)", "Edit(.env.*)", "Edit(!.env.example)",
}

// isSubsequence reports whether sub appears in list in the same order,
// not necessarily next to each other.
func isSubsequence(sub, list []string) bool {
	i := 0
	for _, s := range list {
		if i < len(sub) && s == sub[i] {
			i++
		}
	}
	return i == len(sub)
}

// checkDenyOrder fails the test unless every group appears in deny in its
// own order: the property that keeps negated rules after the rules they
// narrow.
func checkDenyOrder(t *testing.T, deny []string, groups [][]string) {
	t.Helper()
	for _, g := range groups {
		if !isSubsequence(g, deny) {
			t.Errorf("deny list breaks the order of %q:\n%q", g, deny)
		}
	}
}

func TestMergeOrderedKeepsEntryOrder(t *testing.T) {
	tests := []struct {
		name     string
		existing []string
		groups   [][]string
		want     []string
	}{
		{
			name:   "empty file",
			groups: [][]string{secretsRules},
			want:   secretsRules,
		},
		{
			name:     "already merged",
			existing: secretsRules,
			groups:   [][]string{secretsRules},
			want:     secretsRules,
		},
		{
			name:     "negation written before the rules it narrows is added again after them",
			existing: []string{"Read(!.env.example)", "Read(.env)"},
			groups:   [][]string{secretsRules},
			want: []string{
				"Read(!.env.example)", "Read(.env)",
				"Read(.env.*)", "Read(!.env.example)", "Edit(.env)", "Edit(.env.*)", "Edit(!.env.example)",
			},
		},
		{
			name:     "rules in reverse order",
			existing: []string{"Read(.env.*)", "Read(.env)"},
			groups:   [][]string{{"Read(.env)", "Read(.env.*)", "Read(!.env.example)"}},
			want:     []string{"Read(.env.*)", "Read(.env)", "Read(.env.*)", "Read(!.env.example)"},
		},
		{
			name:     "user rules in between are kept where they are",
			existing: []string{"Read(.env)", "Bash(curl:*)"},
			groups:   [][]string{{"Read(.env)", "Read(!.env.example)"}},
			want:     []string{"Read(.env)", "Bash(curl:*)", "Read(!.env.example)"},
		},
		{
			name:   "rule shared by two entries is written once",
			groups: [][]string{{"Bash(yarn:*)", "Bash(npm install:*)"}, {"Bash(yarn:*)", "Bash(pip install *)"}},
			want:   []string{"Bash(yarn:*)", "Bash(npm install:*)", "Bash(pip install *)"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeOrdered(tt.existing, tt.groups)
			if !slices.Equal(got, tt.want) {
				t.Errorf("mergeOrdered() =\n%q\nwant\n%q", got, tt.want)
			}
			checkDenyOrder(t, got, tt.groups)
			if !slices.Equal(got[:len(tt.existing)], tt.existing) {
				t.Errorf("mergeOrdered() changed the existing rules: %q", got)
			}
			if again := mergeOrdered(got, tt.groups); !slices.Equal(again, got) {
				t.Errorf("merging twice changed the list:\n%q\nthen\n%q", got, again)
			}
		})
	}
}

// TestRealCatalogPermissionOrder renders every entry of the real catalog,
// over a settings file holding each entry's rules in reverse, and checks
// that every entry's rules still come out in catalog order. It fails if a
// change to merging, or a new entry, breaks the order a negation depends
// on.
func TestRealCatalogPermissionOrder(t *testing.T) {
	c, err := catalog.LoadRoot(os.DirFS(filepath.Join("..", "..", "catalog")))
	if err != nil {
		t.Fatal(err)
	}
	// Every entry is added directly, without resolving, so exclusive
	// entries such as npm and pnpm are checked together and every key of
	// enforce.claude.with is selected.
	r := &resolve.Result{Spec: spec.Spec{Targets: []spec.Target{spec.TargetClaude}}.WithDefaults()}
	for _, e := range c.Entries() {
		r.Entries = append(r.Entries, resolve.Entry{Entry: e})
	}
	rules := claudeRules(r)
	if len(rules.deny) < 2 || len(rules.ask) < 2 {
		t.Fatalf("found %d catalog entries with deny rules and %d with ask rules, want several of each", len(rules.deny), len(rules.ask))
	}
	reversed := func(groups [][]string) []string {
		var out []string
		for _, g := range groups {
			g = slices.Clone(g)
			slices.Reverse(g)
			out = append(out, g...)
		}
		return out
	}
	deny, ask := reversed(rules.deny), reversed(rules.ask)

	old, err := json.Marshal(map[string]any{"permissions": map[string]any{"deny": deny, "ask": ask}})
	if err != nil {
		t.Fatal(err)
	}
	f, ok, err := renderSettings(r, fstest.MapFS{settingsPath: &fstest.MapFile{Data: old}})
	if err != nil || !ok {
		t.Fatalf("renderSettings() = %v, %v", ok, err)
	}
	var got struct {
		Permissions struct{ Deny, Ask []string }
	}
	if err := json.Unmarshal(f.Content, &got); err != nil {
		t.Fatal(err)
	}
	checkDenyOrder(t, got.Permissions.Deny, rules.deny)
	checkDenyOrder(t, got.Permissions.Ask, rules.ask)
}

func TestClaudeRulesWith(t *testing.T) {
	entry := func(id string, c *catalog.ClaudeEnforce) resolve.Entry {
		e := catalog.Entry{ID: id}
		if c != nil {
			e.Enforce = &catalog.Enforce{Claude: c}
		}
		return resolve.Entry{Entry: e}
	}
	pnpm := entry("pnpm", &catalog.ClaudeEnforce{
		Deny: []string{"Bash(npm *)"},
		With: map[string]catalog.ClaudeConditional{
			"dependency-approval": {Ask: []string{"Bash(pnpm add *)", "PowerShell(pnpm add *)"}},
			"audit-rules":         {Ask: []string{"Bash(pnpm audit *)"}},
		},
	})
	tests := []struct {
		name     string
		entries  []resolve.Entry
		wantDeny [][]string
		wantAsk  [][]string
	}{
		{
			name:     "key not selected adds nothing",
			entries:  []resolve.Entry{pnpm},
			wantDeny: [][]string{{"Bash(npm *)"}},
		},
		{
			name:     "key selected adds its rules to the entry's group",
			entries:  []resolve.Entry{pnpm, entry("dependency-approval", nil)},
			wantDeny: [][]string{{"Bash(npm *)"}},
			wantAsk:  [][]string{{"Bash(pnpm add *)", "PowerShell(pnpm add *)"}},
		},
		{
			name:     "selected before the entry that carries the rules",
			entries:  []resolve.Entry{entry("dependency-approval", nil), pnpm},
			wantDeny: [][]string{{"Bash(npm *)"}},
			wantAsk:  [][]string{{"Bash(pnpm add *)", "PowerShell(pnpm add *)"}},
		},
		{
			name:     "plain ask rules first, then keys in alphabetical order",
			entries:  []resolve.Entry{entry("dependency-approval", nil), entry("audit-rules", nil), pnpm, entry("spec-guard", &catalog.ClaudeEnforce{Ask: []string{"Edit(/spec.yaml)"}})},
			wantDeny: [][]string{{"Bash(npm *)"}},
			wantAsk: [][]string{
				{"Bash(pnpm audit *)", "Bash(pnpm add *)", "PowerShell(pnpm add *)"},
				{"Edit(/spec.yaml)"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &resolve.Result{Spec: spec.Spec{Targets: []spec.Target{spec.TargetClaude}}.WithDefaults(), Entries: tt.entries}
			got := claudeRules(r)
			if !slices.EqualFunc(got.deny, tt.wantDeny, slices.Equal) || !slices.EqualFunc(got.ask, tt.wantAsk, slices.Equal) {
				t.Errorf("claudeRules() = deny %q, ask %q\nwant deny %q, ask %q", got.deny, got.ask, tt.wantDeny, tt.wantAsk)
			}
		})
	}
}

func TestMergeSettingsUnchanged(t *testing.T) {
	// A layout the encoder would change, four-space indent and an inline
	// object: nothing to add, so the bytes must come back as they were.
	old := "{\n    \"permissions\": {\"deny\": [\"Read(.env)\", \"Read(!.env.example)\"]}\n}\n"
	got, err := mergeSettings([]byte(old), permissionRules{deny: [][]string{{"Read(.env)", "Read(!.env.example)"}}})
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != old {
		t.Errorf("mergeSettings() rewrote a file it had nothing to add to:\n%s", got)
	}
}

func TestMergeSettingsEmptyFile(t *testing.T) {
	got, err := mergeSettings([]byte("  \n"), permissionRules{deny: [][]string{{"Read(.env)"}}})
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"permissions\": {\n    \"deny\": [\n      \"Read(.env)\"\n    ]\n  }\n}\n"
	if string(got) != want {
		t.Errorf("mergeSettings() =\n%s\nwant\n%s", got, want)
	}
}

func TestMergeSettingsKeepsCharacters(t *testing.T) {
	got, err := mergeSettings([]byte(`{"permissions": {"deny": ["Bash(echo > out)"]}}`), permissionRules{deny: [][]string{{"Bash(a && b)"}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"Bash(echo > out)"`, `"Bash(a && b)"`} {
		if !strings.Contains(string(got), want) {
			t.Errorf("mergeSettings() =\n%s\nwant it to contain %s unescaped", got, want)
		}
	}
}

func TestMergeSettingsErrors(t *testing.T) {
	tests := []struct {
		name, old, want string
	}{
		{"invalid JSON", `{"permissions": `, "invalid JSON"},
		{"not an object", `["Read(.env)"]`, "must be a JSON object"},
		{"duplicate key", `{"env": {}, "env": {}}`, `duplicate key "env"`},
		{"duplicate key in permissions", `{"permissions": {"deny": [], "deny": []}}`, `permissions: duplicate key "deny"`},
		{"trailing data", `{} {}`, "unexpected data after the object"},
		{"permissions not an object", `{"permissions": []}`, "permissions: must be a JSON object"},
		{"deny not a list of strings", `{"permissions": {"deny": [1]}}`, "permissions.deny must be a list of strings"},
		{"ask not a list of strings", `{"permissions": {"ask": "Bash(npm install *)"}}`, "permissions.ask must be a list of strings"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := mergeSettings([]byte(tt.old), permissionRules{deny: [][]string{{"Read(.env)"}}})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("mergeSettings() = %q, error %v\nwant an error containing %q", got, err, tt.want)
			}
		})
	}
}

func TestMergeSettingsAsk(t *testing.T) {
	install := [][]string{{"Bash(npm install *)", "Bash(go get *)"}}
	tests := []struct {
		name  string
		old   string // "" for a new file
		rules permissionRules
		want  string
	}{
		{
			name:  "new file lists deny before ask",
			rules: permissionRules{deny: [][]string{{"Bash(yarn *)"}}, ask: install},
			want: `{
  "$schema": "https://json.schemastore.org/claude-code-settings.json",
  "permissions": {
    "deny": [
      "Bash(yarn *)"
    ],
    "ask": [
      "Bash(npm install *)",
      "Bash(go get *)"
    ]
  }
}
`,
		},
		{
			name:  "ask only adds no deny list",
			old:   `{"permissions": {"allow": ["Bash(go test *)"]}}`,
			rules: permissionRules{ask: install},
			want: `{
  "permissions": {
    "allow": [
      "Bash(go test *)"
    ],
    "ask": [
      "Bash(npm install *)",
      "Bash(go get *)"
    ]
  }
}
`,
		},
		{
			name:  "user ask rules stay first and shared rules are reused",
			old:   `{"permissions": {"ask": ["Bash(git push *)", "Bash(npm install *)"], "deny": []}}`,
			rules: permissionRules{ask: install},
			want: `{
  "permissions": {
    "ask": [
      "Bash(git push *)",
      "Bash(npm install *)",
      "Bash(go get *)"
    ],
    "deny": []
  }
}
`,
		},
		{
			name:  "nothing to add keeps the bytes",
			old:   "{\"permissions\": {\"ask\": [\"Bash(npm install *)\", \"Bash(go get *)\"]}}\n",
			rules: permissionRules{ask: install},
			want:  "{\"permissions\": {\"ask\": [\"Bash(npm install *)\", \"Bash(go get *)\"]}}\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var old []byte
			if tt.old != "" {
				old = []byte(tt.old)
			}
			got, err := mergeSettings(old, tt.rules)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Errorf("mergeSettings() =\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}

func TestRenderSettingsNothingToWrite(t *testing.T) {
	r := &resolve.Result{Spec: spec.Spec{Targets: []spec.Target{spec.TargetClaude}}.WithDefaults()}
	if _, ok, err := renderSettings(r, nil); ok || err != nil {
		t.Errorf("renderSettings() with no rules and no file = %v, %v; want nothing to write", ok, err)
	}

	old := []byte("{\"model\": \"opus\"}\n")
	f, ok, err := renderSettings(r, fstest.MapFS{settingsPath: &fstest.MapFile{Data: old}})
	if err != nil || !ok || string(f.Content) != string(old) {
		t.Errorf("renderSettings() with no rules = %q, %v, %v; want the existing file unchanged", f.Content, ok, err)
	}
}

func TestMergeGitignore(t *testing.T) {
	const block = "# cyanotype:begin (generated from cyanotype.yaml; regeneration replaces this block)\n" +
		"cyanotype.local.yaml\n.agents.local/\n# cyanotype:end"
	crlf := strings.ReplaceAll(block, "\n", "\r\n")
	tests := []struct {
		name, old, want string
	}{
		{"new file", "", block + "\n"},
		{"appended after a blank line", "dist/\n", "dist/\n\n" + block + "\n"},
		{"no trailing newline", "dist/", "dist/\n\n" + block + "\n"},
		{"already ends with a blank line", "dist/\n\n", "dist/\n\n" + block + "\n"},
		{
			name: "block replaced in place",
			old:  "a\n# cyanotype:begin\nold\n# cyanotype:end\nb\n",
			want: "a\n" + block + "\nb\n",
		},
		{
			name: "CRLF kept, rest byte for byte",
			old:  "a\r\n  # cyanotype:begin old\r\nold\r\n# cyanotype:end\r\nb",
			want: "a\r\n" + crlf + "\r\nb",
		},
		{"CRLF append", "a\r\n", "a\r\n\r\n" + crlf + "\r\n"},
		{"up to date", block + "\n", block + "\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := mergeGitignore(tt.old, gitignoreEntries)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("mergeGitignore() =\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

func TestGitignoreLines(t *testing.T) {
	entry := func(id string, lines ...string) resolve.Entry {
		return resolve.Entry{Entry: catalog.Entry{ID: id, Enforce: &catalog.Enforce{Gitignore: lines}}}
	}
	tests := []struct {
		name    string
		entries []resolve.Entry
		want    []string
	}{
		{
			name: "no entry lines",
			want: gitignoreEntries,
		},
		{
			name:    "entry lines after the personal files, in catalog order",
			entries: []resolve.Entry{entry("secrets", ".env", ".env.*", "!.env.example")},
			want:    []string{"cyanotype.local.yaml", ".agents.local/", ".env", ".env.*", "!.env.example"},
		},
		{
			name:    "entries in selection order, a shared line written once",
			entries: []resolve.Entry{entry("db", "*.db"), {Entry: catalog.Entry{ID: "plain"}}, entry("cache", "*.db", ".cache/")},
			want:    []string{"cyanotype.local.yaml", ".agents.local/", "*.db", ".cache/"},
		},
		{
			name: "a line reused before a negation is written again after it",
			entries: []resolve.Entry{
				entry("secrets", ".env.*", "!.env.example"),
				entry("other", "!.env.example", ".env.*"),
			},
			want: []string{"cyanotype.local.yaml", ".agents.local/", ".env.*", "!.env.example", ".env.*"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := gitignoreLines(&resolve.Result{Entries: tt.entries})
			if !slices.Equal(got, tt.want) {
				t.Errorf("gitignoreLines() =\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

// TestRealCatalogGitignoreOrder renders the .gitignore block for every
// entry of the real catalog that has gitignore lines, and checks that each
// entry's lines come out in catalog order, so negations keep working.
func TestRealCatalogGitignoreOrder(t *testing.T) {
	c, err := catalog.LoadRoot(os.DirFS(filepath.Join("..", "..", "catalog")))
	if err != nil {
		t.Fatal(err)
	}
	// Entries are added directly, without resolving, so exclusive entries
	// can be checked together.
	r := &resolve.Result{}
	var groups [][]string
	for _, e := range c.Entries() {
		if e.Enforce != nil && len(e.Enforce.Gitignore) > 0 {
			r.Entries = append(r.Entries, resolve.Entry{Entry: e})
			groups = append(groups, e.Enforce.Gitignore)
		}
	}
	if len(groups) == 0 {
		t.Fatal("found no catalog entry with gitignore lines")
	}
	f, err := renderGitignore(r, nil)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(string(f.Content), "\n"), "\n")
	for _, g := range groups {
		if !isSubsequence(g, lines) {
			t.Errorf(".gitignore breaks the order of %q:\n%s", g, f.Content)
		}
	}
}

func TestMergeGitignoreErrors(t *testing.T) {
	tests := []struct {
		name, old, want string
	}{
		{"unclosed", "a\n# cyanotype:begin\nb\n", "line 2: # cyanotype:begin is never closed with # cyanotype:end"},
		{"stray end", "# cyanotype:end\n", "line 1: # cyanotype:end without a # cyanotype:begin before it"},
		{"nested", "# cyanotype:begin\n# cyanotype:begin\n# cyanotype:end\n", "line 2: # cyanotype:begin inside an open block"},
		{
			name: "two blocks",
			old:  "# cyanotype:begin\n# cyanotype:end\n# cyanotype:begin\n# cyanotype:end\n",
			want: "line 3: a second # cyanotype:begin block",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := mergeGitignore(tt.old, gitignoreEntries)
			if err == nil || err.Error() != tt.want {
				t.Errorf("mergeGitignore() = %q, error %v\nwant error %q", got, err, tt.want)
			}
		})
	}
}
