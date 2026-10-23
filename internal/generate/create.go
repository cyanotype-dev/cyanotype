package generate

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/cyanotype-dev/cyanotype/internal/spec"
)

// ErrSpecExists reports a project folder that already has a cyanotype.yaml,
// which CreateSpec never overwrites.
var ErrSpecExists = errors.New(SpecFile + " already exists")

// CheckNoSpec returns an error wrapping ErrSpecExists when the project
// already has a cyanotype.yaml, even as a symbolic link, so interfaces can
// stop before asking any question.
func CheckNoSpec(root *os.Root) error {
	_, err := root.Lstat(SpecFile)
	switch {
	case err == nil:
		return fmt.Errorf("%w in %s", ErrSpecExists, root.Name())
	case errors.Is(err, fs.ErrNotExist):
		return nil
	default:
		return fmt.Errorf("check %s: %w", SpecFile, err)
	}
}

// WriteSpec replaces cyanotype.yaml at the root of the project with data,
// the spec a change of choices produced, such as cyanotype select's. It
// writes only when the file still holds read, the bytes the change started
// from, so an edit made in between is never lost: otherwise the error
// wraps ErrChanged. Like Write, it refuses a cyanotype.yaml that is a
// symbolic link or not a regular file, and replaces the file atomically,
// keeping its permissions.
func WriteSpec(root *os.Root, data, read []byte) error {
	if err := checkTarget(root, SpecFile); err != nil {
		return err
	}
	now, err := root.ReadFile(SpecFile)
	if err != nil {
		return fmt.Errorf("read %s: %w", SpecFile, err)
	}
	if !bytes.Equal(now, read) {
		return fmt.Errorf("refusing to write %s: %w; run the command again", SpecFile, ErrChanged)
	}
	if err := writeFile(root, SpecFile, data); err != nil {
		return fmt.Errorf("write %s: %w", SpecFile, err)
	}
	return nil
}

// CreateSpec writes a new cyanotype.yaml at the root of the project, in the
// canonical layout of spec.Save. It never replaces an existing file: the
// file is created exclusively, so one that appears between CheckNoSpec and
// CreateSpec is reported with ErrSpecExists rather than overwritten.
//
// If writing fails after the file was created, CreateSpec removes it, so a
// half-written spec never stays behind. That is the only file it removes,
// and only because this call created it.
func CreateSpec(root *os.Root, f *spec.File) error {
	var buf bytes.Buffer
	if err := f.Save(&buf); err != nil {
		return err
	}

	out, err := root.OpenFile(SpecFile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, filePerm)
	if errors.Is(err, fs.ErrExist) {
		return fmt.Errorf("%w in %s", ErrSpecExists, root.Name())
	}
	if err != nil {
		return fmt.Errorf("create %s: %w", SpecFile, err)
	}
	_, err = out.Write(buf.Bytes())
	if err == nil {
		err = out.Sync()
	}
	if closeErr := out.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = root.Remove(SpecFile) // the file this call created, half written
		return fmt.Errorf("write %s: %w", SpecFile, err)
	}
	return nil
}
