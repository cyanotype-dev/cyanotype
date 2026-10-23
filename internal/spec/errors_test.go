package spec

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestValidationErrorString(t *testing.T) {
	tests := []struct {
		name string
		err  ValidationError
		want string
	}{
		{"field and line", ValidationError{Field: "mode", Line: 3, Msg: "bad"}, "line 3: mode: bad"},
		{"no line", ValidationError{Field: "targets", Msg: "is required"}, "targets: is required"},
		{"whole file", ValidationError{Msg: "file is empty"}, "file is empty"},
		{"syntax", ValidationError{Line: 7, Msg: "invalid YAML: x", Syntax: true}, "near line 7: invalid YAML: x"},
		{"syntax without line", ValidationError{Msg: "invalid YAML: x", Syntax: true}, "invalid YAML: x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestSyntaxMarked checks that only YAML syntax errors are marked as such,
// since only their lines may be one off.
func TestSyntaxMarked(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"unclosed flow sequence", "version: 1\ntargets: [claude\nproject: x\n", true},
		{"bad indentation", "version: 1\n targets: [claude]\nproject: x\n", true},
		{"unknown field", "version: 1\ntargets: [claude]\nproject:\n  name: a\n  goal: b\ntags: [x]\n", false},
		{"wrong type", "version: 1\ntargets: claude\nproject:\n  name: a\n  goal: b\n", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Load(strings.NewReader(tt.input))
			if err == nil {
				t.Fatal("Load() succeeded, want an error")
			}
			for _, ve := range validationErrors(t, err) {
				if ve.Syntax != tt.want {
					t.Errorf("error %q has Syntax = %v, want %v", ve, ve.Syntax, tt.want)
				}
			}
		})
	}
}

// codes returns the Code of each ValidationError joined in err.
func codes(t *testing.T, err error) []string {
	t.Helper()
	errs := []error{err}
	var joined interface{ Unwrap() []error }
	if errors.As(err, &joined) {
		errs = joined.Unwrap()
	}
	out := make([]string, len(errs))
	for i, e := range errs {
		var ve *ValidationError
		if !errors.As(e, &ve) {
			t.Fatalf("error %q is not a *ValidationError", e)
		}
		out[i] = ve.Code
	}
	return out
}

// TestErrorCodes checks the stable code of every kind of problem Load
// reports, which the JSON output exposes to scripts and agents.
func TestErrorCodes(t *testing.T) {
	const valid = "version: 1\ntargets: [claude]\nproject:\n  name: a\n  goal: b\n"
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"syntax", "version: 1\ntargets: [claude\n", []string{CodeYAMLSyntax}},
		{"empty file", "", []string{CodeEmptyFile}},
		{"two documents", valid + "---\nversion: 1\n", []string{CodeMultipleDocuments}},
		{"not a mapping", "- a\n- b\n", []string{CodeNotMapping}},
		{"alias", valid + "answers:\n  domain_notes: &n x\n", []string{CodeAlias}},
		{"missing version", "targets: [claude]\n", []string{CodeRequired}},
		{"version not an integer", "version: one\n", []string{CodeWrongType}},
		{"version zero", "version: 0\n", []string{CodeInvalidValue}},
		{"newer version", "version: 99\n", []string{CodeUnsupportedVersion}},
		{"unknown field", valid + "tags: [x]\n", []string{CodeUnknownField}},
		{"wrong type", "version: 1\ntargets: claude\nproject:\n  name: a\n  goal: b\n", []string{CodeWrongType}},
		{"duplicate key", valid + "mode: simple\nmode: advanced\n", []string{CodeDuplicate}},
		{"required", "version: 1\ntargets: [claude]\nproject:\n  name: a\n", []string{CodeRequired}},
		{"enum", valid + "mode: expert\n", []string{CodeInvalidValue}},
		{"duplicate item", valid + "selections: [go, go]\n", []string{CodeDuplicate}},
		{"empty item", valid + "selections: [go, \"\"]\n", []string{CodeEmpty}},
		{"empty command", valid + "commands:\n  dev: \" \"\n", []string{CodeEmpty}},
		{"bad format", valid + "selections: [Go]\n", []string{CodeInvalidValue}},
		{"locked not selected", valid + "selections: [go]\nlocked: [node]\n", []string{CodeNotSelected}},
		{"path outside the project", valid + "output:\n  topic_dir: ../x\n", []string{CodeInvalidValue}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Load(strings.NewReader(tt.input))
			if err == nil {
				t.Fatal("Load() succeeded, want an error")
			}
			if got := codes(t, err); !slices.Equal(got, tt.want) {
				t.Errorf("codes = %q, want %q\nerror: %v", got, tt.want, err)
			}
		})
	}
	big := strings.NewReader(strings.Repeat("#", maxFileSize+1))
	if _, err := Load(big); !slices.Equal(codes(t, err), []string{CodeTooLarge}) {
		t.Errorf("a file over the limit: codes = %q, want %q", codes(t, err), CodeTooLarge)
	}
}
