// Package ops runs the work that the CLI's commands and the MCP tools
// share, and returns it as the types of the JSON contract: the CLI prints
// them as text or encodes them with --json, and the MCP server returns
// them, so both interfaces compute every answer the same way.
//
// Like every internal package it never prints, reads stdin or exits. The
// project is an *os.Root, so every read stays inside the project folder.
package ops

import (
	"errors"
	"os"
	"strings"

	"github.com/cyanotype-dev/cyanotype/internal/catalog"
	"github.com/cyanotype-dev/cyanotype/internal/contract"
	"github.com/cyanotype-dev/cyanotype/internal/detect"
	"github.com/cyanotype-dev/cyanotype/internal/generate"
	"github.com/cyanotype-dev/cyanotype/internal/spec"
)

// Search returns the entries of c that match q.
func Search(c *catalog.Catalog, q catalog.Query) *contract.Catalog {
	return contract.NewCatalog(c.Version(), c.Search(q))
}

// Spec returns the spec of the project at root.
func Spec(root *os.Root) *contract.Spec {
	f, err := generate.LoadSpec(root)
	if err != nil {
		return &contract.Spec{Diagnostics: failed(err)}
	}
	return contract.NewSpec(f.Spec)
}

// Detect returns the catalog entries the files of the project at root
// suggest. It needs no spec.
func Detect(c *catalog.Catalog, root *os.Root) *contract.Detect {
	return contract.NewDetect(c, detect.Detect(c, root.FS()))
}

// Validate resolves the spec of the project at root against c and renders
// the files, without comparing or writing them, and reports whether that
// works and how long each generated Markdown file is. Valid is false for
// any problem in the spec or in what rendering reads, such as unmatched
// markers; it is left out only when no spec could be read.
func Validate(c *catalog.Catalog, root *os.Root) *contract.Validate {
	f, err := generate.LoadSpec(root)
	if err != nil {
		out := &contract.Validate{Diagnostics: failed(err)}
		if isSpecError(err) {
			out.Valid = new(false)
		}
		return out
	}
	changes, warnings, err := generate.Preview(c, f, root.FS())
	out := &contract.Validate{Diagnostics: contract.Diagnostics{Warnings: contract.Warnings(warnings)}}
	if err != nil {
		out.Errors = contract.Errors(err)
		out.Valid = new(false)
		return out
	}
	out.Valid = new(true)
	out.MaxLines = f.Spec.WithDefaults().Output.MaxLines
	out.Files = []contract.FileLines{}
	for _, ch := range changes {
		if ch.Status != generate.Deleted && strings.HasSuffix(ch.Path, ".md") {
			out.Files = append(out.Files, contract.FileLines{Path: ch.Path, Lines: strings.Count(string(ch.New), "\n")})
		}
	}
	return out
}

// failed returns the diagnostics of a command that could not do its work.
func failed(err error) contract.Diagnostics {
	return contract.Diagnostics{Errors: contract.Errors(err)}
}

// isSpecError reports whether err holds problems found in the spec file,
// as opposed to a missing file or one that cannot be read.
func isSpecError(err error) bool {
	var ve *spec.ValidationError
	return errors.As(err, &ve)
}
