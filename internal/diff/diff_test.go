package diff

import (
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"
)

func TestUnified(t *testing.T) {
	tests := []struct {
		name     string
		old, new string
		want     string
	}{
		{"equal", "a\nb\n", "a\nb\n", ""},
		{"both empty", "", "", ""},
		{
			"new file", "", "a\nb\n",
			"--- old\n+++ new\n@@ -0,0 +1,2 @@\n+a\n+b\n",
		},
		{
			"emptied file", "a\n", "",
			"--- old\n+++ new\n@@ -1 +0,0 @@\n-a\n",
		},
		{
			"insertion", "a\nb\nc\n", "a\nb\nx\nc\n",
			"--- old\n+++ new\n@@ -1,3 +1,4 @@\n a\n b\n+x\n c\n",
		},
		{
			"removal", "a\nb\nc\n", "a\nc\n",
			"--- old\n+++ new\n@@ -1,3 +1,2 @@\n a\n-b\n c\n",
		},
		{
			"replacement", "a\nb\nc\n", "a\nB\nc\n",
			"--- old\n+++ new\n@@ -1,3 +1,3 @@\n a\n-b\n+B\n c\n",
		},
		{
			"context is cut to three lines",
			"1\n2\n3\n4\n5\n6\n7\n8\n9\n", "1\n2\n3\n4\nX\n6\n7\n8\n9\n",
			"--- old\n+++ new\n@@ -2,7 +2,7 @@\n 2\n 3\n 4\n-5\n+X\n 6\n 7\n 8\n",
		},
		{
			"close changes share a hunk",
			"1\n2\n3\n4\n5\n6\n7\n8\n", "X\n2\n3\n4\n5\n6\n7\nY\n",
			"--- old\n+++ new\n@@ -1,8 +1,8 @@\n-1\n+X\n 2\n 3\n 4\n 5\n 6\n 7\n-8\n+Y\n",
		},
		{
			"distant changes get separate hunks",
			"1\n2\n3\n4\n5\n6\n7\n8\n9\n", "X\n2\n3\n4\n5\n6\n7\n8\nY\n",
			"--- old\n+++ new\n@@ -1,4 +1,4 @@\n-1\n+X\n 2\n 3\n 4\n@@ -6,4 +6,4 @@\n 6\n 7\n 8\n-9\n+Y\n",
		},
		{
			"missing final newline added",
			"a\nb", "a\nb\n",
			"--- old\n+++ new\n@@ -1,2 +1,2 @@\n a\n-b\n" + noNewline + "+b\n",
		},
		{
			"missing final newline kept",
			"a\nb", "x\nb",
			"--- old\n+++ new\n@@ -1,2 +1,2 @@\n-a\n+x\n b\n" + noNewline,
		},
		{
			"line endings are compared",
			"a\r\n", "a\n",
			"--- old\n+++ new\n@@ -1 +1 @@\n-a\r\n+a\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Unified("old", "new", []byte(tt.old), []byte(tt.new))
			if got != tt.want {
				t.Errorf("Unified() =\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}

// TestUnifiedTooManyEdits checks that texts needing more than maxEdits edits
// are shown as wholly replaced.
func TestUnifiedTooManyEdits(t *testing.T) {
	var before, after strings.Builder
	n := maxEdits/2 + 1 // n removals and n insertions: more than maxEdits
	for i := range n {
		fmt.Fprintf(&before, "old %d\n", i)
		fmt.Fprintf(&after, "new %d\n", i)
	}
	got := Unified("old", "new", []byte(before.String()), []byte(after.String()))

	var want strings.Builder
	fmt.Fprintf(&want, "--- old\n+++ new\n@@ -1,%d +1,%d @@\n", n, n)
	for _, line := range splitLines([]byte(before.String())) {
		want.WriteString("-" + line)
	}
	for _, line := range splitLines([]byte(after.String())) {
		want.WriteString("+" + line)
	}
	if got != want.String() {
		t.Errorf("Unified() is not a whole replacement:\n%.500s", got)
	}
}

// TestDiffLinesShortest compares the edit scripts of random texts with the
// longest common subsequence computed by dynamic programming: every script
// must turn a into b, and keep as many lines as possible.
func TestDiffLinesShortest(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2)) //nolint:gosec // a fixed seed makes test inputs reproducible; nothing here is secret
	for i := range 2000 {
		a, b := randomLines(r), randomLines(r)
		edits := diffLines(a, b, maxEdits)

		var gotA, gotB []string
		kept := 0
		for _, e := range edits {
			switch e.kind {
			case keep:
				if a[e.a] != b[e.b] {
					t.Fatalf("case %d: kept lines differ: %q and %q", i, a[e.a], b[e.b])
				}
				gotA, gotB = append(gotA, a[e.a]), append(gotB, b[e.b])
				kept++
			case remove:
				gotA = append(gotA, a[e.a])
			case insert:
				gotB = append(gotB, b[e.b])
			}
		}
		if strings.Join(gotA, "") != strings.Join(a, "") || strings.Join(gotB, "") != strings.Join(b, "") {
			t.Fatalf("case %d: edits do not cover both texts in order\na = %q\nb = %q", i, a, b)
		}
		if want := lcs(a, b); kept != want {
			t.Fatalf("case %d: kept %d lines, the longest common subsequence has %d\na = %q\nb = %q", i, kept, want, a, b)
		}
	}
}

// randomLines returns up to 12 lines drawn from a small alphabet, so texts
// share many lines and the search has real choices to make.
func randomLines(r *rand.Rand) []string {
	lines := make([]string, r.IntN(13))
	for i := range lines {
		lines[i] = "abcd"[r.IntN(4):][:1] + "\n"
	}
	return lines
}

// lcs returns the length of the longest common subsequence of a and b.
func lcs(a, b []string) int {
	row := make([]int, len(b)+1)
	for i := range a {
		prev := 0 // row[j] of the previous i
		for j := range b {
			cur := row[j+1]
			if a[i] == b[j] {
				row[j+1] = prev + 1
			} else {
				row[j+1] = max(row[j+1], row[j])
			}
			prev = cur
		}
	}
	return row[len(b)]
}
