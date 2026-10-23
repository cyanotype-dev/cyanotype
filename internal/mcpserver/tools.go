package mcpserver

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/cyanotype-dev/cyanotype/internal/adopt"
	"github.com/cyanotype-dev/cyanotype/internal/catalog"
	"github.com/cyanotype-dev/cyanotype/internal/contract"
	"github.com/cyanotype-dev/cyanotype/internal/ops"
)

// Arguments of the tools. Every field is optional; dir defaults to the
// only allowed folder.
type (
	// dirField is the dir argument, embedded in the arguments of every
	// tool that works on a project.
	dirField struct {
		Dir string `json:"dir"`
	}
	searchArgs struct {
		Query    string   `json:"query"`
		Category string   `json:"category"`
		Stack    []string `json:"stack"`
	}
	previewArgs struct {
		dirField
		Existing string `json:"existing"`
	}
	generateArgs struct {
		dirField
		Existing string `json:"existing"`
		Write    bool   `json:"write"`
	}
	checkArgs struct {
		dirField
		Strict bool `json:"strict"`
	}
	selectArgs struct {
		dirField
		Add    []string `json:"add"`
		Remove []string `json:"remove"`
		Write  bool     `json:"write"`
	}
)

func (d dirField) dir() string { return d.Dir }

// The input schemas share these properties.
var (
	dirProperty = map[string]any{
		"type": "string",
		"description": "The project folder: one of the folders the server may use, or a folder inside one. " +
			"A relative path starts at the folder the server started in. Leave it out when the server may use only one folder.",
	}
	existingProperty = map[string]any{
		"type": "string",
		"enum": []string{string(adopt.Import), string(adopt.Keep), string(adopt.Move), string(adopt.Replace)},
		"description": "What to do with instruction files written by hand, which are never replaced without this choice: " +
			"import (recommended: returns a prompt that has you turn their rules into cyanotype.yaml; nothing is written), " +
			"keep (the whole file goes in a cyanotype:custom block), move (its content goes to a separate file the main one points to), " +
			"or replace (only when git has the file committed, so it can be recovered). Ask the user which; never choose yourself.",
	}
	writeProperty = map[string]any{
		"type":        "boolean",
		"description": "Write the changes. Without it nothing is written. Pass true only when the user asked for this change.",
	}
	idsProperty = func(description string) map[string]any {
		return map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": description}
	}
)

// inputSchema returns an object schema with properties, which allows no
// other property.
func inputSchema(properties map[string]any) map[string]any {
	return map[string]any{"type": "object", "properties": properties, "additionalProperties": false}
}

// readOnly returns the annotations of a tool that changes nothing.
func readOnly(title string) *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{Title: title, ReadOnlyHint: true, OpenWorldHint: new(false)}
}

// writing returns the annotations of a tool that may write. Destructive:
// generate deletes stale generated files and set_selections removes
// selections. Idempotent: the same call twice changes nothing more.
func writing(title string) *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{Title: title, DestructiveHint: new(true), IdempotentHint: true, OpenWorldHint: new(false)}
}

// Failure outputs: each tool's output type holding only diagnostics, for
// an error that stops the tool before it starts.
func specFailure(d contract.Diagnostics) contract.Output   { return &contract.Spec{Diagnostics: d} }
func detectFailure(d contract.Diagnostics) contract.Output { return &contract.Detect{Diagnostics: d} }
func validateFailure(d contract.Diagnostics) contract.Output {
	return &contract.Validate{Diagnostics: d}
}
func generateFailure(d contract.Diagnostics) contract.Output {
	return &contract.Generate{Diagnostics: d}
}
func checkFailure(d contract.Diagnostics) contract.Output  { return &contract.Check{Diagnostics: d} }
func selectFailure(d contract.Diagnostics) contract.Output { return &contract.Select{Diagnostics: d} }

// tools returns the server's tools, in the order of the MCP server
// section of docs/DESIGN.md.
func (s *server) tools() []tool {
	return []tool{
		{
			tool: &mcp.Tool{
				Name:  "search_catalog",
				Title: "Search the catalog",
				Description: "Finds entries in the bundled catalog: the rules, stacks and practices cyanotype can put in the instruction files. " +
					"query holds words that must all appear in an entry's id, name, summary or category, ignoring case; " +
					"stack keeps the entries whose requirements those entry ids meet. Same result as cyanotype catalog search --json.",
				InputSchema: inputSchema(map[string]any{
					"query":    map[string]any{"type": "string", "description": "Words to look for, separated by blanks. Leave it out to list every entry."},
					"category": map[string]any{"type": "string", "description": "Keep only this category, such as package-manager."},
					"stack":    idsProperty("Entry ids the project uses, such as [\"node\", \"pnpm\"]."),
				}),
				Annotations: readOnly("Search the catalog"),
			},
			output: "catalog",
			run: func(raw json.RawMessage) contract.Output {
				var a searchArgs
				if err := decode(raw, &a); err != nil {
					return &contract.Catalog{Diagnostics: usage(err)}
				}
				return ops.Search(s.cat, catalog.Query{Terms: strings.Fields(a.Query), Category: a.Category, Stack: a.Stack})
			},
		},
		{
			tool: &mcp.Tool{
				Name:  "get_spec",
				Title: "Read cyanotype.yaml",
				Description: "Returns the project's cyanotype.yaml, as written and with its defaults filled in. " +
					"It is the team spec only: cyanotype.local.yaml is not supported yet. Same result as cyanotype spec --json.",
				InputSchema: inputSchema(map[string]any{"dir": dirProperty}),
				Annotations: readOnly("Read cyanotype.yaml"),
			},
			output: "spec",
			run: inProject(s, specFailure, func(root *os.Root, _ dirField) contract.Output {
				return ops.Spec(root)
			}),
		},
		{
			tool: &mcp.Tool{
				Name:  "detect_stack",
				Title: "Detect the stack",
				Description: "Lists the catalog entries the project's files suggest, such as node and pnpm from package.json and pnpm-lock.yaml, with the files behind each. " +
					"It needs no cyanotype.yaml. Same result as cyanotype detect --json.",
				InputSchema: inputSchema(map[string]any{"dir": dirProperty}),
				Annotations: readOnly("Detect the stack"),
			},
			output: "detect",
			run: inProject(s, detectFailure, func(root *os.Root, _ dirField) contract.Output {
				return ops.Detect(s.cat, root)
			}),
		},
		{
			tool: &mcp.Tool{
				Name:  "validate",
				Title: "Validate cyanotype.yaml",
				Description: "Checks that cyanotype.yaml is valid and resolves against the catalog, and how long each generated Markdown file would be, " +
					"without comparing or writing files. Same result as cyanotype validate --json.",
				InputSchema: inputSchema(map[string]any{"dir": dirProperty}),
				Annotations: readOnly("Validate cyanotype.yaml"),
			},
			output: "validate",
			run: inProject(s, validateFailure, func(root *os.Root, _ dirField) contract.Output {
				return ops.Validate(s.cat, root)
			}),
		},
		{
			tool: &mcp.Tool{
				Name:  "preview",
				Title: "Preview generated files",
				Description: "Shows what generate would change: every generated file with its status and a unified diff. It never writes. " +
					"Hand-written instruction files are listed in waiting_for_choice until existing gives a choice. " +
					"Same result as cyanotype generate --dry-run --json.",
				InputSchema: inputSchema(map[string]any{"dir": dirProperty, "existing": existingProperty}),
				Annotations: readOnly("Preview generated files"),
			},
			output: "generate",
			run: inProject(s, generateFailure, func(root *os.Root, a previewArgs) contract.Output {
				existing, err := parseExisting(a.Existing)
				if err != nil {
					return generateFailure(usage(err))
				}
				out, _ := ops.Generate(s.cat, root, ops.GenerateOptions{Existing: existing, DryRun: true})
				return out
			}),
		},
		{
			tool: &mcp.Tool{
				Name:  "generate",
				Title: "Generate files",
				Description: "Writes the instruction files and constraints that cyanotype.yaml generates, only with write: true. " +
					"Without it, it shows the changes like preview, and reports hand-written files waiting for a choice as an error. " +
					"The only files it deletes are files it generated earlier, unchanged since. Run preview first, and pass write: true only when the user asked to write. " +
					"Same result as cyanotype generate --json, with --yes for write: true.",
				InputSchema: inputSchema(map[string]any{"dir": dirProperty, "existing": existingProperty, "write": writeProperty}),
				Annotations: writing("Generate files"),
			},
			output: "generate",
			writes: true,
			run: inProject(s, generateFailure, func(root *os.Root, a generateArgs) contract.Output {
				existing, err := parseExisting(a.Existing)
				if err != nil {
					return generateFailure(usage(err))
				}
				out, _ := ops.Generate(s.cat, root, ops.GenerateOptions{Existing: existing, Write: a.Write})
				return out
			}),
		},
		{
			tool: &mcp.Tool{
				Name:  "check",
				Title: "Check for drift",
				Description: "Reports drift between the project and cyanotype.yaml: generated files edited by hand or out of date, tracked files the generated .gitignore keeps out, " +
					"the detected stack, an older catalog pin, long files and pointers to missing files. It never writes. Same result as cyanotype check --json.",
				InputSchema: inputSchema(map[string]any{
					"dir":    dirProperty,
					"strict": map[string]any{"type": "boolean", "description": "Count warnings as drift too."},
				}),
				Annotations: readOnly("Check for drift"),
			},
			output: "check",
			run: inProject(s, checkFailure, func(root *os.Root, a checkArgs) contract.Output {
				return ops.Check(s.cat, root, a.Strict)
			}),
		},
		{
			tool: &mcp.Tool{
				Name:  "set_selections",
				Title: "Change the selected entries",
				Description: "Adds catalog entries to the selections of cyanotype.yaml and removes others, keeping the file's comments, and returns the change as a diff. " +
					"It writes only with write: true, and refuses a change that would leave the spec with errors or remove a locked entry. " +
					"Change the selections only when the user explicitly asks. It changes only cyanotype.yaml: run generate afterwards. " +
					"Same result as cyanotype select --json, with --yes for write: true.",
				InputSchema: inputSchema(map[string]any{
					"dir":    dirProperty,
					"add":    idsProperty("Entry ids to select, appended in this order."),
					"remove": idsProperty("Entry ids to stop selecting."),
					"write":  writeProperty,
				}),
				Annotations: writing("Change the selected entries"),
			},
			output: "select",
			writes: true,
			run: inProject(s, selectFailure, func(root *os.Root, a selectArgs) contract.Output {
				out, change := ops.Select(s.cat, root, a.Add, a.Remove)
				if change != nil && a.Write {
					out = ops.WriteSelect(root, change, out)
				}
				return out
			}),
		},
	}
}

// parseExisting reads the existing argument; "" means no choice.
func parseExisting(value string) (adopt.Choice, error) {
	if value == "" {
		return "", nil
	}
	c, err := adopt.ParseChoice(value)
	if err != nil {
		return "", fmt.Errorf("existing %w", err)
	}
	return c, nil
}
