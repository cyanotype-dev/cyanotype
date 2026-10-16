// Package catalog loads and validates catalog entries.
//
// The catalog is a folder of YAML files, one per selectable technology or
// rule. Each entry carries the text it contributes to generated instruction
// files, the reason behind it, and any real constraints to enforce. The rest
// of the code holds no knowledge about specific stacks; it all comes from here.
package catalog
