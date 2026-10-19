package render

import (
	"fmt"
	"io/fs"
	"strings"
)

const (
	gitignorePath = ".gitignore"
	// gitignoreBegin and gitignoreEnd mark the block cyanotype owns. A line
	// starting with gitignoreBegin opens it, so the explanation after the
	// marker can change without orphaning old blocks.
	gitignoreBegin = "# cyanotype:begin"
	gitignoreEnd   = "# cyanotype:end"
	// gitignoreBeginLine is the opening line cyanotype writes.
	gitignoreBeginLine = gitignoreBegin + " (generated from cyanotype.yaml; regeneration replaces this block)"
)

// gitignoreEntries are the personal files that must never be committed:
// the personal spec and the personal topic folder (see Team rules and
// personal rules in docs/DESIGN.md).
var gitignoreEntries = []string{"cyanotype.local.yaml", ".agents.local/"}

// renderGitignore returns .gitignore with cyanotype's block. An existing
// block is replaced where it is and the rest of the file stays byte for
// byte; without one, the block is appended after a blank line. The block
// uses the file's line endings, CRLF when the file has any.
func renderGitignore(existing fs.FS) (File, error) {
	old, err := readExisting(existing, gitignorePath)
	if err != nil {
		return File{}, err
	}
	content, err := mergeGitignore(string(old))
	if err != nil {
		return File{}, fmt.Errorf("render %s: %w", gitignorePath, err)
	}
	return File{Path: gitignorePath, Content: []byte(content)}, nil
}

// mergeGitignore puts cyanotype's block into the .gitignore content old,
// which is empty for a new file.
func mergeGitignore(old string) (string, error) {
	nl := "\n"
	if strings.Contains(old, "\r\n") {
		nl = "\r\n"
	}
	block := strings.Join(append(append([]string{gitignoreBeginLine}, gitignoreEntries...), gitignoreEnd), nl)

	start, end, err := findGitignoreBlock(old)
	if err != nil {
		return "", err
	}
	if start >= 0 {
		return old[:start] + block + old[end:], nil
	}

	switch {
	case old == "":
	case strings.HasSuffix(old, nl+nl):
	case strings.HasSuffix(old, "\n"):
		old += nl
	default:
		old += nl + nl
	}
	return old + block + nl, nil
}

// findGitignoreBlock returns the byte offsets of cyanotype's block in s:
// start is where the begin line starts, end is where the end line's text
// stops, before its line ending. Both are -1 when there is no block. A
// second block, or a marker without its partner, is an error: replacing the
// wrong lines could drop the user's own entries.
func findGitignoreBlock(s string) (start, end int, err error) {
	start, end = -1, -1
	startLine := 0
	for pos, n := 0, 1; pos < len(s); n++ {
		next := len(s)
		if i := strings.IndexByte(s[pos:], '\n'); i >= 0 {
			next = pos + i + 1
		}
		line := strings.TrimRight(s[pos:next], "\r\n")
		switch trimmed := strings.TrimSpace(line); {
		case strings.HasPrefix(trimmed, gitignoreBegin):
			switch {
			case end >= 0:
				return -1, -1, fmt.Errorf("line %d: a second %s block", n, gitignoreBegin)
			case start >= 0:
				return -1, -1, fmt.Errorf("line %d: %s inside an open block", n, gitignoreBegin)
			}
			start, startLine = pos, n
		case trimmed == gitignoreEnd:
			if start < 0 || end >= 0 {
				return -1, -1, fmt.Errorf("line %d: %s without a %s before it", n, gitignoreEnd, gitignoreBegin)
			}
			end = pos + len(line)
		}
		pos = next
	}
	if start >= 0 && end < 0 {
		return -1, -1, fmt.Errorf("line %d: %s is never closed with %s", startLine, gitignoreBegin, gitignoreEnd)
	}
	return start, end, nil
}
