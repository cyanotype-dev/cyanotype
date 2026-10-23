package generate

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteSpec(t *testing.T) {
	dir, root := project(t, map[string]string{SpecFile: testSpec})
	if err := WriteSpec(root, []byte("new\n"), []byte(testSpec)); err != nil {
		t.Fatalf("WriteSpec() error: %v", err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, SpecFile)); string(got) != "new\n" { //nolint:gosec // test temp dir
		t.Errorf("cyanotype.yaml = %q", got)
	}
	assertNoTempFiles(t, dir)
}

// TestWriteSpecRefusesChangedFile edits the spec between reading and
// writing it, as a user could while looking at the diff.
func TestWriteSpecRefusesChangedFile(t *testing.T) {
	dir, root := project(t, map[string]string{SpecFile: testSpec + "# my note\n"})
	if err := WriteSpec(root, []byte("new\n"), []byte(testSpec)); !errors.Is(err, ErrChanged) {
		t.Errorf("WriteSpec() error = %v, want one wrapping ErrChanged", err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, SpecFile)); string(got) != testSpec+"# my note\n" { //nolint:gosec // test temp dir
		t.Errorf("cyanotype.yaml was changed to %q", got)
	}
}

func TestWriteSpecRefusesSymlink(t *testing.T) {
	dir, root := project(t, map[string]string{"elsewhere.yaml": testSpec})
	symlink(t, "elsewhere.yaml", filepath.Join(dir, SpecFile))
	if err := WriteSpec(root, []byte("new\n"), []byte(testSpec)); !errors.Is(err, ErrSymlink) {
		t.Errorf("WriteSpec() error = %v, want one wrapping ErrSymlink", err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "elsewhere.yaml")); string(got) != testSpec { //nolint:gosec // test temp dir
		t.Errorf("the link target was changed to %q", got)
	}
}
