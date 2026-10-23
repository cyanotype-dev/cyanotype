package spec

// CurrentVersion is the version of the spec format this build reads and
// writes. Older versions are migrated on load; newer ones are rejected.
// It only changes for breaking changes: new optional fields keep it as is.
const CurrentVersion = 1

// Default values applied by WithDefaults. They are never written to the
// file, so a spec only records what the user actually chose.
const (
	// DefaultMode is advanced because a file without a mode was written by
	// hand, and opening it in simple mode could replace its selections.
	DefaultMode           = ModeAdvanced
	DefaultLanguage       = "english"
	DefaultTranslationsBy = TranslationsAgentDraft
	DefaultSplit          = SplitAuto
	DefaultMaxLines       = 100
	DefaultTopicDir       = "docs/agents"
)

// Target is an agent whose instruction files are generated.
type Target string

// Allowed values for Target, one per renderer.
const (
	TargetClaude   Target = "claude"    // CLAUDE.md
	TargetAgentsMD Target = "agents-md" // AGENTS.md
	TargetGemini   Target = "gemini"    // GEMINI.md
	TargetCursor   Target = "cursor"    // .cursor/rules/*.mdc
	TargetCopilot  Target = "copilot"   // .github/copilot-instructions.md
)

// Mode is the interface mode the choices were last made in.
type Mode string

// Allowed values for Mode.
const (
	ModeSimple   Mode = "simple"
	ModeAdvanced Mode = "advanced"
)

// TranslationsBy says who translates the app's user-facing text.
type TranslationsBy string

// Allowed values for TranslationsBy.
const (
	// TranslationsAgentDraft lets the agent draft every locale.
	TranslationsAgentDraft TranslationsBy = "agent-draft"
	// TranslationsAgentDefaultOnly lets the agent write the default locale
	// only and mark the other locales as untranslated.
	TranslationsAgentDefaultOnly TranslationsBy = "agent-default-only"
	// TranslationsHuman leaves translation to human translators.
	TranslationsHuman TranslationsBy = "human"
)

// Split says when the output is split into a main file plus topic files.
type Split string

// Allowed values for Split.
const (
	// SplitAuto splits only when the main file would exceed max_lines.
	SplitAuto   Split = "auto"
	SplitAlways Split = "always"
	SplitNever  Split = "never"
)

// Spec is the content of cyanotype.yaml: the user's choices, never the
// generated text. A zero value means "not set"; WithDefaults fills in the
// defaults. Fields whose yaml tag has no omitempty are required.
// See the Project spec section of docs/DESIGN.md.
type Spec struct {
	Version int `yaml:"version" json:"version"`
	// CatalogVersion pins the catalog the spec was generated with, such as
	// "2026.10", so regeneration is reproducible. Empty means the catalog
	// bundled with this build.
	CatalogVersion string   `yaml:"catalog_version,omitempty" json:"catalog_version,omitempty"`
	Targets        []Target `yaml:"targets" json:"targets"`
	Mode           Mode     `yaml:"mode,omitempty" json:"mode,omitempty"`
	Project        Project  `yaml:"project" json:"project"`
	// Languages sets the human language of each kind of text the agent writes.
	Languages Languages `yaml:"languages,omitempty" json:"languages,omitzero"`
	// Commands holds the project's own commands, which the main instruction
	// file lists so the agent never has to guess how to build or test.
	Commands Commands `yaml:"commands,omitempty" json:"commands,omitzero"`
	// Selections holds catalog entry ids. Order matters: it is the order in
	// which entries appear in generated text, such as {languages}.
	Selections []string `yaml:"selections,omitempty" json:"selections,omitempty"`
	// Locked lists selected ids that a personal cyanotype.local.yaml may not
	// weaken or remove.
	Locked []string `yaml:"locked,omitempty" json:"locked,omitempty"`
	// I18n configures translations in the app being built; nil when the
	// project has no translation settings.
	I18n *I18n `yaml:"i18n,omitempty" json:"i18n,omitempty"`
	// Answers holds free-text answers to open questions, keyed by question
	// id, such as domain_notes.
	Answers     map[string]string `yaml:"answers,omitempty" json:"answers,omitempty"`
	CustomRules []CustomRule      `yaml:"custom_rules,omitempty" json:"custom_rules,omitempty"`
	Output      Output            `yaml:"output,omitempty" json:"output,omitzero"`
}

// Project is the short summary of what is being built. It goes into the
// main instruction file, because the agent needs it in every session.
type Project struct {
	Name         string   `yaml:"name" json:"name"`
	Goal         string   `yaml:"goal" json:"goal"`
	Users        string   `yaml:"users,omitempty" json:"users,omitempty"`
	CoreFeatures []string `yaml:"core_features,omitempty" json:"core_features,omitempty"`
	OutOfScope   []string `yaml:"out_of_scope,omitempty" json:"out_of_scope,omitempty"`
	// Brief is the path of the full product description, relative to the
	// project root, such as docs/PRODUCT.md.
	Brief string `yaml:"brief,omitempty" json:"brief,omitempty"`
}

// Languages holds human language names, such as "english" or "italian".
type Languages struct {
	Code         string `yaml:"code,omitempty" json:"code,omitempty"`
	Comments     string `yaml:"comments,omitempty" json:"comments,omitempty"`
	Docs         string `yaml:"docs,omitempty" json:"docs,omitempty"`
	Commits      string `yaml:"commits,omitempty" json:"commits,omitempty"`
	AgentReplies string `yaml:"agent_replies,omitempty" json:"agent_replies,omitempty"`
}

// Commands holds the shell commands that run the project, such as
// "pnpm dev" or "go test ./...". Each one is optional. The fields are fixed,
// rather than a free map, so a typo in a key is an unknown field and the
// generated file always lists them in the same order.
type Commands struct {
	Dev   string `yaml:"dev,omitempty" json:"dev,omitempty"`
	Build string `yaml:"build,omitempty" json:"build,omitempty"`
	Test  string `yaml:"test,omitempty" json:"test,omitempty"`
	Lint  string `yaml:"lint,omitempty" json:"lint,omitempty"`
}

// I18n holds the translation settings of the app being built. Locales are
// BCP 47 tags such as "en" or "pt-BR".
type I18n struct {
	DefaultLocale string `yaml:"default_locale" json:"default_locale"`
	// Locales lists every supported locale, including the default one.
	Locales        []string       `yaml:"locales,omitempty" json:"locales,omitempty"`
	TranslationsBy TranslationsBy `yaml:"translations_by,omitempty" json:"translations_by,omitempty"`
}

// CustomRule is a rule the catalog cannot know, with the reason behind it.
type CustomRule struct {
	Rule string `yaml:"rule" json:"rule"`
	Why  string `yaml:"why" json:"why"`
	// Locked stops a personal cyanotype.local.yaml from overriding the rule.
	Locked bool `yaml:"locked,omitempty" json:"locked,omitempty"`
}

// Output controls how generated files are laid out.
type Output struct {
	Split Split `yaml:"split,omitempty" json:"split,omitempty"`
	// MaxLines is the main file length above which SplitAuto splits.
	MaxLines int `yaml:"max_lines,omitempty" json:"max_lines,omitempty"`
	// TopicDir is the folder for topic files, relative to the project root.
	TopicDir string `yaml:"topic_dir,omitempty" json:"topic_dir,omitempty"`
}

// WithDefaults returns a copy of s with every unset optional value replaced
// by its default. Code that generates files reads the result; the spec
// itself keeps the zero values so Save does not write defaults to the file.
// The copy shares slices and maps with s.
func (s Spec) WithDefaults() Spec {
	if s.Mode == "" {
		s.Mode = DefaultMode
	}
	for _, lang := range []*string{
		&s.Languages.Code, &s.Languages.Comments, &s.Languages.Docs,
		&s.Languages.Commits, &s.Languages.AgentReplies,
	} {
		if *lang == "" {
			*lang = DefaultLanguage
		}
	}
	if s.I18n != nil {
		i18n := *s.I18n // copy, so s.I18n is left untouched
		if len(i18n.Locales) == 0 && i18n.DefaultLocale != "" {
			i18n.Locales = []string{i18n.DefaultLocale}
		}
		if i18n.TranslationsBy == "" {
			i18n.TranslationsBy = DefaultTranslationsBy
		}
		s.I18n = &i18n
	}
	if s.Output.Split == "" {
		s.Output.Split = DefaultSplit
	}
	if s.Output.MaxLines == 0 {
		s.Output.MaxLines = DefaultMaxLines
	}
	if s.Output.TopicDir == "" {
		s.Output.TopicDir = DefaultTopicDir
	}
	return s
}
