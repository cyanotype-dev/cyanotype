package resolve

// suggest returns the candidate closest to input, for "did you mean"
// hints. A candidate qualifies when reaching it changes at most a third of
// the input's characters, and at least one; on a tie the candidate listed
// first wins, so callers pass candidates sorted.
func suggest(input string, candidates []string) (string, bool) {
	limit := max(1, len([]rune(input))/3)
	best, bestDist := "", limit+1
	for _, c := range candidates {
		if d := editDistance(input, c); d < bestDist {
			best, bestDist = c, d
		}
	}
	return best, best != ""
}

// editDistance returns the Levenshtein distance between a and b: the
// fewest single-character insertions, deletions and substitutions that
// turn a into b. It keeps one row of the usual table at a time.
func editDistance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	row := make([]int, len(rb)+1)
	for j := range row {
		row[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		diag := row[0] // the cell above-left of the one being computed
		row[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			above := row[j]
			row[j] = min(above+1, row[j-1]+1, diag+cost)
			diag = above
		}
	}
	return row[len(rb)]
}
