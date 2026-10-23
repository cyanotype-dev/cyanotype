package contract

import (
	"github.com/cyanotype-dev/cyanotype/internal/generate"
	"github.com/cyanotype-dev/cyanotype/internal/resolve"
)

// Generate is the output of cyanotype generate --json and of the MCP tools
// preview and generate: every generated file compared with the project,
// with the same diff the CLI prints, and what was written.
type Generate struct {
	Head
	// Files lists every generated file, sorted by path, unchanged ones
	// included.
	Files []File `json:"files,omitzero"`
	// Summary counts Files by status.
	Summary *Summary `json:"summary,omitzero"`
	// WaitingForChoice lists the hand-written instruction files that
	// nothing replaces until the user chooses what to do with them.
	WaitingForChoice []string `json:"waiting_for_choice,omitzero"`
	// Written lists the files written or deleted, in the order they were;
	// empty when nothing was written, and partial when writing failed
	// halfway, as the errors then say.
	Written []string `json:"written,omitzero"`
	// Import holds the prompt that converts hand-written files into
	// cyanotype.yaml, when import was chosen; nothing is written then.
	Import *Import `json:"import,omitzero"`
	Diagnostics
}

// File is one generated file compared with the file on disk. Its full
// content is left out: the diff holds every line that changes, and all of
// a created file.
type File struct {
	// Path is relative to the project root, with forward slashes.
	Path string `json:"path"`
	// Status is "created", "modified", "deleted" or "unchanged".
	Status string `json:"status"`
	// Diff is the change as a unified diff, as the CLI prints it; omitted
	// for an unchanged file and for a hand-written file waiting for a
	// choice, which nothing will replace.
	Diff string `json:"diff,omitempty"`
	// CRLFToLF reports that writing the file turns its CRLF line endings
	// into LF.
	CRLFToLF bool `json:"crlf_to_lf,omitempty"`
	// HandWritten reports that the file on disk is an instruction file
	// cyanotype did not generate.
	HandWritten bool `json:"hand_written,omitempty"`
	// Choice is what was chosen for a hand-written file: "keep", "move" or
	// "replace".
	Choice string `json:"choice,omitempty"`
	// MovedFrom names the hand-written file whose content a moved file
	// holds.
	MovedFrom string `json:"moved_from,omitempty"`
}

// Summary counts generated files by status.
type Summary struct {
	Created   int `json:"created"`
	Modified  int `json:"modified"`
	Deleted   int `json:"deleted"`
	Unchanged int `json:"unchanged"`
}

// Import is the prompt for the user's coding agent that converts
// hand-written instruction files into cyanotype.yaml.
type Import struct {
	// Files are the hand-written files to convert.
	Files []string `json:"files"`
	// Prompt is the text to give the agent, the same the CLI prints.
	Prompt string `json:"prompt"`
}

// NewGenerate returns the output for the changes and warnings of
// generate.Preview or generate.PreviewChoices, with nothing written yet.
func NewGenerate(changes []generate.Change, warnings []resolve.Warning) *Generate {
	out := &Generate{
		Files:            make([]File, 0, len(changes)),
		Summary:          &Summary{},
		WaitingForChoice: []string{},
		Written:          []string{},
		Diagnostics:      Diagnostics{Warnings: Warnings(warnings)},
	}
	for _, ch := range changes {
		waiting := ch.HandWritten && ch.Choice == ""
		f := File{Path: ch.Path, Status: ch.Status.String(), CRLFToLF: ch.CRLF, HandWritten: ch.HandWritten,
			Choice: string(ch.Choice), MovedFrom: ch.MovedFrom}
		if !waiting {
			f.Diff = ch.Diff()
		}
		if waiting {
			out.WaitingForChoice = append(out.WaitingForChoice, ch.Path)
		}
		out.Files = append(out.Files, f)
		switch ch.Status {
		case generate.Created:
			out.Summary.Created++
		case generate.Modified:
			out.Summary.Modified++
		case generate.Deleted:
			out.Summary.Deleted++
		case generate.Unchanged:
			out.Summary.Unchanged++
		}
	}
	return out
}
