package spec

import (
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
