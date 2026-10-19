package resolve

import "testing"

func TestEditDistance(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"go", "", 2},
		{"", "go", 2},
		{"pnpm", "pnpm", 0},
		{"pnpn", "pnpm", 1},
		{"nextjs", "next-js", 1},
		{"postgress", "postgres", 1},
		{"kitten", "sitting", 3},
		{"ñandú", "nandu", 2},
	}
	for _, tt := range tests {
		if got := editDistance(tt.a, tt.b); got != tt.want {
			t.Errorf("editDistance(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestSuggest(t *testing.T) {
	candidates := []string{"go", "next-js", "nextra", "pnpm", "postgres", "uv"}
	tests := []struct {
		input string
		want  string // empty for no suggestion
	}{
		{"nextjs", "next-js"},
		{"pnpn", "pnpm"},
		{"postgress", "postgres"},
		{"gp", "go"},
		{"js", ""}, // two changes are too many for two letters
		{"kubernetes", ""},
		{"nextr", "nextra"},
		// "gv" is one change from both "go" and "uv"; the first candidate wins.
		{"gv", "go"},
	}
	for _, tt := range tests {
		got, ok := suggest(tt.input, candidates)
		if got != tt.want || ok != (tt.want != "") {
			t.Errorf("suggest(%q) = %q, %v; want %q", tt.input, got, ok, tt.want)
		}
	}
}
