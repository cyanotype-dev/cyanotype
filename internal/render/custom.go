package render

import (
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
				return nil, fmt.Errorf("line %d: %s inside the block opened on line %d", n, customBegin, start)
			}
			open, start = &customBlock{anchor: anchor}, n
			open.lines = append(open.lines, line)
		case customEnd:
			if open == nil {
				return nil, fmt.Errorf("line %d: %s without a %s before it", n, customEnd, customBegin)
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
		return nil, fmt.Errorf("line %d: %s is never closed with %s", start, customBegin, customEnd)
	}
	return blocks, nil
}
