package contract

// Validate is the output of cyanotype validate --json and of the MCP
// validate tool: whether the spec resolves against the catalog, and how
// long the generated files would be.
//
// It is the one output whose errors are its result: a spec with errors is
// invalid, Valid is false, and Errors lists every problem with its field
// and line. Valid is omitted only when no spec could be read at all.
type Validate struct {
	Head
	// Valid reports that the spec has no errors; warnings are allowed.
	Valid *bool `json:"valid,omitzero"`
	// Files lists every generated Markdown file with its length, when the
	// spec is valid.
	Files []FileLines `json:"files,omitzero"`
	// MaxLines is output.max_lines, the length above which the main file
	// is split or gets a warning.
	MaxLines int `json:"max_lines,omitzero"`
	Diagnostics
}

// FileLines is a generated Markdown file and its number of lines.
type FileLines struct {
	Path  string `json:"path"`
	Lines int    `json:"lines"`
}
