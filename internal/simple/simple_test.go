package simple

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	bundled "github.com/cyanotype-dev/cyanotype/catalog"
	"github.com/cyanotype-dev/cyanotype/internal/catalog"
	"github.com/cyanotype-dev/cyanotype/internal/preset"
	"github.com/cyanotype-dev/cyanotype/internal/render"
	"github.com/cyanotype-dev/cyanotype/internal/resolve"
	"github.com/cyanotype-dev/cyanotype/internal/spec"
)

func loadBundled(t *testing.T) (*catalog.Catalog, []preset.Preset) {
	t.Helper()
	c, err := catalog.LoadRoot(bundled.FS())
	if err != nil {
		t.Fatalf("the bundled catalog is invalid:\n%v", err)
	}
	presets, err := preset.LoadRoot(bundled.FS(), c)
	if err != nil {
		t.Fatalf("the bundled presets are invalid:\n%v", err)
	}
	return c, presets
}

func findPreset(t *testing.T, presets []preset.Preset, id string) preset.Preset {
	t.Helper()
	for _, p := range presets {
		if p.ID == id {
			return p
		}
	}
	t.Fatalf("no bundled preset %q", id)
	return preset.Preset{}
}

func ids(presets []preset.Preset) []string {
	var out []string
	for _, p := range presets {
		out = append(out, p.ID)
	}
	return out
}

func TestAutonomySelections(t *testing.T) {
	tests := []struct {
		autonomy Autonomy
		want     []string
	}{
		{AsksOften, []string{
			"conventional-commits", "small-commits", "plan-before-code",
			"definition-of-done", "no-destructive-commands", "no-test-tampering", "secrets-in-env",
			"no-unrequested-changes", "no-duplicate-files", "rule-maintenance-propose", "dependency-approval",
		}},
		{Balanced, []string{
			"conventional-commits", "small-commits",
			"definition-of-done", "no-destructive-commands", "no-test-tampering", "secrets-in-env",
			"no-unrequested-changes", "no-duplicate-files", "rule-maintenance-propose", "dependency-approval",
		}},
		{Autonomous, []string{
			"conventional-commits", "small-commits", "branch-per-task",
			"definition-of-done", "no-destructive-commands", "no-test-tampering", "secrets-in-env",
			"no-unrequested-changes", "no-duplicate-files", "rule-maintenance-propose",
		}},
	}
	c, _ := loadBundled(t)
	for _, tt := range tests {
		got := AutonomySelections(tt.autonomy)
		if !slices.Equal(got, tt.want) {
			t.Errorf("AutonomySelections(%s) = %q, want %q", tt.autonomy, got, tt.want)
		}
		for _, id := range got {
			if _, ok := c.Get(id); !ok {
				t.Errorf("AutonomySelections(%s) selects %q, which is not in the catalog", tt.autonomy, id)
			}
		}
	}
}

func TestTargets(t *testing.T) {
	tests := []struct {
		agents []Agent
		want   []spec.Target
	}{
		{[]Agent{AgentClaude}, []spec.Target{spec.TargetClaude}},
		{[]Agent{AgentCodex}, []spec.Target{spec.TargetAgentsMD}},
		{[]Agent{AgentOther}, []spec.Target{spec.TargetAgentsMD}},
		{[]Agent{AgentCodex, AgentClaude}, []spec.Target{spec.TargetClaude, spec.TargetAgentsMD}},
		{[]Agent{AgentCursor}, []spec.Target{spec.TargetAgentsMD, spec.TargetCursor}},
		{[]Agent{AgentCopilot, AgentGemini, AgentClaude, AgentOther},
			[]spec.Target{spec.TargetClaude, spec.TargetAgentsMD, spec.TargetGemini, spec.TargetCopilot}},
	}
	for _, tt := range tests {
		if got := Targets(tt.agents); !slices.Equal(got, tt.want) {
			t.Errorf("Targets(%q) = %q, want %q", tt.agents, got, tt.want)
		}
	}
}

func TestCandidates(t *testing.T) {
	c, presets := loadBundled(t)
	tests := []struct {
		projectType preset.ProjectType
		language    string
		want        []string
	}{
		{preset.Website, "typescript", []string{"nextjs-supabase", "vite-react-app"}},
		{preset.Website, LanguageNone, []string{"nextjs-supabase", "vite-react-app"}},
		{preset.Website, "go", []string{"nextjs-supabase", "vite-react-app"}},
		{preset.MobileApp, "python", []string{"expo-app"}},
		{preset.API, "go", []string{"go-api"}},
		{preset.API, "python", []string{"fastapi-postgres"}},
		{preset.API, LanguageOther, []string{"fastapi-postgres", "go-api"}},
		{preset.Script, "go", []string{"go-cli"}},
		{preset.Script, "typescript", []string{"go-cli", "python-script"}},
	}
	for _, tt := range tests {
		got := ids(Candidates(c, presets, tt.projectType, tt.language))
		if !slices.Equal(got, tt.want) {
			t.Errorf("Candidates(%s, %s) = %q, want %q", tt.projectType, tt.language, got, tt.want)
		}
	}
}

func TestOffersLearning(t *testing.T) {
	c, presets := loadBundled(t)
	goCLI := findPreset(t, presets, "go-cli")
	tests := []struct {
		language string
		want     bool
	}{
		{"go", false},
		{"python", true},
		{LanguageOther, true},
		{LanguageNone, true},
		{"", true},
	}
	for _, tt := range tests {
		if got := OffersLearning(c, goCLI, tt.language); got != tt.want {
			t.Errorf("OffersLearning(go-cli, %q) = %v, want %v", tt.language, got, tt.want)
		}
	}
}

func TestOptionsFromCatalog(t *testing.T) {
	c, _ := loadBundled(t)
	var langs, learning []string
	for _, o := range LanguageOptions(c) {
		langs = append(langs, o.Value)
	}
	for _, o := range LearningOptions(c) {
		learning = append(learning, o.Value)
	}
	if want := []string{"go", "python", "typescript", LanguageOther, LanguageNone}; !slices.Equal(langs, want) {
		t.Errorf("LanguageOptions = %q, want %q", langs, want)
	}
	if want := []string{LearningOff, "explain", "guided", "hands-on"}; !slices.Equal(learning, want) {
		t.Errorf("LearningOptions = %q, want %q", learning, want)
	}
}

func TestCanonical(t *testing.T) {
	locales := []struct{ in, want string }{
		{"en", "en"}, {"pt_br", "pt-BR"}, {"PT-br", "pt-BR"}, {"zh-hant-tw", "zh-Hant-TW"}, {" es-419 ", "es-419"},
	}
	for _, tt := range locales {
		if got := CanonicalLocale(tt.in); got != tt.want {
			t.Errorf("CanonicalLocale(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
	languages := []struct{ in, want string }{
		{"Italian", "italian"}, {" Brazilian  Portuguese ", "brazilian-portuguese"}, {"", ""},
	}
	for _, tt := range languages {
		if got := CanonicalLanguage(tt.in); got != tt.want {
			t.Errorf("CanonicalLanguage(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestBuild(t *testing.T) {
	c, presets := loadBundled(t)
	next := findPreset(t, presets, "nextjs-supabase")
	goCLI := findPreset(t, presets, "go-cli")
	base := Answers{Name: "demo", Goal: "A demo.", Autonomy: Balanced, Agents: []Agent{AgentClaude}}

	t.Run("preset first, then autonomy", func(t *testing.T) {
		s, err := Build(c, goCLI, base)
		if err != nil {
			t.Fatal(err)
		}
		want := append(slices.Clone(goCLI.Selections), AutonomySelections(Balanced)...)
		if !slices.Equal(s.Selections, want) {
			t.Errorf("selections = %q, want %q", s.Selections, want)
		}
		if s.Version != spec.CurrentVersion || s.CatalogVersion != c.Version() || s.Mode != spec.ModeSimple {
			t.Errorf("version %d, catalog_version %q, mode %q; want %d, %q, %q",
				s.Version, s.CatalogVersion, s.Mode, spec.CurrentVersion, c.Version(), spec.ModeSimple)
		}
		if s.Project.Name != "demo" || s.Project.Goal != "A demo." || s.Commands != goCLI.Commands {
			t.Errorf("project %+v, commands %+v", s.Project, s.Commands)
		}
		if s.I18n != nil || s.Languages != (spec.Languages{}) {
			t.Errorf("i18n %+v, languages %+v; want neither", s.I18n, s.Languages)
		}
	})

	t.Run("i18n later", func(t *testing.T) {
		a := base
		a.I18n = I18nLater
		s, err := Build(c, next, a)
		if err != nil {
			t.Fatal(err)
		}
		if s.I18n == nil || s.I18n.DefaultLocale != "en" || s.I18n.Locales != nil || s.I18n.TranslationsBy != "" {
			t.Errorf("i18n = %+v, want default locale en only", s.I18n)
		}
		n := len(next.Selections)
		if got := s.Selections[n : n+2]; !slices.Equal(got, []string{"i18n-rules", "i18n-next-intl"}) {
			t.Errorf("selections after the preset = %q, want the i18n entries", got)
		}
	})

	t.Run("i18n yes", func(t *testing.T) {
		a := base
		a.I18n, a.Locales = I18nYes, []string{"it", "en"}
		s, err := Build(c, next, a)
		if err != nil {
			t.Fatal(err)
		}
		if s.I18n == nil || s.I18n.DefaultLocale != "it" || !slices.Equal(s.I18n.Locales, []string{"it", "en"}) {
			t.Errorf("i18n = %+v, want it then en", s.I18n)
		}
	})

	t.Run("learning and replies", func(t *testing.T) {
		a := base
		a.Learning, a.AgentReplies = "guided", "Italian"
		s, err := Build(c, goCLI, a)
		if err != nil {
			t.Fatal(err)
		}
		if last := s.Selections[len(s.Selections)-1]; last != "learning-guided" {
			t.Errorf("last selection = %q, want learning-guided", last)
		}
		if s.Languages.AgentReplies != "italian" {
			t.Errorf("agent_replies = %q, want italian", s.Languages.AgentReplies)
		}
	})

	t.Run("english replies stay implicit", func(t *testing.T) {
		a := base
		a.Learning, a.AgentReplies = LearningOff, "English"
		s, err := Build(c, goCLI, a)
		if err != nil {
			t.Fatal(err)
		}
		if s.Languages.AgentReplies != "" || slices.ContainsFunc(s.Selections, func(id string) bool { return strings.HasPrefix(id, "learning-") }) {
			t.Errorf("agent_replies %q, selections %q; want neither", s.Languages.AgentReplies, s.Selections)
		}
	})

	t.Run("invalid answers", func(t *testing.T) {
		a := Answers{Autonomy: "wild", I18n: I18nYes, Locales: []string{"en"}, Learning: "deep", Agents: []Agent{"robot"}}
		_, err := Build(c, next, a)
		if err == nil {
			t.Fatal("Build succeeded with invalid answers")
		}
		for _, want := range []string{
			"the project name is required", "the project goal is required", `unknown autonomy "wild"`,
			`unknown agent "robot"`, `unknown learning mode "deep"`, "needs at least two locales",
		} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error is missing %q:\n%v", want, err)
			}
		}
	})
}

// TestEveryCombinationGenerates builds the spec of every bundled preset
// with every autonomy level, translation choice and learning mode, and
// checks that it resolves and renders without errors or warnings: whatever
// a user answers, the first generate run is clean.
func TestEveryCombinationGenerates(t *testing.T) {
	c, presets := loadBundled(t)
	for _, p := range presets {
		for _, autonomy := range AutonomyOptions() {
			for _, i18n := range I18nOptions() {
				for _, learning := range LearningOptions(c) {
					a := Answers{
						Name: "demo", Goal: "A demo project.",
						Autonomy: Autonomy(autonomy.Value), I18n: I18n(i18n.Value), Learning: learning.Value,
						Locales: []string{"en", "it"}, AgentReplies: "italian",
						Agents: []Agent{AgentClaude, AgentCodex},
					}
					name := fmt.Sprintf("%s/%s/%s/%s", p.ID, autonomy.Value, i18n.Value, learning.Value)
					t.Run(name, func(t *testing.T) {
						s, err := Build(c, p, a)
						if err != nil {
							t.Fatal(err)
						}
						res, warnings, err := resolve.Resolve(c, spec.New(s))
						if err != nil {
							t.Fatalf("resolve: %v", err)
						}
						if len(warnings) > 0 {
							t.Errorf("resolve warnings: %v", warnings)
						}
						if _, warnings, err := render.Render(res, nil); err != nil || len(warnings) > 0 {
							t.Errorf("render: error %v, warnings %v", err, warnings)
						}
					})
				}
			}
		}
	}
}
