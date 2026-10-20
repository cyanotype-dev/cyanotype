package detect

import (
	"strconv"
	"strings"
)

// parseGoMod returns the module paths a go.mod requires directly, in file
// order. Requirements marked "// indirect" are left out: they are
// dependencies of dependencies, which say nothing about how the project
// itself is built.
//
// It reads the require directives only, one per line or in a
// parenthesized block, which is all detection needs; golang.org/x/mod
// would parse the whole file, at the cost of a dependency.
func parseGoMod(data []byte) ([]string, error) {
	var (
		modules []string
		inBlock bool
		start   int // line of the open block
	)
	for i, line := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		code, comment, _ := strings.Cut(line, "//")
		fields := strings.Fields(code)
		indirect := strings.TrimSpace(comment) == "indirect" || strings.HasPrefix(strings.TrimSpace(comment), "indirect;")

		switch {
		case inBlock && len(fields) == 1 && fields[0] == ")":
			inBlock = false
			continue
		case inBlock:
		case opensRequireBlock(code):
			inBlock, start = true, i+1
			continue
		case len(fields) > 0 && fields[0] == "require":
			fields = fields[1:]
		default:
			continue
		}
		if len(fields) == 0 || indirect {
			continue
		}
		path := fields[0]
		if unquoted, err := strconv.Unquote(path); err == nil {
			path = unquoted
		}
		modules = append(modules, path)
	}
	if inBlock {
		return modules, lineError(start, "require block is never closed")
	}
	return modules, nil
}

// opensRequireBlock reports whether a line of go.mod, without its comment,
// is "require (", with or without the space.
func opensRequireBlock(code string) bool {
	rest, ok := strings.CutPrefix(strings.TrimSpace(code), "require")
	return ok && strings.TrimSpace(rest) == "("
}
