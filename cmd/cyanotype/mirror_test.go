package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestMirrorCommandsText checks the text output of the commands that
// mirror the MCP tools; their JSON output has golden files of its own.
func TestMirrorCommandsText(t *testing.T) {
	detected := map[string]string{
		"package.json":      `{"dependencies": {"next": "16.0.0"}, "scripts": {"dev": "next dev"}}`,
		"package-lock.json": "{}",
		"pnpm-lock.yaml":    "lockfileVersion: '9.0'\n",
	}
	tests := []struct {
		name     string
		files    map[string]string // nil: no project folder
		args     []string
		code     int
		stdout   []string // each must appear
		stderr   []string
		noStdout bool
	}{
		{
			name:   "catalog search",
			args:   []string{"catalog", "search", "package", "manager"},
			stdout: []string{"pnpm: pnpm (package-manager, radio). ", " Requires node.\n", "uv: uv (python-package-manager, radio).", "3 entries.\n"},
		},
		{
			name:   "catalog search with nothing found",
			args:   []string{"catalog", "search", "kubernetes"},
			stdout: []string{"No entries match.\n"},
		},
		{
			name: "catalog with a mistyped subcommand",
			args: []string{"catalog", "serch"},
			code: codeUsage,
		},
		{
			name:  "spec",
			files: map[string]string{"cyanotype.yaml": minimalSpec},
			args:  []string{"spec"},
			stdout: []string{
				"# cyanotype.yaml with its defaults applied; cyanotype.local.yaml is not supported yet; this is the team spec only.\n",
				"mode: advanced\n", "  agent_replies: english\n", "  topic_dir: docs/agents\n",
			},
		},
		{
			name:     "spec with an error",
			files:    map[string]string{"cyanotype.yaml": minimalSpec + "tags: [x]\n"},
			args:     []string{"spec"},
			code:     codeError,
			stderr:   []string{"cyanotype.yaml:7: error: tags: unknown field \"tags\"", "1 error.\n"},
			noStdout: true,
		},
		{
			name:   "detect",
			files:  detected,
			args:   []string{"detect"},
			stdout: []string{"  - Next.js (package.json: next)\n", "Only one of npm and pnpm can be selected", "Scripts in package.json: dev\n"},
		},
		{
			name:   "detect with an unreadable manifest",
			files:  map[string]string{"package.json": "{not json"},
			args:   []string{"detect"},
			stdout: []string{"  - Node.js (package.json)\n"},
			stderr: []string{"package.json: warning: invalid JSON"},
		},
		{
			name:   "detect with nothing found",
			files:  map[string]string{},
			args:   []string{"detect"},
			stdout: []string{"Nothing detected"},
		},
		{
			name:   "validate",
			files:  map[string]string{"cyanotype.yaml": minimalSpec},
			args:   []string{"validate"},
			stdout: []string{"  AGENTS.md: 18 lines\n", "cyanotype.yaml is valid; output.max_lines is 100.\n"},
		},
		{
			name:     "validate with errors",
			files:    map[string]string{"cyanotype.yaml": strings.Replace(minimalSpec, "[conventional-commits]", "[conventional-commits, pnpm]", 1)},
			args:     []string{"validate"},
			code:     codeError,
			stderr:   []string{`cyanotype.yaml:6: error: selections[1]: "pnpm" requires "node", which is not selected`, "1 error; ", "cyanotype.yaml is not valid.\n"},
			noStdout: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := tt.args
			var dir string
			if tt.files != nil {
				dir = newProject(t, tt.files)
				args = append(append([]string{}, args...), "--dir", dir)
			}
			r := runCLI(t, interactive, failReader{t}, args...)
			if r.code != tt.code {
				t.Fatalf("exit code %d, want %d\nstdout:\n%s\nstderr:\n%s\nerror: %v", r.code, tt.code, r.stdout, r.stderr, r.err)
			}
			for _, want := range tt.stdout {
				if !strings.Contains(r.stdout, want) {
					t.Errorf("stdout lacks %q:\n%s", want, r.stdout)
				}
			}
			// Paths in messages name the folder as given; leave it out.
			stderr := strings.ReplaceAll(r.stderr, dir+string(filepath.Separator), "")
			for _, want := range tt.stderr {
				if !strings.Contains(stderr, want) {
					t.Errorf("stderr lacks %q:\n%s", want, stderr)
				}
			}
			if tt.noStdout && r.stdout != "" {
				t.Errorf("stdout is not empty:\n%s", r.stdout)
			}
		})
	}
}
