package render

import "slices"

// mergeOrdered adds the items of groups to existing, which it keeps
// unchanged at the start. It serves every generated list where order
// matters: Claude permission rules, where Read(!.env.example) only narrows
// the rules listed before it, and the .gitignore block, where !.env.example
// only re-includes what the lines before it ignore.
//
// Each group's items end up in the result in their original order, as a
// subsequence: an item already present is reused only when it comes after
// the previous item of the same group, otherwise it is added again at the
// end. That keeps every negation after the items it narrows, whatever order
// the existing list has, at the cost of an occasional duplicate. Merging
// the result again changes nothing.
func mergeOrdered(existing []string, groups [][]string) []string {
	out := slices.Clone(existing)
	for _, group := range groups {
		last := -1
		for _, item := range group {
			i := indexAfter(out, item, last)
			if i < 0 {
				out = append(out, item)
				i = len(out) - 1
			}
			last = i
		}
	}
	return out
}

// indexAfter returns the first index greater than after where list holds
// s, or -1.
func indexAfter(list []string, s string, after int) int {
	for i := after + 1; i < len(list); i++ {
		if list[i] == s {
			return i
		}
	}
	return -1
}
