package detect

import (
	"regexp"
	"slices"
	"strings"

	"github.com/cyanotype-dev/cyanotype/internal/catalog"
)

// requirementName matches the project name at the start of a PEP 508
// requirement, such as "fastapi" in "fastapi[standard]>=0.115; python_version>'3.9'".
var requirementName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*`)

// projectName returns the normalized project name of a PEP 508
// requirement, or "" when it does not start with one, such as a URL.
func projectName(requirement string) string {
	return catalog.NormalizePyPI(requirementName.FindString(strings.TrimSpace(requirement)))
}

// parseRequirements returns the normalized project names of a
// requirements.txt, in file order. Options such as "-r other.txt" or
// "-e ./lib", and lines that are paths or URLs, name no project and are
// skipped; so are comments and blank lines. A line ending in a backslash
// continues on the next one.
func parseRequirements(data []byte) []string {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	text = strings.ReplaceAll(text, "\\\n", " ")
	var names []string
	for line := range strings.Lines(text) {
		fields := strings.Fields(stripRequirementComment(line))
		if len(fields) == 0 {
			continue
		}
		// "name @ https://..." names a project; a line that is a URL or a
		// path does not.
		if first := fields[0]; strings.HasPrefix(first, "-") || strings.Contains(first, "://") ||
			strings.HasPrefix(first, ".") || strings.HasPrefix(first, "/") {
			continue
		}
		line = strings.Join(fields, " ")
		if name := projectName(line); name != "" && !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	return names
}

// stripRequirementComment removes a comment from a requirements line: a #
// at the start, or after whitespace, as pip reads it.
func stripRequirementComment(line string) string {
	for i, r := range line {
		if r == '#' && (i == 0 || line[i-1] == ' ' || line[i-1] == '\t') {
			return line[:i]
		}
	}
	return line
}

// parsePyProject returns the normalized project names of every dependency
// a pyproject.toml lists, sorted and without duplicates. It reads:
//
//   - [project] dependencies, and every list of [project.optional-dependencies];
//   - every list of [dependency-groups] (PEP 735), skipping include-group tables;
//   - [tool.uv] dev-dependencies;
//   - the keys of Poetry's [tool.poetry.dependencies],
//     [tool.poetry.dev-dependencies] and [tool.poetry.group.<name>.dependencies],
//     except python.
//
// It is not a TOML parser: it understands the syntax these tables use
// (tables, keys, strings, arrays, inline tables, comments) and skips any
// other value without interpreting it. A full parser would mean a
// dependency for a few lists of strings, and detection is a suggestion the
// user confirms. Syntax it cannot follow, such as an unclosed string or
// array, is an error, and the file is reported as a Problem.
func parsePyProject(data []byte) ([]string, error) {
	s := &tomlScanner{src: strings.ReplaceAll(string(data), "\r\n", "\n"), line: 1}
	names := map[string]bool{}
	addRequirements := func(v tomlValue) {
		for _, item := range v.strings {
			if name := projectName(item); name != "" {
				names[name] = true
			}
		}
	}

	var table []string
	for {
		s.skipBlank()
		if s.done() {
			break
		}
		if s.peek() == '[' {
			t, err := s.tableHeader()
			if err != nil {
				return nil, err
			}
			table = t
			continue
		}
		key, err := s.key()
		if err != nil {
			return nil, err
		}
		v, err := s.value()
		if err != nil {
			return nil, err
		}
		// The key's full path, so "[project]" + "dependencies" and a
		// top-level "project.dependencies" mean the same.
		path := slices.Concat(table, key)
		switch {
		case isRequirementList(path):
			addRequirements(v)
		case isPoetryDependencyTable(path[:len(path)-1]) && path[len(path)-1] != "python":
			names[catalog.NormalizePyPI(path[len(path)-1])] = true
		}
		if err := s.endOfLine(); err != nil {
			return nil, err
		}
	}
	out := make([]string, 0, len(names))
	for name := range names {
		out = append(out, name)
	}
	slices.Sort(out)
	return out, nil
}

// isRequirementList reports whether the key at path holds a list of PEP
// 508 requirements.
func isRequirementList(path []string) bool {
	switch {
	case slices.Equal(path, []string{"project", "dependencies"}),
		slices.Equal(path, []string{"tool", "uv", "dev-dependencies"}):
		return true
	case len(path) == 3 && path[0] == "project" && path[1] == "optional-dependencies":
		return true
	case len(path) == 2 && path[0] == "dependency-groups":
		return true
	}
	return false
}

// isPoetryDependencyTable reports whether the table at path holds Poetry
// dependencies as keys.
func isPoetryDependencyTable(path []string) bool {
	switch {
	case slices.Equal(path, []string{"tool", "poetry", "dependencies"}),
		slices.Equal(path, []string{"tool", "poetry", "dev-dependencies"}):
		return true
	case len(path) == 5 && path[0] == "tool" && path[1] == "poetry" && path[2] == "group" && path[4] == "dependencies":
		return true
	}
	return false
}

// tomlValue is a value read by tomlScanner: the strings it holds, directly
// or as items of an array. Other values, such as numbers, booleans and
// inline tables, are read past and leave strings empty.
type tomlValue struct {
	strings []string
}

// tomlScanner reads the parts of TOML that pyproject dependency tables use.
type tomlScanner struct {
	src  string
	pos  int
	line int
}

func (s *tomlScanner) done() bool { return s.pos >= len(s.src) }

func (s *tomlScanner) peek() byte {
	if s.done() {
		return 0
	}
	return s.src[s.pos]
}

func (s *tomlScanner) next() byte {
	c := s.src[s.pos]
	s.pos++
	if c == '\n' {
		s.line++
	}
	return c
}

// skipSpace skips spaces and tabs on the current line.
func (s *tomlScanner) skipSpace() {
	for !s.done() && (s.peek() == ' ' || s.peek() == '\t') {
		s.next()
	}
}

// skipComment skips a comment up to the end of the line, if one starts here.
func (s *tomlScanner) skipComment() {
	if s.peek() != '#' {
		return
	}
	for !s.done() && s.peek() != '\n' {
		s.next()
	}
}

// skipBlank skips whitespace, newlines and comments.
func (s *tomlScanner) skipBlank() {
	for !s.done() {
		switch s.peek() {
		case ' ', '\t', '\n':
			s.next()
		case '#':
			s.skipComment()
		default:
			return
		}
	}
}

// endOfLine checks that nothing but a comment follows a key's value.
func (s *tomlScanner) endOfLine() error {
	s.skipSpace()
	s.skipComment()
	if !s.done() && s.peek() != '\n' {
		return lineError(s.line, "unexpected %q after a value", s.peek())
	}
	return nil
}

// tableHeader reads "[a.b]" or "[[a.b]]" and returns the parts of its name.
func (s *tomlScanner) tableHeader() ([]string, error) {
	line := s.line
	s.next() // [
	array := s.peek() == '['
	if array {
		s.next()
	}
	name, err := s.dottedKey(']')
	if err != nil {
		return nil, err
	}
	s.next() // ], checked by dottedKey
	if array {
		if s.peek() != ']' {
			return nil, lineError(line, "table header is not closed")
		}
		s.next()
	}
	return name, s.endOfLine()
}

// key reads a key and the "=" after it, and returns the parts of the key.
func (s *tomlScanner) key() ([]string, error) {
	k, err := s.dottedKey('=')
	if err != nil {
		return nil, err
	}
	s.next() // =, checked by dottedKey
	return k, nil
}

// dottedKey reads bare or quoted key parts joined by dots, up to stop, and
// returns the parts without quotes. A quoted part may hold dots, as in
// "zope.interface", so the parts are never joined into one string.
func (s *tomlScanner) dottedKey(stop byte) ([]string, error) {
	var parts []string
	for {
		s.skipSpace()
		var part string
		switch s.peek() {
		case '"', '\'':
			str, err := s.str()
			if err != nil {
				return nil, err
			}
			part = str
		default:
			start := s.pos
			for !s.done() && isBareKeyChar(s.peek()) {
				s.next()
			}
			part = s.src[start:s.pos]
			if part == "" {
				return nil, lineError(s.line, "expected a key")
			}
		}
		parts = append(parts, part)
		s.skipSpace()
		if s.peek() != '.' {
			break
		}
		s.next()
	}
	if s.peek() != stop {
		return nil, lineError(s.line, "expected %q after %q", stop, strings.Join(parts, "."))
	}
	return parts, nil
}

func isBareKeyChar(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_'
}

// value reads one value: a string, an array, an inline table, or any other
// scalar, which is read up to the next comma, bracket, brace, comment or
// end of line.
func (s *tomlScanner) value() (tomlValue, error) {
	s.skipSpace()
	switch s.peek() {
	case '"', '\'':
		str, err := s.str()
		return tomlValue{strings: []string{str}}, err
	case '[':
		return s.array()
	case '{':
		return tomlValue{}, s.inlineTable()
	}
	start := s.pos
	for !s.done() && !strings.ContainsRune(",]}#\n", rune(s.peek())) {
		s.next()
	}
	if strings.TrimSpace(s.src[start:s.pos]) == "" {
		return tomlValue{}, lineError(s.line, "expected a value")
	}
	return tomlValue{}, nil
}

// array reads an array, possibly over several lines with comments and a
// trailing comma, and returns the strings among its items.
func (s *tomlScanner) array() (tomlValue, error) {
	line := s.line
	s.next() // [
	var v tomlValue
	for {
		s.skipBlank()
		if s.done() {
			return v, lineError(line, "array is never closed")
		}
		if s.peek() == ']' {
			s.next()
			return v, nil
		}
		item, err := s.value()
		if err != nil {
			return v, err
		}
		v.strings = append(v.strings, item.strings...)
		s.skipBlank()
		switch s.peek() {
		case ',':
			s.next()
		case ']':
		default:
			return v, lineError(s.line, "expected , or ] in the array opened on line %d", line)
		}
	}
}

// inlineTable reads past an inline table such as { include-group = "test" }.
func (s *tomlScanner) inlineTable() error {
	line := s.line
	s.next() // {
	for {
		s.skipSpace()
		if s.done() || s.peek() == '\n' {
			return lineError(line, "inline table is never closed")
		}
		if s.peek() == '}' {
			s.next()
			return nil
		}
		if _, err := s.key(); err != nil {
			return err
		}
		if _, err := s.value(); err != nil {
			return err
		}
		s.skipSpace()
		if s.peek() == ',' {
			s.next()
		}
	}
}

// str reads a basic ("...") or literal ('...') string, single-line or
// multi-line, and returns its content. Escapes in basic strings are
// resolved only as far as a project name needs: an escaped character
// stands for itself.
func (s *tomlScanner) str() (string, error) {
	line := s.line
	quote := s.next()
	multi := strings.HasPrefix(s.src[s.pos:], string([]byte{quote, quote}))
	if multi {
		s.pos += 2
	}
	var b strings.Builder
	for {
		if s.done() || (!multi && s.peek() == '\n') {
			return "", lineError(line, "string is never closed")
		}
		c := s.next()
		switch {
		case c == '\\' && quote == '"' && !s.done():
			b.WriteByte(s.next())
		case c == quote && !multi:
			return b.String(), nil
		case c == quote && strings.HasPrefix(s.src[s.pos:], string([]byte{quote, quote})):
			s.pos += 2
			return b.String(), nil
		default:
			b.WriteByte(c)
		}
	}
}
