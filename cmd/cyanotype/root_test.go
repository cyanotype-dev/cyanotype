package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestUsageErrors checks that mistakes in commands and arguments exit with
// the usage code, 2, and say what the mistake is.
func TestUsageErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"unknown command", []string{"foo"}, `unknown command "foo" for "cyanotype"; run "cyanotype --help" to list the commands`},
		{"mistyped command", []string{"genrate"}, `unknown command "genrate" for "cyanotype"; did you mean "generate"?`},
		{"unknown command before a flag", []string{"foo", "--dir", "."}, `unknown command "foo" for "cyanotype"`},
		{"argument to version", []string{"version", "extra"}, `"cyanotype version" takes no arguments, got "extra"`},
		{"several arguments", []string{"generate", "a", "b"}, `"cyanotype generate" takes no arguments, got "a", "b"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := runCLI(t, nonInteractive, failReader{t}, tt.args...)
			if r.code != codeUsage || r.err == nil || !strings.Contains(r.err.Error(), tt.want) {
				t.Errorf("exit code %d, error %v; want %d and an error containing %q", r.code, r.err, codeUsage, tt.want)
			}
		})
	}
}

// TestRootWithoutArguments checks that cyanotype alone still shows the help
// and succeeds, now that the root command has a RunE of its own.
func TestRootWithoutArguments(t *testing.T) {
	r := runCLI(t, nonInteractive, failReader{t})
	if r.code != codeOK || r.err != nil || !strings.Contains(r.stdout, "Usage:") {
		t.Errorf("exit code %d, error %v; want 0 and the help, got:\n%s", r.code, r.err, r.stdout)
	}
}

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
