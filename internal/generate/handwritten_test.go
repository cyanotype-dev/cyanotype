package generate

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/cyanotype-dev/cyanotype/internal/adopt"
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

// regenerate previews the project again without choices and fails the test
// unless every file is unchanged: once a choice is written, the files are
// cyanotype's and regeneration keeps the hand-written text.
func regenerate(t *testing.T, root *os.Root) {
	t.Helper()
	for _, c := range preview(t, root) {
		if c.Status != Unchanged {
			t.Errorf("after writing, %s is %v on regeneration:\n%s", c.Path, c.Status, c.Diff())
		}
	}
}

func TestChoiceKeep(t *testing.T) {
	const mine = "# My rules\r\n\r\nUse pnpm.\r\n"
	dir, root := project(t, map[string]string{"AGENTS.md": mine})
	changes, _, err := PreviewChoices(loadCatalog(t), loadSpec(t, testSpec), root.FS(), Choices{"AGENTS.md": adopt.Keep})
	if err != nil {
		t.Fatal(err)
	}
	c := find(t, changes, "AGENTS.md")
	if !c.HandWritten || c.Choice != adopt.Keep || string(c.Old) != mine {
		t.Errorf("AGENTS.md: HandWritten %v, Choice %q, Old %q", c.HandWritten, c.Choice, c.Old)
	}
	if !strings.Contains(string(c.New), "# tiny\n\n<!-- cyanotype:custom -->\n# My rules\n\nUse pnpm.\n<!-- /cyanotype:custom -->\n\n## ") {
		t.Errorf("the kept block is not right after the title:\n%s", c.New)
	}
	if _, err := Write(root, changes); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md")); !bytes.Equal(got, c.New) { //nolint:gosec // test temp dir
		t.Errorf("AGENTS.md on disk:\n%s", got)
	}
	regenerate(t, root)
}

func TestChoiceMove(t *testing.T) {
	const mine = "# My rules\r\n\r\nUse pnpm.\r\n"
	dir, root := project(t, map[string]string{"AGENTS.md": mine})
	changes, _, err := PreviewChoices(loadCatalog(t), loadSpec(t, testSpec), root.FS(), Choices{"AGENTS.md": adopt.Move})
	if err != nil {
		t.Fatal(err)
	}
	moved := find(t, changes, "docs/agents/original-agents.md")
	if moved.Status != Created || moved.MovedFrom != "AGENTS.md" || string(moved.New) != mine {
		t.Errorf("moved file: %+v", moved)
	}
	main := string(find(t, changes, "AGENTS.md").New)
	if strings.Contains(main, "My rules") || !strings.Contains(main, "read docs/agents/original-agents.md") {
		t.Errorf("AGENTS.md should point to the moved file instead of holding it:\n%s", main)
	}
	written, err := Write(root, changes)
	if err != nil {
		t.Fatal(err)
	}
	// The moved file is written before the original is replaced.
	if len(written) == 0 || written[0] != "docs/agents/original-agents.md" {
		t.Errorf("written = %q; the moved file must come first", written)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "docs", "agents", "original-agents.md")); string(got) != mine { //nolint:gosec // test temp dir
		t.Errorf("moved file on disk = %q, want the original byte for byte", got)
	}
	regenerate(t, root)
}

func TestChoiceMoveOntoExistingFile(t *testing.T) {
	dir, root := project(t, map[string]string{"AGENTS.md": "# Mine\n"})
	changes, _, err := PreviewChoices(loadCatalog(t), loadSpec(t, testSpec), root.FS(), Choices{"AGENTS.md": adopt.Move})
	if err != nil {
		t.Fatal(err)
	}
	// The destination appears between the preview and the write.
	if err := os.MkdirAll(filepath.Join(dir, "docs", "agents"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "docs", "agents", "original-agents.md"), []byte("other\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Write(root, changes); err == nil || !strings.Contains(err.Error(), "that path already exists") {
		t.Errorf("Write() error = %v, want a refusal", err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md")); string(got) != "# Mine\n" { //nolint:gosec // test temp dir
		t.Errorf("AGENTS.md was changed to %q", got)
	}
}

func TestChoiceReplace(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Fatal("git is not installed; replace needs it")
	}
	const mine = "# My rules\n"
	dir, root := project(t, map[string]string{"AGENTS.md": mine})
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(dir))

	preview := func() []Change {
		t.Helper()
		changes, _, err := PreviewChoices(loadCatalog(t), loadSpec(t, testSpec), root.FS(), Choices{"AGENTS.md": adopt.Replace})
		if err != nil {
			t.Fatal(err)
		}
		return changes
	}

	// Not a repository yet: nothing holds a copy, so nothing is written.
	if _, err := Write(root, preview()); !errors.Is(err, adopt.ErrNotRecoverable) {
		t.Fatalf("Write() outside a repository: error = %v, want ErrNotRecoverable", err)
	}
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=Test", "-c", "user.email=test@example.com", "-c", "commit.gpgsign=false"}, args...)...) //nolint:gosec // test code runs git with fixed arguments
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q")
	git("add", "AGENTS.md")
	if _, err := Write(root, preview()); !errors.Is(err, adopt.ErrNotRecoverable) {
		t.Fatalf("Write() before the commit: error = %v, want ErrNotRecoverable", err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md")); string(got) != mine { //nolint:gosec // test temp dir
		t.Fatalf("AGENTS.md was changed to %q", got)
	}

	git("commit", "-q", "-m", "rules")
	changes := preview()
	if _, err := Write(root, changes); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md")); !bytes.Equal(got, find(t, changes, "AGENTS.md").New) { //nolint:gosec // test temp dir
		t.Errorf("AGENTS.md was not replaced:\n%s", got)
	}
	regenerate(t, root)
}

// mustRoot is project without the folder path.
func mustRoot(t *testing.T, files map[string]string) *os.Root {
	t.Helper()
	_, root := project(t, files)
	return root
}
