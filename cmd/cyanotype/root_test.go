package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRootCmd(t *testing.T) {
	tests := []struct {
		name         string
		version      string
		args         []string
		wantOut      string // exact output; checked when wantContains is empty
		wantContains []string
		wantErr      bool
	}{
		{
			name:    "version prints injected version",
			version: "v1.2.3",
			args:    []string{"version"},
			wantOut: "cyanotype v1.2.3\n",
		},
		{
			name:    "version prints default dev",
			version: "dev",
			args:    []string{"version"},
			wantOut: "cyanotype dev\n",
		},
		{
			name:    "version rejects arguments",
			version: "dev",
			args:    []string{"version", "extra"},
			wantErr: true,
		},
		{
			name:         "help lists version command",
			version:      "dev",
			args:         []string{"--help"},
			wantContains: []string{"Usage:", "cyanotype", "version"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			cmd := newRootCmd(tt.version)
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			cmd.SetArgs(tt.args)

			err := cmd.Execute()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Execute() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			got := out.String()
			if len(tt.wantContains) == 0 {
				if got != tt.wantOut {
					t.Errorf("output = %q, want %q", got, tt.wantOut)
				}
				return
			}
			for _, s := range tt.wantContains {
				if !strings.Contains(got, s) {
					t.Errorf("output missing %q:\n%s", s, got)
				}
			}
		})
	}
}
