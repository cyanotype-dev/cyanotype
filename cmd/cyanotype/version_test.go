package main

import (
	"runtime/debug"
	"testing"
)

// TestResolveVersion checks which version is reported for release builds,
// go install builds and builds from a checkout.
func TestResolveVersion(t *testing.T) {
	module := func(v string) *debug.BuildInfo {
		return &debug.BuildInfo{Main: debug.Module{Path: "github.com/cyanotype-dev/cyanotype", Version: v}}
	}
	tests := []struct {
		name    string
		ldflags string
		info    *debug.BuildInfo
		ok      bool
		want    string
	}{
		{"ldflags win over build info", "v0.1.0", module("v0.0.9"), true, "v0.1.0"},
		{"go install of a tag", "dev", module("v0.1.0"), true, "v0.1.0"},
		{"go install of a commit", "dev", module("v0.1.1-0.20261020120000-abcdef123456"), true, "v0.1.1-0.20261020120000-abcdef123456"},
		{"build from a checkout", "dev", module("(devel)"), true, "dev"},
		{"empty module version", "dev", module(""), true, "dev"},
		{"no build info", "dev", nil, false, "dev"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveVersion(tt.ldflags, tt.info, tt.ok); got != tt.want {
				t.Errorf("resolveVersion(%q) = %q, want %q", tt.ldflags, got, tt.want)
			}
		})
	}
}
