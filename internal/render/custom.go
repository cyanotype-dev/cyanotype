package render

import (
	"errors"
	"fmt"
	"strings"
)

// Markers of a protected block. Hand-written text between them, in a file
// cyanotype generated, survives regeneration. Each marker must be alone on
// its line.
const (
	customBegin = "<!-- cyanotype:custom -->"
	customEnd   = "<!-- /cyanotype:custom -->"
)

// CustomBegin and CustomEnd are the markers of a protected block, for
// packages that write blocks of their own, such as one keeping a whole
// hand-written file.
const (
	CustomBegin = customBegin
	CustomEnd   = customEnd
)

// customBlock is a protected block found in an existing file.
type customBlock struct {
	// anchor is the title of the nearest "## " heading above the block, or
	// "" when the block comes before every section. It decides where the
	// block goes when the file is generated again.
	anchor string
	// lines are the block's lines, markers included, exactly as written
	// apart from CRLF line endings, which become LF.
	lines []string
}

// MarkerError reports cyanotype markers that do not pair up in a file: a
// cyanotype:custom marker without its partner or inside another block, or
// a broken .gitignore block. Generation stops on it rather than guess, and
// interfaces that report errors as data give its file and line.
type MarkerError struct {
	// File is the file holding the markers, relative to the project root,
	// or "" until the renderer knows which file it parsed.
	File string
	// Line is the 1-based line of the marker that does not pair up.
	Line int
	Msg  string
}

// Error returns "line N: message"; the caller names the file in front.
func (e *MarkerError) Error() string {
	return fmt.Sprintf("line %d: %s", e.Line, e.Msg)
}

// markerError returns a *MarkerError for line, with a formatted message.
func markerError(line int, format string, args ...any) error {
	return &MarkerError{Line: line, Msg: fmt.Sprintf(format, args...)}
}

// inFile records path as the file of the *MarkerError err holds, if any,
// and returns err.
func inFile(err error, path string) error {
	var me *MarkerError
	if errors.As(err, &me) && me.File == "" {
		me.File = path
	}
	return err
}

// parseCustomBlocks returns the protected blocks of content, in order.
// Headings inside a block are hand-written text, so they never anchor a
// later block.
//
// A marker without its partner, or a block inside another, is an error
// rather than a guess: guessing wrong would silently drop hand-written text.
func parseCustomBlocks(content []byte) ([]customBlock, error) {
	if len(content) == 0 {
		return nil, nil
	}
	text := strings.ReplaceAll(string(content), "\r\n", "\n")
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")

	var (
		blocks []customBlock
		anchor string
		open   *customBlock
		start  int // 1-based line of the open block's begin marker
	)
	for i, line := range lines {
		n := i + 1
		switch strings.TrimSpace(line) {
		case customBegin:
			if open != nil {
				return nil, markerError(n, "%s inside the block opened on line %d", customBegin, start)
			}
			open, start = &customBlock{anchor: anchor}, n
			open.lines = append(open.lines, line)
		case customEnd:
			if open == nil {
				return nil, markerError(n, "%s without a %s before it", customEnd, customBegin)
			}
			open.lines = append(open.lines, line)
			blocks = append(blocks, *open)
			open = nil
		default:
			if open != nil {
				open.lines = append(open.lines, line)
			} else if title, ok := strings.CutPrefix(line, "## "); ok {
				anchor = strings.TrimSpace(title)
			}
		}
	}
	if open != nil {
		return nil, markerError(start, "%s is never closed with %s", customBegin, customEnd)
	}
	return blocks, nil
}
