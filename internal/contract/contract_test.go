package contract

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/cyanotype-dev/cyanotype/internal/adopt"
	"github.com/cyanotype-dev/cyanotype/internal/check"
	"github.com/cyanotype-dev/cyanotype/internal/generate"
	"github.com/cyanotype-dev/cyanotype/internal/lock"
	"github.com/cyanotype-dev/cyanotype/internal/render"
	"github.com/cyanotype-dev/cyanotype/internal/resolve"
	"github.com/cyanotype-dev/cyanotype/internal/spec"
)

func TestMarshalFailure(t *testing.T) {
	out := &Generate{Diagnostics: Diagnostics{Errors: []Diagnostic{Error(CodeNoSpec, "no cyanotype.yaml")}}}
	got, err := Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	want := `{
  "version": 1,
  "warnings": [],
  "errors": [
    {
      "code": "no-spec",
      "severity": "error",
      "message": "no cyanotype.yaml"
    }
  ]
}
`
	if string(got) != want {
		t.Errorf("Marshal() =\n%s\nwant\n%s", got, want)
	}
}

func TestMarshalDoesNotEscapeHTML(t *testing.T) {
	out := NewGenerate([]generate.Change{{Path: "AGENTS.md", Status: generate.Created, New: []byte("<!-- a & b -->\n")}}, nil)
	got, err := Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), `+<!-- a & b -->`) {
		t.Errorf("Marshal() escaped the diff:\n%s", got)
	}
	var back map[string]any
	if err := json.Unmarshal(got, &back); err != nil {
		t.Fatalf("Marshal() is not valid JSON: %v", err)
	}
}

func TestNewGenerate(t *testing.T) {
	changes := []generate.Change{
		{Path: "AGENTS.md", Status: generate.Modified, Old: []byte("# Mine\n"), New: []byte("# Gen\n"), HandWritten: true},
		{Path: "CLAUDE.md", Status: generate.Modified, Old: []byte("a\r\n"), New: []byte("b\n"), CRLF: true},
		{Path: "cyanotype.lock", Status: generate.Unchanged, Old: []byte("x\n"), New: []byte("x\n")},
		{Path: "docs/agents/git.md", Status: generate.Deleted, Old: []byte("old\n")},
		{Path: "docs/agents/original-claude.md", Status: generate.Created, New: []byte("mine\n"), MovedFrom: "CLAUDE.md"},
	}
	warnings := []resolve.Warning{{Code: resolve.CodeTooLong, Field: "output.max_lines", Line: 4, Msg: "too long"}}
	out := NewGenerate(changes, warnings)

	if want := (Summary{Created: 1, Modified: 2, Deleted: 1, Unchanged: 1}); *out.Summary != want {
		t.Errorf("Summary = %+v, want %+v", *out.Summary, want)
	}
	if want := []string{"AGENTS.md"}; !reflect.DeepEqual(out.WaitingForChoice, want) {
		t.Errorf("WaitingForChoice = %q, want %q", out.WaitingForChoice, want)
	}
	if out.Files[0].Diff != "" || !out.Files[0].HandWritten {
		t.Errorf("a hand-written file waiting for a choice = %+v, want no diff", out.Files[0])
	}
	if !out.Files[1].CRLFToLF || !strings.HasPrefix(out.Files[1].Diff, "--- a/CLAUDE.md\n+++ b/CLAUDE.md\n") {
		t.Errorf("a modified file = %+v", out.Files[1])
	}
	if out.Files[2].Diff != "" {
		t.Errorf("an unchanged file has a diff: %q", out.Files[2].Diff)
	}
	if !strings.Contains(out.Files[3].Diff, "+++ /dev/null") || out.Files[3].Status != "deleted" {
		t.Errorf("a deleted file = %+v", out.Files[3])
	}
	if out.Files[4].MovedFrom != "CLAUDE.md" {
		t.Errorf("a moved file = %+v", out.Files[4])
	}
	want := []Diagnostic{{Code: "too-long", Severity: "warning", File: "cyanotype.yaml", Line: 4, Field: "output.max_lines", Message: "too long"}}
	if !reflect.DeepEqual(out.Warnings, want) {
		t.Errorf("Warnings = %+v, want %+v", out.Warnings, want)
	}
	if out.Written == nil {
		t.Error("Written is nil, which would leave it out of a successful output")
	}
}

func TestWarningWithoutCode(t *testing.T) {
	got := Warnings([]resolve.Warning{{File: "docs/agents/git.md", Msg: "x"}})
	if got[0].Code != CodeSpecWarning || got[0].File != "docs/agents/git.md" {
		t.Errorf("Warnings() = %+v", got)
	}
}

func TestNewCheck(t *testing.T) {
	r := check.Report{
		Findings: []check.Finding{
			{Kind: check.Edited, Severity: check.Error, Path: "AGENTS.md", Msg: "was edited by hand"},
			{Kind: check.CatalogOlder, Severity: check.Warning, Path: "cyanotype.yaml", Line: 2, Field: "catalog_version", Msg: "older"},
		},
		Skipped: []string{"the check for tracked files: not a git repository"},
	}
	tests := []struct {
		name   string
		report check.Report
		strict bool
		want   CheckSummary
	}{
		{"errors", r, false, CheckSummary{Errors: 1, Warnings: 1, Drift: true}},
		{"warnings only", check.Report{Findings: r.Findings[1:]}, false, CheckSummary{Warnings: 1}},
		{"warnings only, strict", check.Report{Findings: r.Findings[1:]}, true, CheckSummary{Warnings: 1, Strict: true, Drift: true}},
		{"clean", check.Report{}, false, CheckSummary{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := NewCheck(tt.report, tt.strict)
			if *out.Summary != tt.want {
				t.Errorf("Summary = %+v, want %+v", *out.Summary, tt.want)
			}
			if out.Findings == nil || out.Skipped == nil {
				t.Error("Findings or Skipped is nil, which would leave it out of a check that ran")
			}
		})
	}
	out := NewCheck(r, false)
	want := Diagnostic{Code: "catalog-older", Severity: "warning", File: "cyanotype.yaml", Line: 2, Field: "catalog_version", Message: "older"}
	if out.Findings[1] != want {
		t.Errorf("Findings[1] = %+v, want %+v", out.Findings[1], want)
	}
}

func TestErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want Diagnostic
	}{
		{
			name: "spec error",
			err:  &spec.ValidationError{Code: "unknown-entry", Field: "selections[1]", Line: 9, Msg: `unknown entry "pnpn"`},
			want: Diagnostic{Code: "unknown-entry", Severity: "error", File: "cyanotype.yaml", Line: 9, Field: "selections[1]", Message: `unknown entry "pnpn"`},
		},
		{
			name: "syntax error",
			err:  &spec.ValidationError{Code: "yaml-syntax", Line: 7, Msg: "invalid YAML: x", Syntax: true},
			want: Diagnostic{Code: "yaml-syntax", Severity: "error", File: "cyanotype.yaml", Line: 7, LineApproximate: true, Message: "invalid YAML: x"},
		},
		{
			name: "markers",
			err:  fmt.Errorf("render AGENTS.md: %w", &render.MarkerError{File: "AGENTS.md", Line: 3, Msg: "never closed"}),
			want: Diagnostic{Code: "markers", Severity: "error", File: "AGENTS.md", Line: 3, Message: "never closed"},
		},
		{
			name: "no spec",
			err:  fmt.Errorf("%w in demo", generate.ErrNoSpec),
			want: Diagnostic{Code: "no-spec", Severity: "error", File: "cyanotype.yaml", Message: "no cyanotype.yaml in demo"},
		},
		{
			name: "invalid lock",
			err:  fmt.Errorf("%w: bad", lock.ErrInvalid),
			want: Diagnostic{Code: "lock-invalid", Severity: "error", File: "cyanotype.lock", Message: "cyanotype.lock is invalid: bad"},
		},
		{"newer lock", lock.ErrNewer, Diagnostic{Code: "lock-newer", Severity: "error", File: "cyanotype.lock", Message: lock.ErrNewer.Error()}},
		{"lock from a newer catalog", fmt.Errorf("x %w", render.ErrLockCatalogNewer),
			Diagnostic{Code: "lock-catalog-newer", Severity: "error", File: "cyanotype.lock", Message: "x " + render.ErrLockCatalogNewer.Error()}},
		{"hand-written", fmt.Errorf("refusing to replace AGENTS.md: %w", generate.ErrHandWritten),
			Diagnostic{Code: "hand-written", Severity: "error", Message: "refusing to replace AGENTS.md: it was not generated by cyanotype"}},
		{"replace", fmt.Errorf("x: %w", adopt.ErrNotRecoverable), Diagnostic{Code: "replace-refused", Severity: "error", Message: "x: git could not give it back"}},
		{"keep", fmt.Errorf("x %w", adopt.ErrKeepMarkers), Diagnostic{Code: "keep-refused", Severity: "error", Message: "x " + adopt.ErrKeepMarkers.Error()}},
		{"move", fmt.Errorf("x: %w", adopt.ErrMoveExists), Diagnostic{Code: "move-refused", Severity: "error", Message: "x: that file already exists"}},
		{"move at write", fmt.Errorf("x: %w", generate.ErrExists), Diagnostic{Code: "move-refused", Severity: "error", Message: "x: that path already exists"}},
		{"outside", fmt.Errorf("x: %w", generate.ErrOutsideRoot), Diagnostic{Code: "unsafe-path", Severity: "error", Message: "x: " + generate.ErrOutsideRoot.Error()}},
		{"symlink", fmt.Errorf("x: %w", generate.ErrSymlink), Diagnostic{Code: "unsafe-path", Severity: "error", Message: "x: it is a symbolic link"}},
		{"not regular", fmt.Errorf("x: %w", generate.ErrNotRegular), Diagnostic{Code: "unsafe-path", Severity: "error", Message: "x: it is not a regular file"}},
		{"changed", fmt.Errorf("x: %w", generate.ErrChanged), Diagnostic{Code: "changed-since-preview", Severity: "error", Message: "x: " + generate.ErrChanged.Error()}},
		{"anything else", errors.New("read AGENTS.md: permission denied"), Diagnostic{Code: "error", Severity: "error", Message: "read AGENTS.md: permission denied"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Errors(tt.err)
			if len(got) != 1 || got[0] != tt.want {
				t.Errorf("Errors() = %+v, want [%+v]", got, tt.want)
			}
		})
	}

	joined := errors.Join(&spec.ValidationError{Code: "required", Field: "targets", Msg: "is required"}, errors.New("other"))
	if got := Errors(joined); len(got) != 2 || got[0].Code != "required" || got[1].Code != "error" {
		t.Errorf("Errors(joined) = %+v, want one diagnostic per error", got)
	}
	if got := Errors(nil); got == nil || len(got) != 0 {
		t.Errorf("Errors(nil) = %#v, want an empty slice", got)
	}
}
