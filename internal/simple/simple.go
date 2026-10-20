// Package simple holds simple mode: the few questions that start a project,
// and how their answers become a spec. See the Simple mode section of
// docs/DESIGN.md.
//
// The questions and their options are data here, not text in an interface,
// so the CLI prompts, the future TUI form and the MCP server all ask the
// same thing and map the answers the same way. Like every internal package
// it never prints or reads input: asking belongs to the interfaces.
package simple

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/cyanotype-dev/cyanotype/internal/catalog"
	"github.com/cyanotype-dev/cyanotype/internal/preset"
	"github.com/cyanotype-dev/cyanotype/internal/spec"
)

// Option is one answer a question offers.
type Option struct {
	// Value is what the answer is called in flags and in code, such as
	// "website" or "balanced".
	Value string
	// Label is the answer as people read it, such as "Website".
	Label string
	// Help says in a few words what choosing the answer means; may be empty.
	Help string
	// Later marks an answer shown as coming in a later version, which
	// cannot be chosen yet.
	Later bool
}

// ProjectTypeOptions returns the answers to "What do you want to build?".
// Desktop apps and games are shown as coming later, so users see they are
// planned rather than forgotten.
func ProjectTypeOptions() []Option {
	return []Option{
		{Value: string(preset.Website), Label: "Website", Help: "a site or web app people open in a browser"},
		{Value: string(preset.MobileApp), Label: "Mobile app", Help: "an app for iOS and Android"},
		{Value: string(preset.API), Label: "API", Help: "a backend other programs call over HTTP"},
		{Value: string(preset.Script), Label: "Script or automation", Help: "a script or a command-line tool"},
		{Value: "desktop-app", Label: "Desktop app", Later: true},
		{Value: "game", Label: "Game", Later: true},
	}
}

// Answers to "Do you already know a programming language?" besides the
// languages of the catalog.
const (
	// LanguageOther is a language the catalog has no preset for.
	LanguageOther = "other"
	// LanguageNone means the user does not know a programming language yet.
	LanguageNone = "none"
)

// languageCategory and learningCategory are the catalog categories the
// language and learning questions offer.
const (
	languageCategory = "language"
	learningCategory = "learning"
	learningPrefix   = "learning-"
)

// LanguageOptions returns the answers to "Do you already know a programming
// language?": every language of the catalog, then another language, then
// none.
func LanguageOptions(c *catalog.Catalog) []Option {
	var opts []Option
	for _, e := range c.Entries() {
		if e.Category == languageCategory {
			opts = append(opts, Option{Value: e.ID, Label: e.Name})
		}
	}
	return append(opts,
		Option{Value: LanguageOther, Label: "Another language"},
		Option{Value: LanguageNone, Label: "Not yet"},
	)
}

// LearningOff is the answer that leaves learning mode off, the default.
const LearningOff = "off"

// LearningOptions returns the answers to the learning mode question: off,
// then every entry of the catalog's learning category, named without its
// "learning-" prefix, such as "guided".
func LearningOptions(c *catalog.Catalog) []Option {
	opts := []Option{{Value: LearningOff, Label: "No", Help: "the agent just writes the code"}}
	for _, e := range c.Entries() {
		if e.Category == learningCategory {
			opts = append(opts, Option{
				Value: strings.TrimPrefix(e.ID, learningPrefix),
				Label: strings.TrimPrefix(e.Name, "Learning mode: "),
				Help:  e.Summary,
			})
		}
	}
	return opts
}

// Autonomy is how much the agent may do before asking.
type Autonomy string

// Allowed values for Autonomy.
const (
	AsksOften  Autonomy = "asks-often"
	Balanced   Autonomy = "balanced"
	Autonomous Autonomy = "autonomous"
)

// AutonomyOptions returns the answers to "How much autonomy should the
// agent have?".
func AutonomyOptions() []Option {
	return []Option{
		{Value: string(AsksOften), Label: "Asks often", Help: "plans before larger changes and asks before adding dependencies"},
		{Value: string(Balanced), Label: "Balanced", Help: "works on its own but asks before adding dependencies"},
		{Value: string(Autonomous), Label: "Autonomous", Help: "works on its own, one branch per task"},
	}
}

// Rules every project gets, whatever the autonomy: commit hygiene, safety
// and an honest definition of done. Only how often the agent asks changes.
var (
	gitEntries    = []string{"conventional-commits", "small-commits"}
	safetyEntries = []string{
		"definition-of-done",
		"no-destructive-commands",
		"no-test-tampering",
		"secrets-in-env",
		"no-unrequested-changes",
		"no-duplicate-files",
		"rule-maintenance-propose",
	}
)

// AutonomySelections returns the catalog entries an autonomy level adds to
// the preset's stack, in the order they go into the spec.
func AutonomySelections(a Autonomy) []string {
	sel := slices.Clone(gitEntries)
	if a == Autonomous {
		// Unsupervised work stays on its own branch until it is reviewed.
		sel = append(sel, "branch-per-task")
	}
	if a == AsksOften {
		sel = append(sel, "plan-before-code")
	}
	sel = append(sel, safetyEntries...)
	if a == AsksOften || a == Balanced {
		sel = append(sel, "dependency-approval")
	}
	return sel
}

// I18n is the answer to "Should the app support more than one language?".
type I18n string

// Allowed values for I18n.
const (
	I18nNo    I18n = "no"
	I18nLater I18n = "later"
	I18nYes   I18n = "yes"
)

// DefaultLocale is the app's language when the user names none.
const DefaultLocale = "en"

// I18nOptions returns the answers to "Should the app support more than one
// language?".
func I18nOptions() []Option {
	return []Option{
		{Value: string(I18nNo), Label: "No"},
		{Value: string(I18nLater), Label: "Later", Help: "one language now, ready for more"},
		{Value: string(I18nYes), Label: "Yes", Help: "several languages from the start"},
	}
}

// AsksI18n reports whether the translation question applies to a project
// type: only apps with a user interface have text to translate.
func AsksI18n(t preset.ProjectType) bool {
	return t == preset.Website || t == preset.MobileApp
}

// Agent is a coding agent the user works with.
type Agent string

// Allowed values for Agent.
const (
	AgentClaude  Agent = "claude"
	AgentCodex   Agent = "codex"
	AgentGemini  Agent = "gemini"
	AgentCursor  Agent = "cursor"
	AgentCopilot Agent = "copilot"
	// AgentOther is any other agent that reads AGENTS.md.
	AgentOther Agent = "other"
)

// AgentOptions returns the answers to "Which coding agents do you work
// with?".
func AgentOptions() []Option {
	return []Option{
		{Value: string(AgentClaude), Label: "Claude Code"},
		{Value: string(AgentCodex), Label: "Codex"},
		{Value: string(AgentGemini), Label: "Gemini CLI"},
		{Value: string(AgentCursor), Label: "Cursor"},
		{Value: string(AgentCopilot), Label: "GitHub Copilot"},
		{Value: string(AgentOther), Label: "Another agent that reads AGENTS.md"},
	}
}

// agentTargets maps each agent to the target of its own files.
var agentTargets = map[Agent]spec.Target{
	AgentClaude:  spec.TargetClaude,
	AgentCodex:   spec.TargetAgentsMD,
	AgentGemini:  spec.TargetGemini,
	AgentCursor:  spec.TargetCursor,
	AgentCopilot: spec.TargetCopilot,
	AgentOther:   spec.TargetAgentsMD,
}

// targetOrder is the order targets are written in the spec.
var targetOrder = []spec.Target{
	spec.TargetClaude, spec.TargetAgentsMD, spec.TargetGemini, spec.TargetCursor, spec.TargetCopilot,
}

// Targets returns the spec targets for the agents the user works with.
// Each agent gets its own target, and AGENTS.md is added unless Claude Code
// is the only agent: it is the shared main file the others import or
// mirror, and Cursor and GitHub Copilot already read it before cyanotype
// generates their own files.
func Targets(agents []Agent) []spec.Target {
	want := map[spec.Target]bool{}
	for _, a := range agents {
		if t, ok := agentTargets[a]; ok {
			want[t] = true
		}
		if a != AgentClaude {
			want[spec.TargetAgentsMD] = true
		}
	}
	var targets []spec.Target
	for _, t := range targetOrder {
		if want[t] {
			targets = append(targets, t)
		}
	}
	return targets
}

// Candidates returns the presets that fit a project type, narrowed to
// those written in the language the user knows when there are any. A
// language without a preset of the type, LanguageOther or LanguageNone
// leaves every preset of the type.
func Candidates(c *catalog.Catalog, presets []preset.Preset, t preset.ProjectType, language string) []preset.Preset {
	var ofType, inLanguage []preset.Preset
	for _, p := range presets {
		if p.ProjectType != t {
			continue
		}
		ofType = append(ofType, p)
		if slices.Contains(p.Languages(c), language) {
			inLanguage = append(inLanguage, p)
		}
	}
	if len(inLanguage) > 0 {
		return inLanguage
	}
	return ofType
}

// OffersLearning reports whether simple mode offers learning mode: only to
// users who do not know a language of the preset yet. It is never turned on
// by default, because it makes the agent slower by design.
func OffersLearning(c *catalog.Catalog, p preset.Preset, language string) bool {
	return !slices.Contains(p.Languages(c), language)
}

// CanonicalLocale rewrites a locale tag the way spec expects it: "pt_br"
// and "PT-br" become "pt-BR", "zh-hant" becomes "zh-Hant". It does not
// check the result; spec.ValidLocale does.
func CanonicalLocale(tag string) string {
	parts := strings.Split(strings.ReplaceAll(strings.TrimSpace(tag), "_", "-"), "-")
	for i, p := range parts {
		switch {
		case i == 0:
			parts[i] = strings.ToLower(p)
		case len(p) == 4:
			parts[i] = strings.ToUpper(p[:1]) + strings.ToLower(p[1:])
		default:
			parts[i] = strings.ToUpper(p)
		}
	}
	return strings.Join(parts, "-")
}

// CanonicalLanguage rewrites a human language name the way spec expects
// it: "Brazilian Portuguese" becomes "brazilian-portuguese".
func CanonicalLanguage(name string) string {
	return strings.Join(strings.Fields(strings.ToLower(name)), "-")
}

// Answers holds every answer of simple mode.
type Answers struct {
	// Name is the project name; interfaces default it to the folder name.
	Name string
	// Goal describes the project in one or two sentences.
	Goal string
	// Learning is a value of LearningOptions; empty means LearningOff.
	Learning string
	Autonomy Autonomy
	I18n     I18n
	// Locales lists the app's locales, the default one first. With
	// I18nLater only the first is used, DefaultLocale when there is none.
	Locales []string
	// AgentReplies is the human language the agent replies in, such as
	// "italian"; empty means the spec default, English.
	AgentReplies string
	Agents       []Agent
}

// Stack is the technology part of a spec: the entries, and the commands
// and output settings that go with them. A preset gives it to a new
// project, detection to an existing one; the rest of simple mode is the
// same for both.
type Stack struct {
	// Selections are entry ids in the order they go into the spec.
	Selections []string
	// I18nSelections are the entries added after preset.GenericI18n when
	// the app is multilingual, such as the i18n library of the framework.
	I18nSelections []string
	Commands       spec.Commands
	Output         spec.Output
}

// PresetStack returns the stack of a preset. The result shares no slices
// with p.
func PresetStack(p preset.Preset) Stack {
	return Stack{
		Selections:     slices.Clone(p.Selections),
		I18nSelections: slices.Clone(p.I18nSelections),
		Commands:       p.Commands,
		Output:         p.Output,
	}
}

// Build returns the spec simple mode writes for a preset and the answers:
// the preset's stack, then the i18n entries, the rules of the autonomy level
// and learning mode, with the catalog version pinned. Invalid answers are
// errors, joined with errors.Join; Build does not resolve the spec against
// the catalog, which callers do before saving it.
func Build(c *catalog.Catalog, p preset.Preset, a Answers) (spec.Spec, error) {
	return BuildStack(c, PresetStack(p), a)
}

// BuildStack is Build for any stack, such as one detection found. Entries
// the answers add that the stack already selects, such as
// conventional-commits detected from a commitlint config, are not added
// twice.
func BuildStack(c *catalog.Catalog, st Stack, a Answers) (spec.Spec, error) {
	var errs []error
	fail := func(format string, args ...any) { errs = append(errs, fmt.Errorf(format, args...)) }

	if strings.TrimSpace(a.Name) == "" {
		fail("the project name is required")
	}
	if strings.TrimSpace(a.Goal) == "" {
		fail("the project goal is required")
	}
	if !slices.Contains([]Autonomy{AsksOften, Balanced, Autonomous}, a.Autonomy) {
		fail("unknown autonomy %q", a.Autonomy)
	}
	if len(a.Agents) == 0 {
		fail("choose at least one agent")
	}
	for _, ag := range a.Agents {
		if _, ok := agentTargets[ag]; !ok {
			fail("unknown agent %q", ag)
		}
	}
	learning := ""
	if a.Learning != "" && a.Learning != LearningOff {
		learning = learningPrefix + a.Learning
		if e, ok := c.Get(learning); !ok || e.Category != learningCategory {
			fail("unknown learning mode %q", a.Learning)
		}
	}
	var i18n *spec.I18n
	switch a.I18n {
	case "", I18nNo:
	case I18nLater:
		i18n = &spec.I18n{DefaultLocale: DefaultLocale}
		if len(a.Locales) > 0 {
			i18n.DefaultLocale = a.Locales[0]
		}
	case I18nYes:
		if len(a.Locales) < 2 {
			fail("a multilingual app needs at least two locales, the default one first, such as en,it")
		} else {
			i18n = &spec.I18n{DefaultLocale: a.Locales[0], Locales: slices.Clone(a.Locales)}
		}
	default:
		fail("unknown translation choice %q", a.I18n)
	}
	if len(errs) > 0 {
		return spec.Spec{}, errors.Join(errs...)
	}

	s := spec.Spec{
		Version:        spec.CurrentVersion,
		CatalogVersion: c.Version(),
		Targets:        Targets(a.Agents),
		Mode:           spec.ModeSimple,
		Project:        spec.Project{Name: strings.TrimSpace(a.Name), Goal: strings.TrimSpace(a.Goal)},
		Commands:       st.Commands,
		Selections:     slices.Clone(st.Selections),
		Output:         st.Output,
	}
	add := func(ids ...string) {
		for _, id := range ids {
			if !slices.Contains(s.Selections, id) {
				s.Selections = append(s.Selections, id)
			}
		}
	}
	if i18n != nil {
		s.I18n = i18n
		add(preset.GenericI18n)
		add(st.I18nSelections...)
	}
	add(AutonomySelections(a.Autonomy)...)
	if learning != "" {
		add(learning)
	}
	// Defaults are never written to the spec, so English stays implicit.
	if replies := CanonicalLanguage(a.AgentReplies); replies != spec.DefaultLanguage {
		s.Languages.AgentReplies = replies
	}
	return s, nil
}
