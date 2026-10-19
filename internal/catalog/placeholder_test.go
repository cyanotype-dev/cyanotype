package catalog

import (
	"reflect"
	"testing"
)

func TestUsedPlaceholders(t *testing.T) {
	tests := []struct {
		text string
		want []string
	}{
		{"No placeholders here.", nil},
		{"Learning {languages}.", []string{"languages"}},
		{"{a} then {b} then {a} again.", []string{"a", "b"}},
		{"Code `{children}` stays literal, {outside} does not.", []string{"outside"}},
		{"```\nfunc() {x}\n```\nAfter {fence}.", []string{"fence"}},
		{"Not names: {} {Upper} {1x} { spaced }.", nil},
	}
	for _, tt := range tests {
		if got := UsedPlaceholders(tt.text); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("UsedPlaceholders(%q) = %q, want %q", tt.text, got, tt.want)
		}
	}
}

func TestFillPlaceholders(t *testing.T) {
	values := map[string]string{"languages": "Go and TypeScript"}
	tests := []struct {
		text, want string
	}{
		{"Learning {languages}.", "Learning Go and TypeScript."},
		{"{languages}, again {languages}.", "Go and TypeScript, again Go and TypeScript."},
		{"Keep `{languages}` and {other}.", "Keep `{languages}` and {other}."},
	}
	for _, tt := range tests {
		if got := FillPlaceholders(tt.text, values); got != tt.want {
			t.Errorf("FillPlaceholders(%q) = %q, want %q", tt.text, got, tt.want)
		}
	}
}
