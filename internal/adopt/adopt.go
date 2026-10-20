// Package adopt handles the instruction files a project already has when
// cyanotype first generates its own: a CLAUDE.md or AGENTS.md written by
// hand, without the cyanotype header.
//
// Such a file is never replaced silently. The user chooses one of four
// things to do with it, and every interface offers the same four: import
// its rules into cyanotype.yaml with the user's coding agent, keep it whole
// in a protected block, move it to a file of its own that the main file
// points to, or replace it, only when git can give it back. Like every
// internal package it never prints or reads input: asking belongs to the
// interfaces. See docs/DESIGN.md, Hand-written instruction files.
package adopt

import (
	"fmt"
	"strings"
)

// Choice is what to do with a hand-written instruction file.
type Choice string

// The choices, in the order interfaces offer them: the recommended one
// first, the only one that can lose text last.
const (
	// Import prints a prompt for the user's coding agent that converts the
	// file into cyanotype.yaml. Nothing is written.
	Import Choice = "import"
	// Keep wraps the whole file in a cyanotype:custom block, which
	// regeneration keeps.
	Keep Choice = "keep"
	// Move puts the file's content in a file of its own, which the main
	// instruction file tells the agent to read.
	Move Choice = "move"
	// Replace overwrites the file with the generated one. It is allowed
	// only when git holds the file as it is, so it can always be recovered.
	Replace Choice = "replace"
)

// Choices returns every choice, in the order interfaces offer them.
func Choices() []Choice {
	return []Choice{Import, Keep, Move, Replace}
}

// ParseChoice returns the choice named s, such as "keep".
func ParseChoice(s string) (Choice, error) {
	for _, c := range Choices() {
		if string(c) == s {
			return c, nil
		}
	}
	names := make([]string, len(Choices()))
	for i, c := range Choices() {
		names[i] = string(c)
	}
	return "", fmt.Errorf("%q is not a choice for an existing instruction file; choose %s", s,
		strings.Join(names[:len(names)-1], ", ")+" or "+names[len(names)-1])
}

// Writes reports whether the choice lets generation write the file's path.
// Import does not: the agent writes cyanotype.yaml, and the file stays as
// it is until the user regenerates.
func (c Choice) Writes() bool {
	return c == Keep || c == Move || c == Replace
}
