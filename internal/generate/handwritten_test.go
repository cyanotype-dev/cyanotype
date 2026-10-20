package generate

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestPreviewHandWritten checks that an instruction file without the
// cyanotype header is reported as hand-written, and a generated one edited
// by hand is not: it keeps its header, so regeneration may replace it.
func TestPreviewHandWritten(t *testing.T) {
	_, root := project(t, map[string]string{"AGENTS.md": "# My rules\n\nUse pnpm.\n"})
	changes := preview(t, root)
	c := find(t, changes, "AGENTS.md")
	if c.Status != Modified || !c.HandWritten {
		t.Errorf("hand-written AGENTS.md: status %v, HandWritten %v; want modified, true", c.Status, c.HandWritten)
	}
	if got := HandWritten(changes); !slices.Equal(got, []string{"AGENTS.md"}) {
		t.Errorf("HandWritten() = %q", got)
	}
	// .gitignore is merged, never replaced, so it is never hand-written.
	if find(t, changes, ".gitignore").HandWritten {
		t.Error(".gitignore is reported as hand-written")
	}

	generated := string(find(t, preview(t, mustRoot(t, nil)), "AGENTS.md").New)
	edited := strings.Replace(generated, "# tiny\n", "# tiny, edited\n", 1)
	_, root = project(t, map[string]string{"AGENTS.md": edited})
	if c := find(t, preview(t, root), "AGENTS.md"); c.Status != Modified || c.HandWritten {
		t.Errorf("edited generated AGENTS.md: status %v, HandWritten %v; want modified, false", c.Status, c.HandWritten)
	}
}

// TestWriteRefusesHandWritten checks that Write replaces no hand-written
// file and writes nothing else either.
func TestWriteRefusesHandWritten(t *testing.T) {
	const mine = "# My rules\n\nUse pnpm.\n"
	dir, root := project(t, map[string]string{"AGENTS.md": mine})
	written, err := Write(root, preview(t, root))
	if !errors.Is(err, ErrHandWritten) || !strings.Contains(err.Error(), "refusing to replace AGENTS.md") {
		t.Fatalf("Write() error = %v, want ErrHandWritten for AGENTS.md", err)
	}
	if written != nil {
		t.Errorf("Write() reports %q written", written)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md")); string(got) != mine { //nolint:gosec // test temp dir
		t.Errorf("AGENTS.md was changed to %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, ".gitignore")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf(".gitignore was written: %v", err)
	}
}

// mustRoot is project without the folder path.
func mustRoot(t *testing.T, files map[string]string) *os.Root {
	t.Helper()
	_, root := project(t, files)
	return root
}
