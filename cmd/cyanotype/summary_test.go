package main

import (
	"testing"

	"github.com/cyanotype-dev/cyanotype/internal/generate"
)

func TestWroteSummary(t *testing.T) {
	written := generate.Change{Status: generate.Modified}
	deleted := generate.Change{Status: generate.Deleted}
	tests := []struct {
		changes []generate.Change
		want    string
	}{
		{[]generate.Change{written}, "Wrote 1 file."},
		{[]generate.Change{written, written, {Status: generate.Unchanged}}, "Wrote 2 files."},
		{[]generate.Change{written, written, deleted}, "Wrote 2 files and deleted 1."},
		{[]generate.Change{deleted}, "Deleted 1 file."},
	}
	for _, tt := range tests {
		n := 0
		for _, c := range tt.changes {
			if c.Status != generate.Unchanged {
				n++
			}
		}
		if got := wroteSummary(tt.changes, n); got != tt.want {
			t.Errorf("wroteSummary(%v) = %q, want %q", tt.changes, got, tt.want)
		}
	}
}
