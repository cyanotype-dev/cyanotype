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
	// Requires and Conflicts hold plain entry ids. Conflicts only links
	// entries of different categories and must be declared on both sides.
	Requires  []string `yaml:"requires,omitempty"`
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

// Enforce lists the real constraints generated alongside the snippet.
type Enforce struct {
	Claude *ClaudeEnforce `yaml:"claude,omitempty"`
}

// ClaudeEnforce holds constraints written to Claude Code settings.
type ClaudeEnforce struct {
	// Deny holds permission rules added to the deny list.
	Deny []string `yaml:"deny,omitempty"`
}

// Detect lists what reveals the entry in an existing project.
type Detect struct {
	// Files are paths relative to the project root.
	Files []string `yaml:"files,omitempty"`
}
