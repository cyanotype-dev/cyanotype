package spec

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

// ValidationError describes one problem in a spec file.
type ValidationError struct {
	// Field is a YAML path such as "project.core_features[1]"; empty for
	// problems that concern the whole file.
	Field string
	// Line is the 1-based line of the field, or of its closest existing
	// parent when the field is missing; 0 when unknown.
	Line int
	Msg  string
	// Syntax reports a YAML syntax error. yaml.v3 reports the line of some
	// syntax errors one off, so Line is only approximate: interfaces should
	// say "near line" and show the lines around it.
	Syntax bool
}

func (e *ValidationError) Error() string {
	var b strings.Builder
	switch {
	case e.Line > 0 && e.Syntax:
		fmt.Fprintf(&b, "near line %d: ", e.Line)
	case e.Line > 0:
		fmt.Fprintf(&b, "line %d: ", e.Line)
	}
	if e.Field != "" {
		b.WriteString(e.Field + ": ")
	}
	b.WriteString(e.Msg)
	return b.String()
}

var (
	syntaxError   = regexp.MustCompile(`^yaml: line (\d+): (.*)$`)
	typeErrorLine = regexp.MustCompile(`^line (\d+): (.*)$`)
	unknownField  = regexp.MustCompile(`^field (\S+) not found in type `)
	cannotDecode  = regexp.MustCompile("^cannot unmarshal (!!\\w+)(?: `(.*)`)? into (\\S+)$")
)

// syntaxErrors turns a YAML parse error into a ValidationError marked as a
// syntax error, whose line may be one off.
func syntaxErrors(err error) []error {
	msg := strings.TrimPrefix(err.Error(), "yaml: ")
	line := 0
	if m := syntaxError.FindStringSubmatch(err.Error()); m != nil {
		line, _ = strconv.Atoi(m[1])
		msg = m[2]
	}
	return []error{&ValidationError{Line: line, Msg: "invalid YAML: " + msg, Syntax: true}}
}

// decodeErrors turns the errors of a strict decode into one ValidationError
// per problem. yaml.v3 reports only a line and Go type names, so the field
// is found in doc by line and the message is reworded in YAML terms.
func decodeErrors(doc *yaml.Node, err error) []error {
	var typeErr *yaml.TypeError
	if !errors.As(err, &typeErr) {
		return syntaxErrors(err)
	}
	errs := make([]error, 0, len(typeErr.Errors))
	for _, msg := range typeErr.Errors {
		ve := &ValidationError{Msg: msg}
		if m := typeErrorLine.FindStringSubmatch(msg); m != nil {
			ve.Line, _ = strconv.Atoi(m[1])
			ve.Field = fieldAtLine(doc, ve.Line)
			ve.Msg = m[2]
		}
		switch m := unknownField.FindStringSubmatch(ve.Msg); {
		case m != nil:
			ve.Msg = fmt.Sprintf("unknown field %q; check for a typo, or upgrade cyanotype if a newer version wrote this file", m[1])
		default:
			if m := cannotDecode.FindStringSubmatch(ve.Msg); m != nil {
				ve.Msg = fmt.Sprintf("expected %s, got %s", describeGoType(m[3]), describeTag(m[1], m[2]))
			}
		}
		errs = append(errs, ve)
	}
	return errs
}

// namedTypes lets describeGoType tell structs from strings by type name.
var namedTypes = map[string]reflect.Kind{}

func init() {
	for _, t := range []reflect.Type{
		reflect.TypeFor[Spec](), reflect.TypeFor[Project](), reflect.TypeFor[Languages](),
		reflect.TypeFor[I18n](), reflect.TypeFor[CustomRule](), reflect.TypeFor[Output](),
		reflect.TypeFor[Target](), reflect.TypeFor[Mode](), reflect.TypeFor[TranslationsBy](),
		reflect.TypeFor[Split](),
	} {
		namedTypes[t.String()] = t.Kind()
	}
}

// describeGoType names a Go type as YAML users know it: "[]spec.Target" is a list.
func describeGoType(name string) string {
	name = strings.TrimPrefix(name, "*")
	switch {
	case strings.HasPrefix(name, "[]"):
		return "a list"
	case strings.HasPrefix(name, "map["), namedTypes[name] == reflect.Struct:
		return "a mapping"
	case name == "string", namedTypes[name] == reflect.String:
		return "text"
	case name == "int":
		return "an integer"
	case name == "bool":
		return "true or false"
	}
	return name
}

// describeTag names a YAML tag, adding the value for scalars.
func describeTag(tag, value string) string {
	names := map[string]string{
		"!!seq": "a list", "!!map": "a mapping", "!!str": "text",
		"!!int": "an integer", "!!float": "a number", "!!bool": "a boolean",
	}
	name, ok := names[tag]
	if !ok {
		name = tag
	}
	if value != "" {
		name += fmt.Sprintf(" %q", value)
	}
	return name
}

// withLines fills in the line of each ValidationError from doc.
func withLines(doc *yaml.Node, errs []error) []error {
	for _, err := range errs {
		var ve *ValidationError
		if errors.As(err, &ve) && ve.Line == 0 && ve.Field != "" {
			ve.Line = lineOf(doc, ve.Field)
		}
	}
	return errs
}
