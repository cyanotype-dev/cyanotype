package render

import (
	"io/fs"
	"regexp"
	"slices"
	"strings"
)

// readPointer matches the pointers generated files use to send the agent to
// another file, such as "- Before writing or changing tests, read
// docs/agents/testing.md." or the pointer to a moved file, "- Before
// starting a task, read docs/agents/original-claude.md, which holds ...".
// The path ends at the period or comma that follows it.
var readPointer = regexp.MustCompile(`^- .*\bread (\S+?)[.,](?:\s|$)`)

// References returns the project files a generated Markdown file sends the
// agent to, in order and without repeats: imports such as "@AGENTS.md",
// and the paths of "read <path>" pointers, which Further reading and the
// pointer to a moved file use. Protected blocks are included, since the
// pointer to a moved file lives in one. Only paths inside the project are
// returned, and only words that look like paths, with a "." or a "/", so
// a hand-written "read carefully." is not taken for a file.
func References(content []byte) []string {
	var refs []string
	add := func(p string) {
		p = strings.TrimPrefix(p, "./")
		if fs.ValidPath(p) && strings.ContainsAny(p, "./") && !slices.Contains(refs, p) {
			refs = append(refs, p)
		}
	}
	text := strings.ReplaceAll(string(content), "\r\n", "\n")
	for line := range strings.SplitSeq(text, "\n") {
		line = strings.TrimSpace(line)
		if p, ok := strings.CutPrefix(line, "@"); ok && p != "" && !strings.ContainsAny(p, " \t") {
			add(p)
			continue
		}
		if m := readPointer.FindStringSubmatch(line); m != nil {
			add(m[1])
		}
	}
	return refs
}
