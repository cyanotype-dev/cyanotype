package mcpserver

import (
	"encoding/json"
	"fmt"

	"github.com/cyanotype-dev/cyanotype/schema"
)

// specFileDef is the name, under $defs, of spec.schema.json once it is
// bundled into an output schema. "spec" is taken: it is the output of
// get_spec.
const specFileDef = "spec_file"

// outputSchema returns the schema of a tool's result: the definition def
// of output.schema.json, made self-contained. A client gets one schema
// per tool, with no file to fetch, so every definition goes along under
// $defs, and spec.schema.json, which the output of get_spec refers to,
// joins them as specFileDef.
func outputSchema(def string) (map[string]any, error) {
	var out, spec map[string]any
	if err := json.Unmarshal(schema.Output, &out); err != nil {
		return nil, fmt.Errorf("read output.schema.json: %w", err)
	}
	if err := json.Unmarshal(schema.Spec, &spec); err != nil {
		return nil, fmt.Errorf("read spec.schema.json: %w", err)
	}
	defs, ok := out["$defs"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("output.schema.json has no $defs")
	}
	if _, ok := defs[def].(map[string]any); !ok {
		return nil, fmt.Errorf("output.schema.json has no definition %q", def)
	}
	delete(spec, "$schema")
	delete(spec, "$id")
	defs[specFileDef] = spec
	rewriteRefs(defs)
	return map[string]any{
		"$schema": out["$schema"],
		"type":    "object",
		"$ref":    "#/$defs/" + def,
		"$defs":   defs,
	}, nil
}

// rewriteRefs points every reference to spec.schema.json in v at its
// bundled copy.
func rewriteRefs(v any) {
	switch v := v.(type) {
	case map[string]any:
		for k, child := range v {
			if k == "$ref" && child == "spec.schema.json" {
				v[k] = "#/$defs/" + specFileDef
				continue
			}
			rewriteRefs(child)
		}
	case []any:
		for _, child := range v {
			rewriteRefs(child)
		}
	}
}
