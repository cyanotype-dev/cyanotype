package adopt

import (
	"bytes"
	"slices"
	"testing"

	"github.com/cyanotype-dev/cyanotype/internal/detect"
	"github.com/cyanotype-dev/cyanotype/internal/simple"
	"github.com/cyanotype-dev/cyanotype/internal/spec"
)

func TestDraft(t *testing.T) {
	node := detect.Result{Findings: []detect.Finding{
		{ID: "node", Evidence: []detect.Evidence{{File: "package.json"}}},
		{ID: "npm", Evidence: []detect.Evidence{{File: "package-lock.json"}}},
	}}
	tests := []struct {
		name        string
		in          DraftInput
		wantTargets []spec.Target
		wantStack   []string
	}{
		{
			name:        "targets of the hand-written files",
			in:          DraftInput{HandWritten: []string{"AGENTS.md", "CLAUDE.md", ".cursorrules"}, Name: "demo"},
			wantTargets: []spec.Target{spec.TargetClaude, spec.TargetAgentsMD},
		},
		{
			name:        "agents chosen by the user",
			in:          DraftInput{HandWritten: []string{"CLAUDE.md"}, Agents: []simple.Agent{simple.AgentCodex}, Name: "demo"},
			wantTargets: []spec.Target{spec.TargetAgentsMD},
		},
		{
			name:        "detected stack",
			in:          DraftInput{HandWritten: []string{"CLAUDE.md"}, Name: "demo", Found: node},
			wantTargets: []spec.Target{spec.TargetClaude},
			wantStack:   []string{"node", "npm"},
		},
		{
			name:        "detected stack without an entry",
			in:          DraftInput{HandWritten: []string{"CLAUDE.md"}, Name: "demo", Found: node, Without: []string{"npm"}},
			wantTargets: []spec.Target{spec.TargetClaude},
			wantStack:   []string{"node"},
		},
	}
	c := testCatalog(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			draft, stack, err := Draft(c, tt.in)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(stack, tt.wantStack) {
				t.Errorf("stack = %v, want %v", stack, tt.wantStack)
			}
			f, err := spec.Load(bytes.NewReader(draft))
			if err != nil {
				t.Fatalf("the draft is not a valid spec: %v\n%s", err, draft)
			}
			s := f.Spec
			if !slices.Equal(s.Targets, tt.wantTargets) {
				t.Errorf("targets = %v, want %v", s.Targets, tt.wantTargets)
			}
			if !slices.Equal(s.Selections, tt.wantStack) {
				t.Errorf("selections = %v, want %v", s.Selections, tt.wantStack)
			}
			if s.Project.Name != "demo" || s.Project.Goal != GoalPlaceholder {
				t.Errorf("project = %+v, want the name and the goal placeholder", s.Project)
			}
			if s.CatalogVersion != c.Version() {
				t.Errorf("catalog_version = %q, want %q", s.CatalogVersion, c.Version())
			}
		})
	}
}
