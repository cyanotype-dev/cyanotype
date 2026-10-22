package generate

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/cyanotype-dev/cyanotype/internal/adopt"
	"github.com/cyanotype-dev/cyanotype/internal/lock"
	"github.com/cyanotype-dev/cyanotype/internal/render"
)

// checkLockRecords checks that the lock among changes records the catalog
// version and every other generated file, with the hash of the part
// cyanotype owns, and no moved file.
func checkLockRecords(t *testing.T, changes []Change) {
	t.Helper()
	var l *lock.Lock
	for _, c := range changes {
		if c.Path == lock.Path {
			var err error
			if l, err = lock.Parse(c.New); err != nil {
				t.Fatalf("the generated lock does not parse: %v", err)
			}
		}
	}
	if l == nil {
		t.Fatal("no change holds the lock")
	}
	if l.CatalogVersion != loadCatalog(t).Version() {
		t.Errorf("lock catalog_version = %q, want the bundled %q", l.CatalogVersion, loadCatalog(t).Version())
	}
	for _, c := range changes {
		if c.MovedFrom != "" {
			// Hand-written rules moved aside are never cyanotype's.
			if got := l.Files[c.Path]; got != "" {
				t.Errorf("lock records the moved file %s", c.Path)
			}
			continue
		}
		owned, ok, err := render.Owned(c.Path, c.New)
		if err != nil {
			t.Fatal(err)
		}
		if got := l.Files[c.Path]; ok && got != lock.Hash(owned) {
			t.Errorf("lock records %s as %q, want %q", c.Path, got, lock.Hash(owned))
		} else if !ok && got != "" {
			t.Errorf("lock records %s, which has no owned content", c.Path)
		}
	}
}

// TestLockNeverRecordsMovedFiles checks that a file holding hand-written
// rules moved aside by --existing move is never recorded, so it can never
// be taken for a stale generated file and deleted, even once the output
// that points to it changes.
func TestLockNeverRecordsMovedFiles(t *testing.T) {
	_, root := project(t, map[string]string{"AGENTS.md": "# My rules\n"})
	changes, _, err := PreviewChoices(loadCatalog(t), loadSpec(t, testSpec), root.FS(), Choices{"AGENTS.md": adopt.Move})
	if err != nil {
		t.Fatal(err)
	}
	checkLockRecords(t, changes)
	if _, err := Write(root, changes); err != nil {
		t.Fatal(err)
	}
	l, err := lock.Read(root.FS())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := l.Files["docs/agents/original-agents.md"]; ok {
		t.Errorf("the lock records the moved file: %v", l.Files)
	}

	other := strings.Replace(testSpec, "targets: [agents-md, cursor]", "targets: [claude]", 1)
	again, _, err := Preview(loadCatalog(t), loadSpec(t, other), root.FS())
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range again {
		if c.Status == Deleted && c.Path != "AGENTS.md" {
			t.Errorf("%s would be deleted", c.Path)
		}
	}
}

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
