package catalog

import "testing"

func TestValidVersion(t *testing.T) {
	tests := []struct {
		v    string
		want bool
	}{
		{"2026.10", true},
		{"2026.01", true},
		{"2026.10.1", true},
		{"2026.10.12", true},
		{"2026.1", false},
		{"2026.13", false},
		{"2026.00", false},
		{"2026.10.0", false},
		{"2026.10.01", false},
		{"26.10", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := ValidVersion(tt.v); got != tt.want {
			t.Errorf("ValidVersion(%q) = %v, want %v", tt.v, got, tt.want)
		}
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"2026.10", "2026.10", 0},
		{"2026.10", "2026.10.0", 0},
		{"2026.09", "2026.10", -1},
		{"2026.10", "2026.09", 1},
		{"2026.10", "2026.10.1", -1},
		{"2026.10.2", "2026.10.10", -1},
		{"2027.01", "2026.12", 1},
	}
	for _, tt := range tests {
		if got := CompareVersions(tt.a, tt.b); got != tt.want {
			t.Errorf("CompareVersions(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}
