package main

import (
	"strings"
	"testing"
)

// handWritten is a CLAUDE.md written before cyanotype.
const handWritten = "# Shop\n\nUse pnpm, never npm.\nRun pnpm test before every commit.\n"

// claudeSpec targets Claude Code only, so CLAUDE.md is the main file.
const claudeSpec = `version: 1
targets: [claude]
project:
  name: shop
  goal: An online shop.
selections: [conventional-commits]
`

// TestGenerateNeverReplacesHandWritten checks that --yes does not replace
// an instruction file cyanotype did not generate.
func TestGenerateNeverReplacesHandWritten(t *testing.T) {
	dir := newProject(t, map[string]string{"cyanotype.yaml": claudeSpec, "CLAUDE.md": handWritten})
	r := runCLI(t, nonInteractive, failReader{t}, "generate", "--dir", dir, "--yes")
	if r.code == codeOK || r.code == codeNotWritten {
		t.Fatalf("exit code %d; want a refusal", r.code)
	}
	if !strings.Contains(r.stderr, "CLAUDE.md has no cyanotype header") {
		t.Errorf("stderr does not explain the refusal:\n%s", r.stderr)
	}
	files := tree(t, dir)
	if files["CLAUDE.md"] != handWritten {
		t.Errorf("CLAUDE.md was changed to:\n%s", files["CLAUDE.md"])
	}
	if len(files) != 2 {
		t.Errorf("files = %q; nothing should be written", keys(files))
	}
}
