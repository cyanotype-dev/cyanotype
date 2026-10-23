package render

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/cyanotype-dev/cyanotype/internal/lock"
)

// Owned returns the part of a generated file that cyanotype owns, in the
// form cyanotype.lock hashes it, and false for a file whose content is not
// hashed: .claude/settings.json, where cyanotype owns rules rather than
// text, and the lock itself. The same function serves the files cyanotype
// renders and the files on disk, so their hashes compare.
//
//   - A Markdown file is owned whole, except its protected blocks. Line
//     endings become LF, a byte order mark is dropped, the blocks go,
//     markers included, runs of blank lines become one and blank lines at
//     either end go. So the hash survives a CRLF checkout and any text
//     written inside, or moved with, a cyanotype:custom block.
//   - .gitignore is owned only between its cyanotype:begin and
//     cyanotype:end lines; false when the file has no block.
//
// A protected block that is never closed is an error, as when rendering.
func Owned(path string, content []byte) ([]byte, bool, error) {
	switch path {
	case settingsPath, lock.Path:
		return nil, false, nil
	case gitignorePath:
		s := string(content)
		start, end, err := findGitignoreBlock(s)
		if err != nil {
			return nil, false, fmt.Errorf("%s: %w", path, err)
		}
		if start < 0 {
			return nil, false, nil
		}
		return []byte(strings.ReplaceAll(s[start:end], "\r\n", "\n") + "\n"), true, nil
	}
	owned, err := ownedMarkdown(content)
	if err != nil {
		return nil, false, fmt.Errorf("%s: %w", path, inFile(err, path))
	}
	return owned, true, nil
}

// ownedMarkdown returns content without its protected blocks, normalized
// as Owned describes.
func ownedMarkdown(content []byte) ([]byte, error) {
	if _, err := parseCustomBlocks(content); err != nil {
		return nil, err
	}
	text := strings.ReplaceAll(string(bytes.TrimPrefix(content, []byte("\xef\xbb\xbf"))), "\r\n", "\n")

	var out []string
	inBlock, blank := false, false
	for line := range strings.SplitSeq(strings.TrimSuffix(text, "\n"), "\n") {
		switch trimmed := strings.TrimSpace(line); {
		case trimmed == customBegin:
			inBlock = true
		case trimmed == customEnd:
			inBlock = false
		case inBlock:
		case trimmed == "":
			blank = len(out) > 0
		default:
			if blank {
				out = append(out, "")
			}
			out = append(out, line)
			blank = false
		}
	}
	if len(out) == 0 {
		return nil, nil
	}
	return []byte(strings.Join(out, "\n") + "\n"), nil
}

// ownedHash returns lock.Hash of the part of content cyanotype owns, and
// false when the file has none; see Owned.
func ownedHash(path string, content []byte) (string, bool, error) {
	owned, ok, err := Owned(path, content)
	if err != nil || !ok {
		return "", false, err
	}
	return lock.Hash(owned), true, nil
}
