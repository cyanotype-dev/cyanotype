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

func TestMergeDenyKeepsEntryOrder(t *testing.T) {
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
			got := mergeDeny(tt.existing, tt.groups)
			if !slices.Equal(got, tt.want) {
				t.Errorf("mergeDeny() =\n%q\nwant\n%q", got, tt.want)
			}
			checkDenyOrder(t, got, tt.groups)
			if !slices.Equal(got[:len(tt.existing)], tt.existing) {
				t.Errorf("mergeDeny() changed the existing rules: %q", got)
			}
			if again := mergeDeny(got, tt.groups); !slices.Equal(again, got) {
				t.Errorf("merging twice changed the list:\n%q\nthen\n%q", got, again)
			}
		})
	}
}

// TestRealCatalogDenyOrder renders every entry of the real catalog that has
// deny rules, over a settings file holding each entry's rules in reverse,
// and checks that every entry's rules still come out in catalog order. It
// fails if a change to merging, or a new entry, breaks the order a negation
// depends on.
func TestRealCatalogDenyOrder(t *testing.T) {
	c, err := catalog.LoadRoot(os.DirFS(filepath.Join("..", "..", "catalog")))
	if err != nil {
		t.Fatal(err)
	}
	// Entries are added directly, without resolving, so exclusive entries
	// such as npm and pnpm can be checked together.
	r := &resolve.Result{Spec: spec.Spec{Targets: []spec.Target{spec.TargetClaude}}.WithDefaults()}
	var reversed []string
	for _, e := range c.Entries() {
		if e.Enforce == nil || e.Enforce.Claude == nil || len(e.Enforce.Claude.Deny) == 0 {
			continue
		}
		r.Entries = append(r.Entries, resolve.Entry{Entry: e})
		rules := slices.Clone(e.Enforce.Claude.Deny)
		slices.Reverse(rules)
		reversed = append(reversed, rules...)
	}
	groups := denyGroups(r)
	if len(groups) < 2 {
		t.Fatalf("found %d catalog entries with deny rules, want several", len(groups))
	}

	old, err := json.Marshal(map[string]any{"permissions": map[string]any{"deny": reversed}})
	if err != nil {
		t.Fatal(err)
	}
	f, ok, err := renderSettings(r, fstest.MapFS{settingsPath: &fstest.MapFile{Data: old}})
	if err != nil || !ok {
		t.Fatalf("renderSettings() = %v, %v", ok, err)
	}
	var got struct {
		Permissions struct{ Deny []string }
	}
	if err := json.Unmarshal(f.Content, &got); err != nil {
		t.Fatal(err)
	}
	checkDenyOrder(t, got.Permissions.Deny, groups)
}

func TestMergeSettingsUnchanged(t *testing.T) {
	// A layout the encoder would change, four-space indent and an inline
	// object: nothing to add, so the bytes must come back as they were.
	old := "{\n    \"permissions\": {\"deny\": [\"Read(.env)\", \"Read(!.env.example)\"]}\n}\n"
	got, err := mergeSettings([]byte(old), [][]string{{"Read(.env)", "Read(!.env.example)"}})
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != old {
		t.Errorf("mergeSettings() rewrote a file it had nothing to add to:\n%s", got)
	}
}

func TestMergeSettingsEmptyFile(t *testing.T) {
	got, err := mergeSettings([]byte("  \n"), [][]string{{"Read(.env)"}})
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"permissions\": {\n    \"deny\": [\n      \"Read(.env)\"\n    ]\n  }\n}\n"
	if string(got) != want {
		t.Errorf("mergeSettings() =\n%s\nwant\n%s", got, want)
	}
}

func TestMergeSettingsKeepsCharacters(t *testing.T) {
	got, err := mergeSettings([]byte(`{"permissions": {"deny": ["Bash(echo > out)"]}}`), [][]string{{"Bash(a && b)"}})
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := mergeSettings([]byte(tt.old), [][]string{{"Read(.env)"}})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("mergeSettings() = %q, error %v\nwant an error containing %q", got, err, tt.want)
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
			got, err := mergeGitignore(tt.old)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("mergeGitignore() =\n%q\nwant\n%q", got, tt.want)
			}
		})
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
			got, err := mergeGitignore(tt.old)
			if err == nil || err.Error() != tt.want {
				t.Errorf("mergeGitignore() = %q, error %v\nwant error %q", got, err, tt.want)
			}
		})
	}
}
