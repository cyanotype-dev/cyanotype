package contract

import (
	"errors"

	"github.com/cyanotype-dev/cyanotype/internal/adopt"
	"github.com/cyanotype-dev/cyanotype/internal/generate"
	"github.com/cyanotype-dev/cyanotype/internal/lock"
	"github.com/cyanotype-dev/cyanotype/internal/render"
	"github.com/cyanotype-dev/cyanotype/internal/resolve"
	"github.com/cyanotype-dev/cyanotype/internal/spec"
)

// The severities of a Diagnostic.
const (
	SeverityError   = "error"
	SeverityWarning = "warning"
)

// Codes of the errors that do not come from the spec. Errors in the spec
// carry the codes of spec.ValidationError, such as "unknown-entry", and
// warnings the codes of resolve.Warning, such as "catalog-older". Codes are
// stable, while messages may change; a reader that meets a code it does
// not know treats it as CodeError.
const (
	// CodeError: any other error, such as a file that cannot be read.
	CodeError = "error"
	// CodeUsage: wrong flags or arguments.
	CodeUsage = "usage"
	// CodeNoSpec: the project folder has no cyanotype.yaml.
	CodeNoSpec = "no-spec"
	// CodeLockInvalid: cyanotype.lock cannot be read.
	CodeLockInvalid = "lock-invalid"
	// CodeLockNewer: cyanotype.lock was written by a newer cyanotype.
	CodeLockNewer = "lock-newer"
	// CodeLockCatalogNewer: cyanotype.lock was generated with a newer
	// catalog than the bundled one.
	CodeLockCatalogNewer = "lock-catalog-newer"
	// CodeMarkers: cyanotype markers that do not pair up in a file.
	CodeMarkers = "markers"
	// CodeChoiceRequired: hand-written instruction files are waiting for a
	// choice of what to do with them.
	CodeChoiceRequired = "choice-required"
	// CodeHandWritten: a write would replace a hand-written instruction
	// file nobody chose to replace.
	CodeHandWritten = "hand-written"
	// CodeReplaceRefused: replace was chosen, but git could not give the
	// file back.
	CodeReplaceRefused = "replace-refused"
	// CodeKeepRefused: keep was chosen for a file that already has
	// cyanotype:custom markers.
	CodeKeepRefused = "keep-refused"
	// CodeMoveRefused: move was chosen, but its destination exists.
	CodeMoveRefused = "move-refused"
	// CodeUnsafePath: a path outside the project root, or a target that is
	// a symbolic link or not a regular file.
	CodeUnsafePath = "unsafe-path"
	// CodeChangedSincePreview: a file changed between the preview and the
	// write.
	CodeChangedSincePreview = "changed-since-preview"
	// CodeSpecWarning: a warning without a code of its own.
	CodeSpecWarning = "spec-warning"
)

// Diagnostic is one error, warning or finding. Every interface reports
// problems in this shape, so a reader handles all of them the same way.
type Diagnostic struct {
	// Code names the kind of problem; see the Code constants.
	Code string `json:"code"`
	// Severity is SeverityError or SeverityWarning.
	Severity string `json:"severity"`
	// File is the file the problem is about, relative to the project root,
	// such as "cyanotype.yaml"; omitted when it is about no file.
	File string `json:"file,omitempty"`
	// Line is the 1-based line in File; omitted when unknown.
	Line int `json:"line,omitempty"`
	// LineApproximate marks a line that may be one off, as for some YAML
	// syntax errors: show the lines around it.
	LineApproximate bool `json:"line_approximate,omitempty"`
	// Field is the spec field, such as "selections[3]"; omitted when the
	// problem is not about one.
	Field string `json:"field,omitempty"`
	// Message says what is wrong and what to do, in English.
	Message string `json:"message"`
}

// Warnings returns the warnings of resolve and render as diagnostics. A
// warning about the spec names cyanotype.yaml as its file.
func Warnings(warnings []resolve.Warning) []Diagnostic {
	out := make([]Diagnostic, 0, len(warnings))
	for _, w := range warnings {
		d := Diagnostic{Code: w.Code, Severity: SeverityWarning, File: w.File, Line: w.Line, Field: w.Field, Message: w.Msg}
		if d.Code == "" {
			d.Code = CodeSpecWarning
		}
		if d.File == "" {
			d.File = generate.SpecFile
		}
		out = append(out, d)
	}
	return out
}

// Errors returns every problem err holds as diagnostics: each error joined
// with errors.Join becomes one, with the code, file, line and field the
// core gives it.
func Errors(err error) []Diagnostic {
	if err == nil {
		return []Diagnostic{}
	}
	errs := []error{err}
	var joined interface{ Unwrap() []error }
	if errors.As(err, &joined) {
		errs = joined.Unwrap()
	}
	out := make([]Diagnostic, 0, len(errs))
	for _, e := range errs {
		out = append(out, diagnose(e))
	}
	return out
}

// Error returns one diagnostic with the given code and message, for errors
// an interface finds itself, such as CodeUsage.
func Error(code, message string) Diagnostic {
	return Diagnostic{Code: code, Severity: SeverityError, Message: message}
}

// diagnose turns one error into a diagnostic.
func diagnose(err error) Diagnostic {
	d := Error(CodeError, err.Error())
	var ve *spec.ValidationError
	var me *render.MarkerError
	switch {
	case errors.As(err, &ve):
		d.Code, d.File, d.Line, d.Field, d.Message = ve.Code, generate.SpecFile, ve.Line, ve.Field, ve.Msg
		d.LineApproximate = ve.Syntax && ve.Line > 0
		if d.Code == "" {
			d.Code = CodeError
		}
	case errors.As(err, &me):
		d.Code, d.File, d.Line, d.Message = CodeMarkers, me.File, me.Line, me.Msg
	case errors.Is(err, generate.ErrNoSpec):
		d.Code, d.File = CodeNoSpec, generate.SpecFile
	case errors.Is(err, lock.ErrInvalid):
		d.Code, d.File = CodeLockInvalid, lock.Path
	case errors.Is(err, lock.ErrNewer):
		d.Code, d.File = CodeLockNewer, lock.Path
	case errors.Is(err, render.ErrLockCatalogNewer):
		d.Code, d.File = CodeLockCatalogNewer, lock.Path
	case errors.Is(err, generate.ErrHandWritten):
		d.Code = CodeHandWritten
	case errors.Is(err, adopt.ErrNotRecoverable):
		d.Code = CodeReplaceRefused
	case errors.Is(err, adopt.ErrKeepMarkers):
		d.Code = CodeKeepRefused
	case errors.Is(err, adopt.ErrMoveExists), errors.Is(err, generate.ErrExists):
		d.Code = CodeMoveRefused
	case errors.Is(err, generate.ErrOutsideRoot), errors.Is(err, generate.ErrSymlink), errors.Is(err, generate.ErrNotRegular):
		d.Code = CodeUnsafePath
	case errors.Is(err, generate.ErrChanged):
		d.Code = CodeChangedSincePreview
	}
	return d
}
