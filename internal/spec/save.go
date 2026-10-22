package spec

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"
)

// maxFlowWidth is the longest line a flow sequence such as
// "targets: [claude, cursor]" may occupy before Save writes it as a block
// list, one item per line.
const maxFlowWidth = 80

// Save validates the spec and writes it to w in the canonical layout.
//
// For a loaded file, the spec is merged into the original YAML tree, so the
// user's comments, key order and quoting survive. The layout is always
// canonical: two-space indentation, one blank line between top-level keys
// and none elsewhere, and flow sequences longer than maxFlowWidth written
// as block lists. See the Project spec section of docs/DESIGN.md.
func (f *File) Save(w io.Writer) error {
	if err := Validate(f.Spec); err != nil {
		return err
	}

	var fresh yaml.Node
	if err := fresh.Encode(f.Spec); err != nil {
		return fmt.Errorf("encode spec: %w", err)
	}
	quoteCatalogVersion(&fresh)
	if f.doc == nil {
		f.doc = &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{&fresh}}
	} else {
		merge(root(f.doc), &fresh)
	}

	top := root(f.doc)
	limitFlowWidth(top, 0)

	var buf bytes.Buffer
	for i := 0; i+1 < len(top.Content); i += 2 {
		if i > 0 {
			buf.WriteString("\n")
		}
		// Each top-level pair is encoded as a document of its own, which
		// yields the same text as encoding the whole mapping, plus a place
		// to put the blank line. The file's leading and trailing comments
		// travel with the first and last pair.
		pair := &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{
			Kind:    yaml.MappingNode,
			Content: []*yaml.Node{top.Content[i], top.Content[i+1]},
		}}}
		if i == 0 {
			pair.HeadComment = f.doc.HeadComment
		}
		if i+2 == len(top.Content) {
			pair.FootComment = f.doc.FootComment
		}
		if err := encode(&buf, pair); err != nil {
			return err
		}
	}

	if _, err := w.Write(buf.Bytes()); err != nil {
		return fmt.Errorf("write spec: %w", err)
	}
	return nil
}

// quoteCatalogVersion double-quotes catalog_version in the freshly encoded
// mapping m. yaml.v3 quotes only the values YAML would read as something
// else, such as 2026.10 as a number, and leaves 2026.10.1 plain; a pin is
// always written quoted, so every new spec looks the same whatever its
// version. A pin already in a loaded file keeps its style (see mergeScalar).
func quoteCatalogVersion(m *yaml.Node) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == "catalog_version" {
			m.Content[i+1].Style = yaml.DoubleQuotedStyle
		}
	}
}

func encode(w io.Writer, n *yaml.Node) error {
	enc := yaml.NewEncoder(w)
	enc.SetIndent(2)
	if err := enc.Encode(n); err != nil {
		return fmt.Errorf("encode spec: %w", err)
	}
	if err := enc.Close(); err != nil {
		return fmt.Errorf("encode spec: %w", err)
	}
	return nil
}

// merge updates dst in place so it holds the values of src, keeping the
// comments, order and styles of dst wherever a value survives. src comes
// from encoding the spec, so it has the new values and no comments.
func merge(dst, src *yaml.Node) {
	if dst.Kind != src.Kind {
		replace(dst, src)
		return
	}
	switch dst.Kind {
	case yaml.MappingNode:
		mergeMapping(dst, src)
	case yaml.SequenceNode:
		mergeSequence(dst, src)
	case yaml.ScalarNode:
		mergeScalar(dst, src)
	}
}

// replace overwrites dst with src but keeps the comments of dst.
func replace(dst, src *yaml.Node) {
	head, line, foot := dst.HeadComment, dst.LineComment, dst.FootComment
	*dst = *src
	dst.HeadComment, dst.LineComment, dst.FootComment = head, line, foot
}

// mergeMapping keeps the keys of dst that src still has, in their original
// order, drops the others, and inserts the keys only src has right after
// the key that precedes them in src.
func mergeMapping(dst, src *yaml.Node) {
	var out []*yaml.Node
	for i := 0; i+1 < len(dst.Content); i += 2 {
		k, v := dst.Content[i], dst.Content[i+1]
		if _, sv := mappingPair(src, k.Value); sv != nil {
			merge(v, sv)
			out = append(out, k, v)
		}
	}

	pos := 0 // where the next new key goes in out
	for i := 0; i+1 < len(src.Content); i += 2 {
		k, v := src.Content[i], src.Content[i+1]
		if j := keyIndex(out, k.Value); j >= 0 {
			pos = j + 2
			continue
		}
		out = append(out[:pos], append([]*yaml.Node{k, v}, out[pos:]...)...)
		pos += 2
	}
	dst.Content = out
}

// keyIndex returns the index of key in the flattened pairs of a mapping, or -1.
func keyIndex(pairs []*yaml.Node, key string) int {
	for i := 0; i+1 < len(pairs); i += 2 {
		if pairs[i].Value == key {
			return i
		}
	}
	return -1
}

// mergeSequence takes the items and order of src. A scalar item reuses the
// node of an equal item in dst, so its comments survive a reorder; other
// items, such as custom rules, are merged by position.
func mergeSequence(dst, src *yaml.Node) {
	used := make([]bool, len(dst.Content))
	out := make([]*yaml.Node, 0, len(src.Content))
	for i, s := range src.Content {
		if s.Kind == yaml.ScalarNode {
			out = append(out, takeScalar(dst.Content, used, s))
			continue
		}
		if i < len(dst.Content) && !used[i] && dst.Content[i].Kind != yaml.ScalarNode {
			used[i] = true
			merge(dst.Content[i], s)
			out = append(out, dst.Content[i])
			continue
		}
		out = append(out, s)
	}
	dst.Content = out
}

// takeScalar returns the first unused scalar in items equal to s, marking
// it used, or s itself when there is none.
func takeScalar(items []*yaml.Node, used []bool, s *yaml.Node) *yaml.Node {
	for i, n := range items {
		if !used[i] && n.Kind == yaml.ScalarNode && n.Value == s.Value {
			used[i] = true
			return n
		}
	}
	return s
}

// mergeScalar leaves dst untouched when its text is unchanged, keeping
// quoting such as an unquoted 2026.10. A changed string keeps the style of
// dst; yaml.v3 still adds quotes where the new value needs them.
func mergeScalar(dst, src *yaml.Node) {
	if dst.Value == src.Value {
		return
	}
	keepStyle := dst.ShortTag() == "!!str" && src.ShortTag() == "!!str"
	dst.Value, dst.Tag = src.Value, src.Tag
	if !keepStyle {
		dst.Style = src.Style
	}
}

// limitFlowWidth turns into block lists the flow sequences under the
// mapping m that would not fit in maxFlowWidth, or whose items carry
// comments, which a flow sequence cannot show. col is the column of the
// keys of m.
func limitFlowWidth(m *yaml.Node, col int) {
	if m.Kind != yaml.MappingNode || m.Style&yaml.FlowStyle != 0 {
		return
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		k, v := m.Content[i], m.Content[i+1]
		switch v.Kind {
		case yaml.MappingNode:
			limitFlowWidth(v, col+2)
		case yaml.SequenceNode:
			if v.Style&yaml.FlowStyle != 0 &&
				(col+utf8.RuneCountInString(k.Value+": ")+flowWidth(v) > maxFlowWidth || itemsHaveComments(v)) {
				v.Style &^= yaml.FlowStyle
			}
			if v.Style&yaml.FlowStyle == 0 {
				for _, item := range v.Content {
					// Block items sit at col+2, after "- " their keys at col+4.
					limitFlowWidth(item, col+4)
				}
			}
		}
	}
}

// flowWidth returns the length of seq written as a flow sequence, without comments.
func flowWidth(seq *yaml.Node) int {
	bare := *seq
	bare.HeadComment, bare.LineComment, bare.FootComment = "", "", ""
	out, err := yaml.Marshal(&bare)
	if err != nil {
		return maxFlowWidth + 1 // cannot measure: treat as too wide
	}
	return utf8.RuneCountInString(strings.TrimRight(string(out), "\n"))
}

func itemsHaveComments(seq *yaml.Node) bool {
	for _, item := range seq.Content {
		if item.HeadComment != "" || item.LineComment != "" || item.FootComment != "" {
			return true
		}
	}
	return false
}
