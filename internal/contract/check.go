package contract

import "github.com/cyanotype-dev/cyanotype/internal/check"

// Check is the output of cyanotype check --json and of the MCP check tool.
// Warnings of the spec are findings here, as in the CLI's output, so the
// Warnings field of a check that ran is always empty.
type Check struct {
	Head
	// Findings are sorted by path, then line. Their code is the finding's
	// kind, such as "out-of-sync".
	Findings []Diagnostic `json:"findings,omitzero"`
	// Skipped names the checks that could not run, with the reason, such
	// as the one for tracked files outside a git repository.
	Skipped []string `json:"skipped,omitzero"`
	// Summary counts the findings and says whether they fail the check.
	Summary *CheckSummary `json:"summary,omitzero"`
	Diagnostics
}

// CheckSummary counts the findings of a check.
type CheckSummary struct {
	Errors   int `json:"errors"`
	Warnings int `json:"warnings"`
	// Strict reports that warnings fail the check too.
	Strict bool `json:"strict"`
	// Drift reports that the check failed: errors, or warnings with
	// Strict. The CLI then exits with code 3.
	Drift bool `json:"drift"`
}

// NewCheck returns the output for a report of check.Run.
func NewCheck(r check.Report, strict bool) *Check {
	out := &Check{Findings: make([]Diagnostic, 0, len(r.Findings)), Skipped: []string{}}
	for _, f := range r.Findings {
		out.Findings = append(out.Findings, Diagnostic{Code: string(f.Kind), Severity: f.Severity.String(),
			File: f.Path, Line: f.Line, Field: f.Field, Message: f.Msg})
	}
	out.Skipped = append(out.Skipped, r.Skipped...)
	errs, warns := r.Count(check.Error), r.Count(check.Warning)
	out.Summary = &CheckSummary{Errors: errs, Warnings: warns, Strict: strict, Drift: errs > 0 || (strict && warns > 0)}
	return out
}
