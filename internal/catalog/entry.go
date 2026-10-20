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
}

// Detect lists what reveals the entry in an existing project.
type Detect struct {
	// Files are paths relative to the project root.
	Files []string `yaml:"files,omitempty"`
}
