package catalog

import (
	"regexp"
	"slices"
	"strings"
)

// PlaceholderLanguages is filled with the names of the selected entries in
// the language category, such as "Go and TypeScript".
const PlaceholderLanguages = "languages"

// placeholderNames lists every placeholder a snippet may use.
var placeholderNames = []string{PlaceholderLanguages}

// placeholder matches {name} in snippet text.
var placeholder = regexp.MustCompile(`\{([a-z][a-z0-9_]*)\}`)

// Placeholders returns the names a snippet may write as {name}. The catalog
// rejects any other name, and internal/resolve fills each of these with
// text that depends on the spec.
func Placeholders() []string {
	return slices.Clone(placeholderNames)
}

// UsedPlaceholders returns the names of the placeholders in text, in order
// of first use and without repeats, known or not. Text inside backticks is
// code and stays literal, so `{children}` in a JSX example is not a
// placeholder.
func UsedPlaceholders(text string) []string {
	var names []string
	forEachProse(text, func(prose string) string {
		for _, m := range placeholder.FindAllStringSubmatch(prose, -1) {
			if !slices.Contains(names, m[1]) {
				names = append(names, m[1])
			}
		}
		return prose
	})
	return names
}

// FillPlaceholders replaces every {name} in text that has a value in
// values, outside backticks. Other placeholders are left as they are.
func FillPlaceholders(text string, values map[string]string) string {
	return forEachProse(text, func(prose string) string {
		return placeholder.ReplaceAllStringFunc(prose, func(m string) string {
			if v, ok := values[m[1:len(m)-1]]; ok {
				return v
			}
			return m
		})
	})
}

// forEachProse calls f on each part of text outside backticks and returns
// text with those parts replaced by what f returns. Splitting on every
// backtick also handles ``` fences: three toggles leave the state flipped.
func forEachProse(text string, f func(prose string) string) string {
	parts := strings.Split(text, "`")
	for i := 0; i < len(parts); i += 2 {
		parts[i] = f(parts[i])
	}
	return strings.Join(parts, "`")
}
