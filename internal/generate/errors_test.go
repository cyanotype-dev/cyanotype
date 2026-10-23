package generate

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/cyanotype-dev/cyanotype/internal/adopt"
)

// TestWriteRefusalsAreTyped checks that each refusal of Write wraps its own
// error, so interfaces that report errors as data can name it with a code.
func TestWriteRefusalsAreTyped(t *testing.T) {
	tests := []struct {
		name    string
		files   map[string]string
		changes []Change
		want    error
	}{
		{
			name:    "outside the root",
			changes: []Change{{Path: "../evil.md", Status: Created, New: []byte("x\n")}},
			want:    ErrOutsideRoot,
		},
		{
			name:    "a folder",
			files:   map[string]string{"AGENTS.md/keep": "x"},
			changes: []Change{{Path: "AGENTS.md", Status: Modified, New: []byte("x\n")}},
			want:    ErrNotRegular,
		},
		{
			name:    "changed since the preview",
			files:   map[string]string{"docs/agents/git.md": "edited\n"},
			changes: []Change{{Path: "docs/agents/git.md", Status: Deleted, Old: []byte("generated\n")}},
			want:    ErrChanged,
		},
		{
			name:    "hand-written without a choice",
			files:   map[string]string{"AGENTS.md": "# Mine\n"},
			changes: []Change{{Path: "AGENTS.md", Status: Modified, Old: []byte("# Mine\n"), New: []byte("x\n"), HandWritten: true}},
			want:    ErrHandWritten,
		},
		{
			name:  "move onto an existing file",
			files: map[string]string{"docs/agents/original-agents.md": "other\n"},
			changes: []Change{{Path: "docs/agents/original-agents.md", Status: Created, New: []byte("x\n"),
				MovedFrom: "AGENTS.md"}},
			want: ErrExists,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, root := project(t, tt.files)
			if _, err := Write(root, tt.changes); !errors.Is(err, tt.want) {
				t.Errorf("Write() error = %v, want one wrapping %q", err, tt.want)
			}
		})
	}

	t.Run("a symbolic link", func(t *testing.T) {
		dir, root := project(t, map[string]string{"AGENTS.md": "shared\n"})
		symlink(t, "AGENTS.md", filepath.Join(dir, "CLAUDE.md"))
		_, err := Write(root, []Change{{Path: "CLAUDE.md", Status: Modified, New: []byte("x\n")}})
		if !errors.Is(err, ErrSymlink) {
			t.Errorf("Write() error = %v, want one wrapping %q", err, ErrSymlink)
		}
	})

	t.Run("replace without git", func(t *testing.T) {
		dir, root := project(t, map[string]string{"AGENTS.md": "# Mine\n"})
		t.Setenv("PATH", filepath.Join(dir, "no-git-here"))
		_, err := Write(root, []Change{{Path: "AGENTS.md", Status: Modified, New: []byte("x\n"),
			HandWritten: true, Choice: adopt.Replace}})
		if !errors.Is(err, adopt.ErrNotRecoverable) {
			t.Errorf("Write() error = %v, want one wrapping %q", err, adopt.ErrNotRecoverable)
		}
		if got, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md")); string(got) != "# Mine\n" { //nolint:gosec // test temp dir
			t.Errorf("AGENTS.md was changed to %q", got)
		}
	})
}
