package simple

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/cyanotype-dev/cyanotype/internal/detect"
	"github.com/cyanotype-dev/cyanotype/internal/preset"
	"github.com/cyanotype-dev/cyanotype/internal/resolve"
	"github.com/cyanotype-dev/cyanotype/internal/spec"
)

// found builds a detection result that found ids, with package.json
// scripts.
func found(scripts []string, ids ...string) detect.Result {
	r := detect.Result{Scripts: scripts}
	for _, id := range ids {
		r.Findings = append(r.Findings, detect.Finding{ID: id, Evidence: []detect.Evidence{{File: id + ".test"}}})
	}
	return r
}

func TestDetectStack(t *testing.T) {
	c, presets := loadBundled(t)
	tests := []struct {
		name          string
		result        detect.Result
		with, without []string
		want          Detected
	}{
		{
			name: "complete Next.js stack in the preset's order, with commands from scripts",
			// Catalog order, as detect returns it.
			result: found([]string{"build", "dev", "lint", "start"}, "eslint-prettier", "nextjs", "node", "pnpm", "tailwind", "typescript"),
			want: Detected{
				Selections:     []string{"typescript", "node", "pnpm", "nextjs", "tailwind", "eslint-prettier"},
				Commands:       spec.Commands{Dev: "pnpm dev", Build: "pnpm build", Lint: "pnpm lint"},
				CommandScripts: []string{"dev", "build", "lint"},
				ProjectType:    preset.Website,
			},
		},
		{
			name:   "two lockfiles are a choice",
			result: found(nil, "node", "npm", "pnpm"),
			want: Detected{
				Selections: []string{"node", "npm", "pnpm"},
				Choices:    []Choice{{Kind: ChoiceClash, IDs: []string{"npm", "pnpm"}, Category: "package-manager"}},
			},
		},
		{
			name:   "an open clash of package managers leaves the commands out",
			result: found([]string{"dev", "test"}, "node", "npm", "pnpm"),
			want: Detected{
				Selections: []string{"node", "npm", "pnpm"},
				Choices:    []Choice{{Kind: ChoiceClash, IDs: []string{"npm", "pnpm"}, Category: "package-manager"}},
			},
		},
		{
			name:   "with answers a clash",
			result: found([]string{"test"}, "node", "npm", "pnpm"),
			with:   []string{"pnpm"},
			want: Detected{
				Selections:     []string{"node", "pnpm"},
				Commands:       spec.Commands{Test: "pnpm test"},
				CommandScripts: []string{"test"},
				// Only website presets use Node.js with pnpm.
				ProjectType: preset.Website,
			},
		},
		{
			name:   "requirements are added, and an unmet group is a choice with the closest preset's default",
			result: found(nil, "nextjs", "shadcn-ui"),
			want: Detected{
				Selections:  []string{"node", "nextjs", "tailwind", "shadcn-ui"},
				Added:       []Requirement{{ID: "node", By: "nextjs"}, {ID: "tailwind", By: "shadcn-ui"}},
				Choices:     []Choice{{Kind: ChoiceGroup, IDs: []string{"npm", "pnpm"}, By: "node", Default: "pnpm"}},
				ProjectType: preset.Website,
			},
		},
		{
			// An app.json in a Next.js project reveals Expo by mistake.
			name:    "without removes a false positive",
			result:  found(nil, "expo", "nextjs", "node", "pnpm", "typescript"),
			without: []string{"expo"},
			want: Detected{
				Selections:  []string{"typescript", "node", "pnpm", "nextjs"},
				ProjectType: preset.Website,
			},
		},
		{
			// Expo app and Next.js + Supabase share three entries each: the
			// first of them sets the order and the default.
			name:   "both frameworks: the closest presets build different things",
			result: found(nil, "expo", "nextjs", "node", "typescript"),
			want: Detected{
				Selections: []string{"typescript", "node", "expo", "nextjs"},
				Choices:    []Choice{{Kind: ChoiceGroup, IDs: []string{"npm", "pnpm"}, By: "node", Default: "npm"}},
			},
		},
		{
			name:   "Go alone could be a CLI or an API",
			result: found(nil, "go", "golangci-lint"),
			want:   Detected{Selections: []string{"go", "golangci-lint"}},
		},
		{
			name:   "a detected i18n library means the app is translated",
			result: found(nil, "i18n-next-intl", "nextjs", "node", "pnpm"),
			want: Detected{
				Selections:   []string{"node", "pnpm", "nextjs", "i18n-next-intl"},
				ProjectType:  preset.Website,
				Multilingual: true,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DetectStack(c, presets, tt.result, tt.with, tt.without)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("DetectStack =\n%+v\nwant\n%+v", got, tt.want)
			}
		})
	}
}

func TestDetectStackErrors(t *testing.T) {
	c, presets := loadBundled(t)
	r := found(nil, "node", "npm")
	tests := []struct {
		name          string
		with, without []string
		msgs          []string
	}{
		{"unknown ids", []string{"pnpn"}, []string{"exppo"}, []string{
			`with: unknown entry "pnpn"; did you mean "pnpm"?`,
			`without: unknown entry "exppo"; did you mean "expo"?`,
		}},
		{"both lists", []string{"npm"}, []string{"npm"}, []string{`"npm" is both added and removed`}},
		{"two of a radio category", []string{"npm", "pnpm"}, nil, []string{`"npm" and "pnpm" are both in radio category "package-manager"`}},
		{"removing what was not detected", nil, []string{"pnpm"}, []string{`without: "pnpm" was not detected`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DetectStack(c, presets, r, tt.with, tt.without)
			if err == nil {
				t.Fatal("want an error")
			}
			for _, msg := range tt.msgs {
				if !strings.Contains(err.Error(), msg) {
					t.Errorf("error %q does not contain %q", err, msg)
				}
			}
		})
	}
}

func TestDetectedStack(t *testing.T) {
	c, presets := loadBundled(t)
	tests := []struct {
		name   string
		result detect.Result
		t      preset.ProjectType
		i18n   []string
	}{
		{"Next.js gets next-intl", found(nil, "nextjs", "node", "pnpm"), preset.Website, []string{"i18n-next-intl"}},
		{"Vite gets react-i18next", found(nil, "node", "pnpm", "vite-react"), preset.Website, []string{"i18n-react-i18next"}},
		{"a detected library is not added twice", found(nil, "i18n-next-intl", "nextjs", "node", "pnpm"), preset.Website, nil},
		// next-intl requires Next.js, which a Node.js script does not have.
		{"a library that does not fit is left out", found(nil, "node", "pnpm"), preset.Website, nil},
		{"no preset of the type shares an entry", found(nil, "go"), preset.Website, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, err := DetectStack(c, presets, tt.result, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			st, err := d.Stack(c, presets, tt.t)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(st.I18nSelections, tt.i18n) {
				t.Errorf("I18nSelections = %q, want %q", st.I18nSelections, tt.i18n)
			}
			if !slices.Equal(st.Selections, d.Selections) || st.Commands != d.Commands {
				t.Errorf("Stack() = %+v, want the detected selections and commands", st)
			}
		})
	}

	d, err := DetectStack(c, presets, found(nil, "node", "npm", "pnpm"), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Stack(c, presets, preset.Website); err == nil {
		t.Error("Stack() with a choice left: want an error")
	}
}

// TestBuildStackFromDetection checks that a detected stack becomes a spec
// that resolves, with entries the stack already has not added twice.
func TestBuildStackFromDetection(t *testing.T) {
	c, presets := loadBundled(t)
	d, err := DetectStack(c, presets, found([]string{"dev"}, "conventional-commits", "nextjs", "node", "pnpm", "typescript"), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	st, err := d.Stack(c, presets, d.ProjectType)
	if err != nil {
		t.Fatal(err)
	}
	s, err := BuildStack(c, st, Answers{
		Name: "shop", Goal: "An online shop.", Autonomy: Balanced,
		I18n: I18nLater, Agents: []Agent{AgentClaude},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"typescript", "node", "pnpm", "nextjs", "conventional-commits",
		"i18n-rules", "i18n-next-intl",
		"small-commits", "definition-of-done", "no-destructive-commands", "no-test-tampering", "secrets-in-env",
		"no-unrequested-changes", "no-duplicate-files", "rule-maintenance-propose", "dependency-approval",
	}
	if !slices.Equal(s.Selections, want) {
		t.Errorf("Selections =\n%q\nwant\n%q", s.Selections, want)
	}
	if s.Commands.Dev != "pnpm dev" || s.Mode != spec.ModeSimple || s.CatalogVersion != c.Version() {
		t.Errorf("spec = %+v", s)
	}
	if _, _, err := resolve.Resolve(c, spec.New(s)); err != nil {
		t.Errorf("the spec does not resolve:\n%v", err)
	}
}

func TestFits(t *testing.T) {
	c, _ := loadBundled(t)
	sel := []string{"node", "pnpm", "nextjs", "i18n-next-intl"}
	tests := []struct {
		id   string
		want bool
	}{
		{"vitest", true},
		{"i18n-next-intl", false},     // already selected
		{"i18n-react-i18next", false}, // same radio category as next-intl
		{"npm", false},                // same radio category as pnpm
		{"fastapi", false},            // requires python
		{"shadcn-ui", false},          // requires tailwind
		{"ghost", false},
	}
	for _, tt := range tests {
		if got := fits(c, sel, tt.id); got != tt.want {
			t.Errorf("fits(%q) = %v, want %v", tt.id, got, tt.want)
		}
	}
}

func TestLanguagesAndLearning(t *testing.T) {
	c, _ := loadBundled(t)
	if got := Languages(c, []string{"node", "typescript", "pnpm", "python"}); !slices.Equal(got, []string{"typescript", "python"}) {
		t.Errorf("Languages = %q", got)
	}
	if OffersLearningFor(c, []string{"typescript", "node"}, "typescript") {
		t.Error("learning offered to someone who knows the stack's language")
	}
	if !OffersLearningFor(c, []string{"go"}, "python") {
		t.Error("learning not offered to someone who does not know Go")
	}
}
