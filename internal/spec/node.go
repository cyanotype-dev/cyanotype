package spec

import (
	"fmt"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

// This file holds helpers that walk the yaml.Node tree of a spec file. The
// tree keeps what Go structs lose: line numbers, comments and key order.

// root returns the top-level mapping of a document node.
func root(doc *yaml.Node) *yaml.Node {
	if doc == nil || doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return nil
	}
	return doc.Content[0]
}

// mappingPair returns the key and value nodes of key in the mapping m, or
// nils when m is not a mapping or has no such key.
func mappingPair(m *yaml.Node, key string) (k, v *yaml.Node) {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil, nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i], m.Content[i+1]
		}
	}
	return nil, nil
}

// pathSegment is one step of a field path: a mapping key, or a list index
// when key is empty.
type pathSegment struct {
	key   string
	index int
}

// splitPath parses a field path such as "custom_rules[1].why" into segments.
func splitPath(field string) []pathSegment {
	var segs []pathSegment
	for part := range strings.SplitSeq(field, ".") {
		key, rest, _ := strings.Cut(part, "[")
		if key != "" {
			segs = append(segs, pathSegment{key: key})
		}
		for rest != "" {
			num, after, _ := strings.Cut(rest, "]")
			i, err := strconv.Atoi(num)
			if err != nil {
				return segs
			}
			segs = append(segs, pathSegment{index: i})
			rest = strings.TrimPrefix(after, "[")
		}
	}
	return segs
}

// lineOf returns the line of the field at path in doc. When the field is
// missing it returns the line of its closest existing parent, and 0 when
// not even the first segment exists.
func lineOf(doc *yaml.Node, field string) int {
	node, line := root(doc), 0
	for _, seg := range splitPath(field) {
		switch {
		case seg.key != "":
			k, v := mappingPair(node, seg.key)
			if k == nil {
				return line
			}
			// The key's line, not the value's: a block value starts below it.
			node, line = v, k.Line
		case node != nil && node.Kind == yaml.SequenceNode && seg.index < len(node.Content):
			node = node.Content[seg.index]
			line = node.Line
		default:
			return line
		}
	}
	return line
}

// fieldAtLine returns the path of the outermost field that starts on line,
// or "" if none does. It maps yaml.v3 errors, which carry only a line, back
// to a field.
func fieldAtLine(doc *yaml.Node, line int) string {
	var find func(n *yaml.Node, path string) (string, bool)
	find = func(n *yaml.Node, path string) (string, bool) {
		switch n.Kind {
		case yaml.MappingNode:
			for i := 0; i+1 < len(n.Content); i += 2 {
				k, v := n.Content[i], n.Content[i+1]
				p := k.Value
				if path != "" {
					p = path + "." + k.Value
				}
				if k.Line == line {
					return p, true
				}
				if found, ok := find(v, p); ok {
					return found, true
				}
			}
		case yaml.SequenceNode:
			for i, item := range n.Content {
				p := fmt.Sprintf("%s[%d]", path, i)
				if item.Line == line && item.Kind != yaml.MappingNode {
					return p, true
				}
				if found, ok := find(item, p); ok {
					return found, true
				}
			}
		}
		return "", false
	}
	if r := root(doc); r != nil {
		if p, ok := find(r, ""); ok {
			return p
		}
	}
	return ""
}
