package generate

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/cyanotype-dev/cyanotype/internal/lock"
)

func TestDeletedDiff(t *testing.T) {
	c := Change{Path: "docs/agents/git.md", Status: Deleted, Old: []byte("# Git workflow\n")}
	want := "--- a/docs/agents/git.md\n+++ /dev/null\n@@ -1 +0,0 @@\n-# Git workflow\n"
	if got := c.Diff(); got != want {
		t.Errorf("Diff() =\n%s\nwant:\n%s", got, want)
	}
	if got := Deleted.String(); got != "deleted" {
		t.Errorf("Deleted.String() = %q", got)
	}
}

func TestWriteDeletes(t *testing.T) {
	dir, root := project(t, map[string]string{"docs/agents/git.md": "# Git workflow\n", "AGENTS.md": "old\n"})
	changes := []Change{
		{Path: "AGENTS.md", Status: Modified, Old: []byte("old\n"), New: []byte("new\n")},
		{Path: "docs/agents/git.md", Status: Deleted, Old: []byte("# Git workflow\n")},
	}
	written, err := Write(root, changes)
	if err != nil {
		t.Fatalf("Write() error: %v", err)
	}
	if want := []string{"AGENTS.md", "docs/agents/git.md"}; !slices.Equal(written, want) {
		t.Errorf("Write() = %q, want %q", written, want)
	}
	if _, err := os.Stat(filepath.Join(dir, "docs", "agents", "git.md")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("docs/agents/git.md still exists: %v", err)
	}
	// Folders are never removed, even when deleting leaves one empty.
	if info, err := os.Stat(filepath.Join(dir, "docs", "agents")); err != nil || !info.IsDir() {
		t.Errorf("docs/agents was removed: %v", err)
	}
}

// TestWriteRefusesChangedDeletion edits a file between Preview and Write,
// as a user could while reading the diff: the edit must not be lost.
func TestWriteRefusesChangedDeletion(t *testing.T) {
	dir, root := project(t, map[string]string{"docs/agents/git.md": "# Git workflow\n- my note\n"})
	_, err := Write(root, []Change{{Path: "docs/agents/git.md", Status: Deleted, Old: []byte("# Git workflow\n")}})
	if err == nil || !strings.Contains(err.Error(), "it changed after cyanotype compared it") {
		t.Fatalf("Write() error = %v, want a refusal", err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "docs", "agents", "git.md")); string(got) != "# Git workflow\n- my note\n" { //nolint:gosec // test temp dir
		t.Errorf("docs/agents/git.md = %q", got)
	}
}

func TestWriteRefusesDeletingSymlinks(t *testing.T) {
	dir, root := project(t, map[string]string{"AGENTS.md": "shared\n"})
	symlink(t, "AGENTS.md", filepath.Join(dir, "CLAUDE.md"))
	_, err := Write(root, []Change{{Path: "CLAUDE.md", Status: Deleted, Old: []byte("shared\n")}})
	if err == nil || !strings.Contains(err.Error(), "it is a symbolic link") {
		t.Fatalf("Write() error = %v, want a refusal", err)
	}
	if _, err := os.Lstat(filepath.Join(dir, "CLAUDE.md")); err != nil {
		t.Errorf("the link was removed: %v", err)
	}
}

// TestWriteOrder checks that the lock is written before the other files
// and that deletions come last, so a failure in between leaves a lock that
// corrects itself on the next run.
func TestWriteOrder(t *testing.T) {
	_, root := project(t, map[string]string{"docs/agents/git.md": "# Git workflow\n"})
	changes := []Change{
		{Path: "AGENTS.md", Status: Created, New: []byte("a\n")},
		{Path: "docs/agents/git.md", Status: Deleted, Old: []byte("# Git workflow\n")},
		{Path: "docs/agents/original-claude.md", Status: Created, New: []byte("o\n"), MovedFrom: "CLAUDE.md"},
		{Path: lock.Path, Status: Created, New: []byte("version: 1\n")},
	}
	written, err := Write(root, changes)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"docs/agents/original-claude.md", lock.Path, "AGENTS.md", "docs/agents/git.md"}
	if !slices.Equal(written, want) {
		t.Errorf("Write() order = %q, want %q", written, want)
	}
}
