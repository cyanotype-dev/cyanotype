package catalog

// Select says how an entry is chosen within its category.
type Select string

// Allowed values for Select.
const (
	// SelectRadio makes the entry exclusive within its category.
	SelectRadio Select = "radio"
	// SelectCheckbox lets the entry be combined with others in its category.
	SelectCheckbox Select = "checkbox"
)

// Level tells simple mode how much experience an entry assumes.
type Level string

// Allowed values for Level.
const (
	LevelBeginner     Level = "beginner"
	LevelIntermediate Level = "intermediate"
	LevelAdvanced     Level = "advanced"
)

// Entry is one selectable technology or rule in the catalog, decoded from a
// single YAML file. See the catalog section of docs/DESIGN.md.
type Entry struct {
	ID       string `yaml:"id"`
	Name     string `yaml:"name"`
	Category string `yaml:"category"`
	Select   Select `yaml:"select"`
	// URL is the target of the "i" icon; technologies have one, rules usually do not.
	URL     string `yaml:"url,omitempty"`
	Summary string `yaml:"summary"`
	// Requires, RequiresOneOf and Conflicts hold plain entry ids.
	//
	// Requires lists entries that must all be selected too.
	Requires []string `yaml:"requires,omitempty"`
	// RequiresOneOf holds groups of alternatives, such as [nextjs, vite-react]:
	// each group is satisfied when at least one of its entries is selected.
	// It exists because some entries work with any one of several others,
	// which Requires cannot express.
	RequiresOneOf [][]string `yaml:"requires_one_of,omitempty"`
	// Conflicts only links entries of different categories and must be
	// declared on both sides.
	Conflicts []string `yaml:"conflicts,omitempty"`
	Level     Level    `yaml:"level,omitempty"`
	// Topic names the split file the snippet goes to when output is split.
	Topic   string   `yaml:"topic,omitempty"`
	Snippet Snippet  `yaml:"snippet"`
	Enforce *Enforce `yaml:"enforce,omitempty"`
	Detect  *Detect  `yaml:"detect,omitempty"`
}

// Snippet is the rule an entry adds to generated files, with its reason.
type Snippet struct {
	Rule string `yaml:"rule"`
	Why  string `yaml:"why"`
}

// Enforce lists the real constraints generated alongside the snippet. Each
// key names where a constraint goes: an agent's settings, or a file of the
// repository that holds for everyone, humans included.
type Enforce struct {
	Claude *ClaudeEnforce `yaml:"claude,omitempty"`
	// Gitignore holds lines for the .gitignore block cyanotype owns, such
	// as ".env". They keep the order written here, because a negation such
	// as "!.env.example" only re-includes what the lines before it ignore.
	Gitignore []string `yaml:"gitignore,omitempty"`
}

// ClaudeEnforce holds constraints written to Claude Code settings.
type ClaudeEnforce struct {
	// Deny holds permission rules added to the deny list. A denied action
	// stays blocked even when the user would approve it.
	Deny []string `yaml:"deny,omitempty"`
	// Ask holds permission rules added to the ask list, for rules that mean
	// "ask first" rather than "never": Claude Code asks the user before the
	// action, so an approved change can still happen.
	Ask []string `yaml:"ask,omitempty"`
	// With holds rules that apply only when another entry, named by the
	// key, is selected too. It lets a stack entry carry the rules another
	// entry needs for that stack, such as pnpm's install command for
	// dependency-approval, so a project only gets the rules of the tools
	// it uses and the code needs no knowledge of either entry.
	With map[string]ClaudeConditional `yaml:"with,omitempty"`
}

// ClaudeConditional holds the rules of one key of ClaudeEnforce.With. Only
// ask rules are supported for now; conditional deny rules could be added
// here later without a new field.
type ClaudeConditional struct {
	// Ask holds permission rules added to the ask list when the key's
	// entry is selected. It is required: a key without rules does nothing.
	Ask []string `yaml:"ask"`
}

// Detect lists what reveals the entry in an existing project. Finding any
// one of its files or dependencies is enough. The names live here, in the
// catalog, so detecting a new stack needs no code change; the code only
// knows how to read each kind of manifest.
type Detect struct {
	// Files are paths relative to the project root.
	Files []string `yaml:"files,omitempty"`
	// Dependencies are package names, per ecosystem, that reveal the entry
	// when a manifest at the project root lists them.
	Dependencies *Dependencies `yaml:"dependencies,omitempty"`
	// RunScript says how this tool runs a script of package.json, such as
	// "pnpm {script}", with {script} standing for the script's name. Only
	// package managers have one: detection uses it to turn the scripts it
	// finds into the spec's commands, which differ between tools ("npm run
	// dev", "pnpm dev").
	RunScript string `yaml:"run_script,omitempty"`
}

// Dependencies holds package names per ecosystem. Each field is one
// ecosystem, so a misspelled one is an unknown field rather than a list
// detection silently never reads.
type Dependencies struct {
	// NPM holds package names read from package.json: dependencies,
	// devDependencies, optionalDependencies and peerDependencies.
	NPM []string `yaml:"npm,omitempty"`
	// PyPI holds project names read from pyproject.toml and
	// requirements.txt, in their normalized form (PEP 503): lowercase, with
	// hyphens for runs of "-", "_" and ".", so "Flask_SQLAlchemy" is
	// written "flask-sqlalchemy".
	PyPI []string `yaml:"pypi,omitempty"`
	// Go holds module paths read from the require directives of go.mod. A
	// path also matches its major versions and subpaths:
	// "github.com/jackc/pgx" matches "github.com/jackc/pgx/v5".
	Go []string `yaml:"go,omitempty"`
}

// ScriptPlaceholder is what RunScript holds in place of the script's name.
const ScriptPlaceholder = "{script}"
