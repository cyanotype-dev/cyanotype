package catalog

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"
)

// Names inside a catalog folder laid out like catalog/ in the repository.
const (
	// ManifestFile holds what describes the catalog as a whole.
	ManifestFile = "catalog.yaml"
	// EntriesDir holds one YAML file per entry.
	EntriesDir = "entries"
)

// Manifest describes the catalog as a whole, as opposed to one entry. It is
// decoded from catalog.yaml at the root of the catalog folder.
type Manifest struct {
	// Version is the catalog's own date-based version, such as "2026.10".
	// It lives in the data rather than in the code because a catalog
	// downloaded on its own must carry its version with it, and specs pin it.
	Version string `yaml:"version"`
}

// LoadRoot loads a whole catalog folder: the manifest in catalog.yaml and
// the entries in the entries folder. Problems in both are returned together,
// joined with errors.Join; each one is a *ValidationError unless reading
// the files failed.
func LoadRoot(fsys fs.FS) (*Catalog, error) {
	m, errs, err := loadManifest(fsys)
	if err != nil {
		return nil, err
	}

	entries, err := fs.Sub(fsys, EntriesDir)
	if err != nil {
		return nil, fmt.Errorf("open %s folder: %w", EntriesDir, err)
	}
	c, err := Load(entries)
	if err != nil {
		var ve *ValidationError
		if !errors.As(err, &ve) {
			return nil, err // reading failed, so validation problems are beside the point
		}
		// Flatten, so callers see one list of problems.
		var joined interface{ Unwrap() []error }
		if errors.As(err, &joined) {
			errs = append(errs, joined.Unwrap()...)
		} else {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	c.version = m.Version
	return c, nil
}

// loadManifest reads and validates catalog.yaml. Validation problems come
// back as errs; err is set only when the file cannot be read.
func loadManifest(fsys fs.FS) (m Manifest, errs []error, err error) {
	data, err := fs.ReadFile(fsys, ManifestFile)
	if errors.Is(err, fs.ErrNotExist) {
		return m, []error{&ValidationError{File: ManifestFile, Msg: "is missing; it holds the catalog version"}}, nil
	}
	if err != nil {
		return m, nil, fmt.Errorf("read %s: %w", ManifestFile, err)
	}

	m, errs = decode[Manifest](ManifestFile, data)
	if len(errs) > 0 {
		return m, errs, nil
	}
	switch {
	case strings.TrimSpace(m.Version) == "":
		errs = append(errs, &ValidationError{File: ManifestFile, Field: "version", Msg: "is required"})
	case !ValidVersion(m.Version):
		errs = append(errs, &ValidationError{File: ManifestFile, Field: "version", Msg: fmt.Sprintf(
			"%q must be a date-based version such as \"2026.10\" or \"2026.10.1\", quoted", m.Version)})
	}
	return m, errs, nil
}
