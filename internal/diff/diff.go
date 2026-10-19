// Package diff compares two texts line by line and formats the difference
// as a unified diff, the format of git diff and diff -u.
//
// Go's standard library has no diff package, and the generated files
// cyanotype compares are a few hundred lines at most, so a small
// implementation of Myers' algorithm is enough and adds no dependency.
// See "An O(ND) Difference Algorithm and Its Variations", Eugene W. Myers,
// Algorithmica 1(2), 1986.
package diff

import (
	"bytes"
	"fmt"
	"slices"
	"strings"
)

// contextLines is how many unchanged lines a hunk shows around each change,
// as diff -u and git diff do by default.
const contextLines = 3

// maxEdits bounds the search for the shortest edit script. Its memory grows
// with the square of the number of edits, so beyond this many the texts are
// shown as wholly replaced: at that point the result is a rewrite anyway.
const maxEdits = 1000

// noNewline marks a last line without a line ending, as diff -u does.
const noNewline = "\\ No newline at end of file\n"

// Unified returns the unified diff that turns before into after, with
// oldName and newName in the "---" and "+++" headers, such as "a/AGENTS.md"
// or "/dev/null" for a file that does not exist yet. It returns "" when the
// texts are equal.
//
// Lines are compared byte for byte, line endings included, so a caller that
// wants to ignore CRLF must convert it first.
func Unified(oldName, newName string, before, after []byte) string {
	if bytes.Equal(before, after) {
		return ""
	}
	a, b := splitLines(before), splitLines(after)
	edits := diffLines(a, b, maxEdits)

	var out strings.Builder
	fmt.Fprintf(&out, "--- %s\n+++ %s\n", oldName, newName)
	for _, h := range hunks(edits) {
		writeHunk(&out, a, b, edits[h.start:h.end])
	}
	return out.String()
}

// splitLines splits text into lines, each keeping its "\n". Only the last
// line can lack one.
func splitLines(text []byte) []string {
	if len(text) == 0 {
		return nil
	}
	lines := strings.SplitAfter(string(text), "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// kind says what an edit does to one line.
type kind byte

const (
	keep   kind = ' ' // the line is in both texts
	remove kind = '-' // the line is only in the old text
	insert kind = '+' // the line is only in the new text
)

// edit is one step of an edit script. a and b are positions in the old and
// new lines: the line itself for the side that has it, and for the other
// side the number of lines before it, which hunk headers need.
type edit struct {
	kind kind
	a, b int
}

// diffLines returns an edit script turning a into b. Lines the two texts
// share at the start and end are kept without searching, which is most of a
// regenerated file; the rest goes to Myers' algorithm. When the middle needs
// more than limit edits, it is replaced wholesale.
func diffLines(a, b []string, limit int) []edit {
	prefix := 0
	for prefix < len(a) && prefix < len(b) && a[prefix] == b[prefix] {
		prefix++
	}
	suffix := 0
	for suffix < len(a)-prefix && suffix < len(b)-prefix && a[len(a)-1-suffix] == b[len(b)-1-suffix] {
		suffix++
	}
	midA, midB := a[prefix:len(a)-suffix], b[prefix:len(b)-suffix]

	edits := make([]edit, 0, len(a)+len(b))
	for i := range prefix {
		edits = append(edits, edit{keep, i, i})
	}
	middle, ok := myers(midA, midB, limit)
	if !ok {
		middle = replaceAll(len(midA), len(midB))
	}
	for _, e := range middle {
		edits = append(edits, edit{e.kind, e.a + prefix, e.b + prefix})
	}
	for i := range suffix {
		edits = append(edits, edit{keep, len(a) - suffix + i, len(b) - suffix + i})
	}
	return edits
}

// replaceAll removes all n old lines, then inserts all m new ones.
func replaceAll(n, m int) []edit {
	edits := make([]edit, 0, n+m)
	for i := range n {
		edits = append(edits, edit{remove, i, 0})
	}
	for j := range m {
		edits = append(edits, edit{insert, n, j})
	}
	return edits
}

// myers finds a shortest edit script turning a into b, or reports false when
// it needs more than limit edits.
//
// The search walks a grid where moving right removes a line of a, moving
// down inserts a line of b, and moving diagonally keeps a line both share.
// For each number of edits d it records, on every diagonal k = x - y, the
// furthest x a path with d edits reaches. The first d whose path reaches the
// bottom-right corner is the shortest; the recorded rows let backtrack
// retrace that path.
func myers(a, b []string, limit int) ([]edit, bool) {
	n, m := len(a), len(b)
	if n == 0 && m == 0 {
		return nil, true
	}
	// v[off+k] is the furthest x on diagonal k. Diagonals run from -d to d,
	// and each step reads its neighbors, hence the extra slot on each side.
	off := limit + 1
	v := make([]int, 2*off+1)
	// trace[d] holds v[-d..d] after d edits: only the diagonals reachable
	// with d edits, so memory grows with d squared rather than d times n+m.
	var trace [][]int
	for d := 0; d <= limit; d++ {
		for k := -d; k <= d; k += 2 {
			// Step down from diagonal k+1 (an insertion) or right from
			// diagonal k-1 (a removal), whichever reached further.
			var x int
			if k == -d || (k != d && v[off+k-1] < v[off+k+1]) {
				x = v[off+k+1]
			} else {
				x = v[off+k-1] + 1
			}
			// Then follow the diagonal while the lines match.
			y := x - k
			for x < n && y < m && a[x] == b[y] {
				x++
				y++
			}
			v[off+k] = x
			if x >= n && y >= m {
				trace = append(trace, slices.Clone(v[off-d:off+d+1]))
				return backtrack(trace, n, m), true
			}
		}
		trace = append(trace, slices.Clone(v[off-d:off+d+1]))
	}
	return nil, false
}

// backtrack retraces the path myers found, from the bottom-right corner
// (n, m) back to (0, 0), and returns its edits in order.
func backtrack(trace [][]int, n, m int) []edit {
	var edits []edit
	x, y := n, m
	for d := len(trace) - 1; d >= 0; d-- {
		k := x - y
		// Where the d-th edit started; for d == 0 there is no edit, only
		// the diagonal from the origin.
		startX, startY := 0, 0
		var step edit
		if d > 0 {
			prev := trace[d-1]
			at := func(k int) int { return prev[k+d-1] }
			if k == -d || (k != d && at(k-1) < at(k+1)) {
				startX = at(k + 1)
				startY = startX - (k + 1)
				step = edit{insert, startX, startY}
				startY++
			} else {
				startX = at(k - 1)
				startY = startX - (k - 1)
				step = edit{remove, startX, startY}
				startX++
			}
		}
		for x > startX && y > startY {
			x--
			y--
			edits = append(edits, edit{keep, x, y})
		}
		if d > 0 {
			edits = append(edits, step)
			x, y = step.a, step.b
		}
	}
	slices.Reverse(edits)
	return edits
}

// span is a hunk: the edits from start up to, not including, end.
type span struct{ start, end int }

// hunks groups the changes in edits with contextLines of kept lines around
// them. Changes separated by at most twice contextLines kept lines share a
// hunk, since their contexts would otherwise touch or overlap.
func hunks(edits []edit) []span {
	var out []span
	for i := 0; i < len(edits); {
		if edits[i].kind == keep {
			i++
			continue
		}
		start := max(i-contextLines, 0)
		j := i
		for {
			for j < len(edits) && edits[j].kind != keep {
				j++
			}
			next := j
			for next < len(edits) && edits[next].kind == keep {
				next++
			}
			if next == len(edits) || next-j > 2*contextLines {
				break
			}
			j = next
		}
		end := min(j+contextLines, len(edits))
		out = append(out, span{start, end})
		i = end
	}
	return out
}

// writeHunk writes one hunk: its "@@" header, then each line with its
// prefix.
func writeHunk(out *strings.Builder, a, b []string, edits []edit) {
	oldCount, newCount := 0, 0
	for _, e := range edits {
		if e.kind != insert {
			oldCount++
		}
		if e.kind != remove {
			newCount++
		}
	}
	fmt.Fprintf(out, "@@ -%s +%s @@\n", hunkRange(edits[0].a, oldCount), hunkRange(edits[0].b, newCount))
	for _, e := range edits {
		var line string
		if e.kind == insert {
			line = b[e.b]
		} else {
			line = a[e.a]
		}
		out.WriteByte(byte(e.kind))
		out.WriteString(line)
		if !strings.HasSuffix(line, "\n") {
			out.WriteString("\n" + noNewline)
		}
	}
}

// hunkRange formats one side of a hunk header. start is the 0-based position
// of the hunk's first line. Lines are numbered from 1, a count of 1 is left
// out, and an empty side names the line before it, as diff -u does.
func hunkRange(start, count int) string {
	switch count {
	case 0:
		return fmt.Sprintf("%d,0", start)
	case 1:
		return fmt.Sprintf("%d", start+1)
	}
	return fmt.Sprintf("%d,%d", start+1, count)
}
