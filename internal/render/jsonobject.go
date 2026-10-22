package render

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
)

// jsonObject is a JSON object that keeps its keys in order and its values as
// raw bytes. encoding/json decodes objects into maps, which lose the key
// order and, through float64, the exact form of numbers; merging into a file
// someone wrote by hand must keep both.
type jsonObject []jsonMember

// jsonMember is one key and its raw value.
type jsonMember struct {
	key   string
	value json.RawMessage
}

// parseJSONObject decodes data, which must hold exactly one JSON object.
// Duplicate keys are an error: which value counts is up to each reader, so
// merging into such a file could change its meaning.
func parseJSONObject(data []byte) (jsonObject, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	if tok != json.Delim('{') {
		return nil, errors.New("must be a JSON object")
	}
	var obj jsonObject
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("invalid JSON: %w", err)
		}
		key, _ := tok.(string) // inside an object, a token in key position is always a string
		if _, ok := obj.get(key); ok {
			return nil, fmt.Errorf("duplicate key %q", key)
		}
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return nil, fmt.Errorf("invalid JSON: %w", err)
		}
		obj = append(obj, jsonMember{key: key, value: value})
	}
	if _, err := dec.Token(); err != nil { // the closing brace
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("invalid JSON: unexpected data after the object")
	}
	return obj, nil
}

// get returns the raw value of key.
func (o jsonObject) get(key string) (json.RawMessage, bool) {
	for _, m := range o {
		if m.key == key {
			return m.value, true
		}
	}
	return nil, false
}

// set replaces the value of key where it is, or adds key at the end.
func (o jsonObject) set(key string, value json.RawMessage) jsonObject {
	for i, m := range o {
		if m.key == key {
			o[i].value = value
			return o
		}
	}
	return append(o, jsonMember{key: key, value: value})
}

// without returns o without key, keeping the other members in order.
func (o jsonObject) without(key string) jsonObject {
	return slices.DeleteFunc(slices.Clone(o), func(m jsonMember) bool { return m.key == key })
}

// MarshalJSON writes the members in order. It implements json.Marshaler, so
// an encoder can indent the result.
func (o jsonObject) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, m := range o {
		if i > 0 {
			b.WriteByte(',')
		}
		key, err := marshalJSON(m.key)
		if err != nil {
			return nil, err
		}
		b.Write(key)
		b.WriteByte(':')
		b.Write(m.value)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

// marshalJSON encodes v compactly without HTML escaping, which would turn
// the ">" of a rule such as "Bash(echo > out)" into ">".
func marshalJSON(v any) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(b.Bytes(), []byte("\n")), nil
}

// indentJSON encodes v with two-space indentation, without HTML escaping
// and with a trailing newline, as generated files are written.
func indentJSON(v any) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
