package adopt

import (
	"bytes"
	"fmt"
	"slices"

	"github.com/cyanotype-dev/cyanotype/internal/catalog"
	"github.com/cyanotype-dev/cyanotype/internal/detect"
	"github.com/cyanotype-dev/cyanotype/internal/preset"
	"github.com/cyanotype-dev/cyanotype/internal/simple"
	"github.com/cyanotype-dev/cyanotype/internal/spec"
)

// DraftInput is what Draft needs to know about a project without a spec.
type DraftInput struct {
	// HandWritten are the instruction files written by hand, relative to
	// the project root. CLAUDE.md and AGENTS.md among them decide the
	// targets when Agents is empty.
	HandWritten []string
	// Agents are the agents to generate for, as chosen by the user; empty
	// means the agents of the hand-written files.
	Agents []simple.Agent
	// Name is the project name.
	Name string
	// Found is what detection found. Leave it empty to start the draft
	// without a stack.
	Found detect.Result
	// Presets order the detected stack and fill in its commands.
	Presets []preset.Preset
	// With and Without adjust the detected stack, as init's --with and
	// --without do.
	With, Without []string
}

// Draft returns the cyanotype.yaml cyanotype can fill in for the import
// prompt before any question: the targets, the project name, and the
// detected stack with its commands. It also returns the stack, which
// decides the catalog entries the prompt lists.
//
// It lives here, not in an interface, because init and the MCP server's
// import_instructions prompt must hand the agent the same draft.
func Draft(c *catalog.Catalog, in DraftInput) (draft []byte, stack []string, err error) {
	var targets []spec.Target
	if len(in.Agents) > 0 {
		targets = simple.Targets(in.Agents)
	} else {
		for _, t := range []struct {
			file   string
			target spec.Target
		}{{"CLAUDE.md", spec.TargetClaude}, {"AGENTS.md", spec.TargetAgentsMD}} {
			if slices.Contains(in.HandWritten, t.file) {
				targets = append(targets, t.target)
			}
		}
	}
	// The goal is required and only the files can tell it, so the draft
	// holds a placeholder the prompt asks the agent to replace.
	s := spec.Spec{Version: spec.CurrentVersion, CatalogVersion: c.Version(), Targets: targets,
		Project: spec.Project{Name: in.Name, Goal: GoalPlaceholder}}
	if !in.Found.Empty() {
		stack = in.Found.IDs()
		if d, err := simple.DetectStack(c, in.Presets, in.Found, in.With, in.Without); err == nil {
			stack, s.Commands = d.Selections, d.Commands
		}
		s.Selections = stack
	}
	var buf bytes.Buffer
	if err := spec.New(s).Save(&buf); err != nil {
		return nil, nil, fmt.Errorf("write the draft spec: %w", err)
	}
	return buf.Bytes(), stack, nil
}
