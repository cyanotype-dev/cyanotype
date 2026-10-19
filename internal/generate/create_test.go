package generate

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCreateSpec(t *testing.T) {
	dir, root := project(t, nil)
	if err := CheckNoSpec(root); err != nil {
		t.Fatalf("CheckNoSpec() on an empty project = %v", err)
	}

	if err := CreateSpec(root, loadSpec(t, testSpec)); err != nil {
		t.Fatalf("CreateSpec() error: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, SpecFile)) //nolint:gosec // test temp dir
	if err != nil {
		t.Fatal(err)
	}
	// testSpec is in canonical layout apart from the blank lines between
	// top-level keys, which Save adds.
	want := "version: 1\n\ntargets: [agents-md, cursor]\n\nproject:\n  name: tiny\n  goal: A tiny project for tests.\n\nselections: [conventional-commits]\n"
	if string(got) != want {
		t.Errorf("written spec:\n%s\nwant:\n%s", got, want)
	}
}

func TestCreateSpecRefusesExisting(t *testing.T) {
	const existing = "# hand-written\n"
	dir, root := project(t, map[string]string{SpecFile: existing})

	if err := CheckNoSpec(root); !errors.Is(err, ErrSpecExists) {
		t.Errorf("CheckNoSpec() = %v, want ErrSpecExists", err)
	}
	if err := CreateSpec(root, loadSpec(t, testSpec)); !errors.Is(err, ErrSpecExists) {
		t.Errorf("CreateSpec() = %v, want ErrSpecExists", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, SpecFile)) //nolint:gosec // test temp dir
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != existing {
		t.Errorf("the existing spec was changed to %q", got)
	}
}

func TestCreateSpecRefusesSymlink(t *testing.T) {
	dir, root := project(t, map[string]string{"elsewhere.yaml": "kept\n"})
	symlink(t, "elsewhere.yaml", filepath.Join(dir, SpecFile))

	if err := CheckNoSpec(root); !errors.Is(err, ErrSpecExists) {
		t.Errorf("CheckNoSpec() = %v, want ErrSpecExists", err)
	}
	if err := CreateSpec(root, loadSpec(t, testSpec)); !errors.Is(err, ErrSpecExists) {
		t.Errorf("CreateSpec() = %v, want ErrSpecExists", err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "elsewhere.yaml")); string(got) != "kept\n" { //nolint:gosec // test temp dir
		t.Errorf("the link target was changed to %q", got)
	}
}

func TestCreateSpecRefusesInvalidSpec(t *testing.T) {
	dir, root := project(t, nil)
	f := loadSpec(t, testSpec)
	f.Spec.Project.Goal = ""

	if err := CreateSpec(root, f); err == nil {
		t.Fatal("CreateSpec() succeeded with an invalid spec")
	}
	if _, err := os.Stat(filepath.Join(dir, SpecFile)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("an invalid spec left a file behind: %v", err)
	}
}
