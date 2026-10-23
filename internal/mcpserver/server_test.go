package mcpserver

import (
	"context"
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	bundled "github.com/cyanotype-dev/cyanotype/catalog"
	"github.com/cyanotype-dev/cyanotype/internal/catalog"
	"github.com/cyanotype-dev/cyanotype/internal/contract"
	"github.com/cyanotype-dev/cyanotype/internal/preset"
)

const minimalSpec = `version: 1
targets: [agents-md]
project:
  name: tiny
  goal: A tiny project for tests.
selections: [conventional-commits]
`

// newProject writes files, given by slash-separated path, into a new
// temporary folder, and returns it.
func newProject(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// connect starts the server with the allowed folders dirs, the first of
// them the folder it starts in, and returns a client session to it over
// an in-memory transport.
func connect(t *testing.T, dirs ...string) *mcp.ClientSession {
	t.Helper()
	c, err := catalog.LoadRoot(bundled.FS())
	if err != nil {
		t.Fatal(err)
	}
	presets, err := preset.LoadRoot(bundled.FS(), c)
	if err != nil {
		t.Fatal(err)
	}
	folders, err := OpenFolders(dirs[0], dirs)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = folders.Close() })
	srv, err := New(c, presets, Options{Folders: folders, Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	serverEnd, clientEnd := mcp.NewInMemoryTransports()
	ss, err := srv.Connect(ctx, serverEnd, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ss.Close() })
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "test"}, nil).Connect(ctx, clientEnd, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

// call calls a tool and returns the result's text, decoded, after checking
// that the text and the structured content hold the same JSON.
func call(t *testing.T, cs *mcp.ClientSession, name string, args any) (*mcp.CallToolResult, map[string]any) {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if len(res.Content) != 1 {
		t.Fatalf("%s: %d content blocks, want 1", name, len(res.Content))
	}
	text, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("%s: content is %T, want text", name, res.Content[0])
	}
	var fromText, structured map[string]any
	if err := json.Unmarshal([]byte(text.Text), &fromText); err != nil {
		t.Fatalf("%s: text is not JSON: %v\n%s", name, err, text.Text)
	}
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &structured); err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(fromText)
	b, _ := json.Marshal(structured)
	if string(a) != string(b) {
		t.Errorf("%s: structured content differs from the text:\n%s\n%s", name, a, b)
	}
	return res, fromText
}

// errorCodes returns the codes of an output's errors.
func errorCodes(out map[string]any) []string {
	var codes []string
	errs, _ := out["errors"].([]any)
	for _, e := range errs {
		codes = append(codes, e.(map[string]any)["code"].(string))
	}
	return codes
}

func TestListTools(t *testing.T) {
	cs := connect(t, newProject(t, nil))
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
		schema, ok := tool.OutputSchema.(map[string]any)
		if !ok {
			t.Errorf("%s: no output schema", tool.Name)
			continue
		}
		defs, _ := schema["$defs"].(map[string]any)
		ref, _ := schema["$ref"].(string)
		if _, ok := defs[strings.TrimPrefix(ref, "#/$defs/")]; !ok || schema["type"] != "object" {
			t.Errorf("%s: output schema $ref %q does not name an object definition", tool.Name, ref)
		}
		in, _ := tool.InputSchema.(map[string]any)
		if in["additionalProperties"] != false {
			t.Errorf("%s: input schema allows unknown arguments", tool.Name)
		}
		readOnly := tool.Annotations != nil && tool.Annotations.ReadOnlyHint
		props, _ := in["properties"].(map[string]any)
		if _, hasWrite := props["write"]; hasWrite == readOnly {
			t.Errorf("%s: read-only %v, but write argument %v", tool.Name, readOnly, hasWrite)
		}
	}
	want := []string{"check", "detect_stack", "generate", "get_spec", "preview", "search_catalog", "set_selections", "validate"}
	slices.Sort(names)
	if !slices.Equal(names, want) {
		t.Errorf("tools = %v, want %v", names, want)
	}
}

// TestOutputSchemaBundlesSpec checks that the output schema of get_spec
// carries spec.schema.json instead of referring to the file.
func TestOutputSchemaBundlesSpec(t *testing.T) {
	s, err := outputSchema("spec")
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(s)
	if strings.Contains(string(data), `"spec.schema.json"`) {
		t.Error("the schema still refers to spec.schema.json")
	}
	if _, ok := s["$defs"].(map[string]any)[specFileDef]; !ok {
		t.Errorf("$defs has no %s", specFileDef)
	}
	if _, err := outputSchema("nothing"); err == nil {
		t.Error(`outputSchema("nothing") succeeded`)
	}
}

func TestTools(t *testing.T) {
	handWritten := map[string]string{"cyanotype.yaml": minimalSpec, "AGENTS.md": "# Our rules\n"}
	tests := []struct {
		name    string
		files   map[string]string
		tool    string
		args    map[string]any
		isError bool
		codes   []string // error codes, in order
		check   func(t *testing.T, dir string, out map[string]any)
		after   map[string]string // files that must hold this content afterwards
		absent  []string          // files that must not exist afterwards
	}{
		{
			name: "search", tool: "search_catalog", args: map[string]any{"query": "package manager"},
			check: func(t *testing.T, _ string, out map[string]any) {
				if len(out["entries"].([]any)) == 0 {
					t.Error("no entries")
				}
			},
		},
		{name: "search unknown argument", tool: "search_catalog", args: map[string]any{"terms": "x"}, isError: true, codes: []string{"usage"}},
		{
			name: "spec", files: map[string]string{"cyanotype.yaml": minimalSpec}, tool: "get_spec",
			check: func(t *testing.T, _ string, out map[string]any) {
				if out["scope"] != "team" {
					t.Errorf("scope = %v, want team", out["scope"])
				}
			},
		},
		{name: "spec missing", tool: "get_spec", isError: true, codes: []string{"no-spec"}},
		{name: "spec outside", tool: "get_spec", args: map[string]any{"dir": ".."}, isError: true, codes: []string{"outside-allowed-folders"}},
		{name: "spec network path", tool: "get_spec", args: map[string]any{"dir": `\\server\share`}, isError: true, codes: []string{"usage"}},
		{name: "spec unknown argument", tool: "get_spec", args: map[string]any{"path": "."}, isError: true, codes: []string{"usage"}},
		{
			name: "detect", files: map[string]string{"go.mod": "module example.com/x\n\ngo 1.27\n"}, tool: "detect_stack",
			check: func(t *testing.T, _ string, out map[string]any) {
				if len(out["findings"].([]any)) == 0 {
					t.Error("no findings for go.mod")
				}
			},
		},
		{name: "validate", files: map[string]string{"cyanotype.yaml": minimalSpec}, tool: "validate"},
		{
			name: "validate errors", tool: "validate", isError: true, codes: []string{"unknown-entry"},
			files: map[string]string{"cyanotype.yaml": strings.Replace(minimalSpec, "[conventional-commits]", "[conventional-commits, pnpn]", 1)},
		},
		{
			name: "preview", files: map[string]string{"cyanotype.yaml": minimalSpec}, tool: "preview",
			absent: []string{"AGENTS.md", "cyanotype.lock"},
		},
		{
			name: "preview hand-written", files: handWritten, tool: "preview",
			after: map[string]string{"AGENTS.md": "# Our rules\n"},
			check: func(t *testing.T, _ string, out map[string]any) {
				if w := out["waiting_for_choice"].([]any); len(w) != 1 || w[0] != "AGENTS.md" {
					t.Errorf("waiting_for_choice = %v", w)
				}
			},
		},
		{name: "preview bad choice", files: handWritten, tool: "preview", args: map[string]any{"existing": "overwrite"}, isError: true, codes: []string{"usage"}},
		{
			name: "generate without write", files: map[string]string{"cyanotype.yaml": minimalSpec}, tool: "generate",
			absent: []string{"AGENTS.md", "cyanotype.lock"},
		},
		{
			name: "generate write", files: map[string]string{"cyanotype.yaml": minimalSpec}, tool: "generate", args: map[string]any{"write": true},
			check: func(t *testing.T, dir string, out map[string]any) {
				if len(out["written"].([]any)) == 0 {
					t.Error("written is empty")
				}
				if _, err := os.Stat(filepath.Join(dir, "AGENTS.md")); err != nil {
					t.Error(err)
				}
			},
		},
		{
			name: "generate hand-written without a choice", files: handWritten, tool: "generate", args: map[string]any{"write": true},
			isError: true, codes: []string{"choice-required"}, after: map[string]string{"AGENTS.md": "# Our rules\n"},
		},
		{
			name: "generate replace outside git", files: handWritten, tool: "generate", args: map[string]any{"write": true, "existing": "replace"},
			isError: true, codes: []string{"replace-refused"}, after: map[string]string{"AGENTS.md": "# Our rules\n"},
		},
		{
			name: "generate import", files: handWritten, tool: "generate", args: map[string]any{"write": true, "existing": "import"},
			after: map[string]string{"AGENTS.md": "# Our rules\n"}, absent: []string{"cyanotype.lock"},
			check: func(t *testing.T, _ string, out map[string]any) {
				imp, _ := out["import"].(map[string]any)
				if prompt, _ := imp["prompt"].(string); !strings.Contains(prompt, "cyanotype.yaml") {
					t.Errorf("import = %v, want the prompt", out["import"])
				}
			},
		},
		{
			name: "generate keep", files: handWritten, tool: "generate", args: map[string]any{"write": true, "existing": "keep"},
			check: func(t *testing.T, dir string, _ map[string]any) {
				data, err := os.ReadFile(filepath.Join(dir, "AGENTS.md")) //nolint:gosec // test temp dir
				if err != nil || !strings.Contains(string(data), "# Our rules") || !strings.Contains(string(data), "cyanotype:custom") {
					t.Errorf("AGENTS.md does not keep the rules in a custom block:\n%s", data)
				}
			},
		},
		{name: "check", files: map[string]string{"cyanotype.yaml": minimalSpec}, tool: "check", args: map[string]any{"strict": true},
			check: func(t *testing.T, _ string, out map[string]any) {
				if s := out["summary"].(map[string]any); s["drift"] != true || s["strict"] != true {
					t.Errorf("summary = %v, want drift: files never generated", s)
				}
			},
		},
		{
			name: "select without write", files: map[string]string{"cyanotype.yaml": minimalSpec}, tool: "set_selections",
			args: map[string]any{"add": []string{"small-commits"}}, after: map[string]string{"cyanotype.yaml": minimalSpec},
			check: func(t *testing.T, _ string, out map[string]any) {
				if out["written"] != false || !strings.Contains(out["spec_diff"].(string), "small-commits") {
					t.Errorf("written = %v, spec_diff = %v", out["written"], out["spec_diff"])
				}
			},
		},
		{
			name: "select write", files: map[string]string{"cyanotype.yaml": minimalSpec}, tool: "set_selections",
			args: map[string]any{"add": []string{"small-commits"}, "write": true},
			check: func(t *testing.T, dir string, out map[string]any) {
				data, err := os.ReadFile(filepath.Join(dir, "cyanotype.yaml")) //nolint:gosec // test temp dir
				if err != nil || !strings.Contains(string(data), "selections: [conventional-commits, small-commits]") || out["written"] != true {
					t.Errorf("written = %v, cyanotype.yaml:\n%s", out["written"], data)
				}
			},
		},
		{
			name: "select locked", files: map[string]string{"cyanotype.yaml": minimalSpec + "locked: [conventional-commits]\n"}, tool: "set_selections",
			args: map[string]any{"remove": []string{"conventional-commits"}, "write": true}, isError: true, codes: []string{"locked"},
			after: map[string]string{"cyanotype.yaml": minimalSpec + "locked: [conventional-commits]\n"},
		},
		{
			name: "select refused", files: map[string]string{"cyanotype.yaml": minimalSpec}, tool: "set_selections",
			args: map[string]any{"add": []string{"pnpn"}, "write": true}, isError: true, codes: []string{"unknown-entry"},
			after: map[string]string{"cyanotype.yaml": minimalSpec},
		},
		{name: "select nothing", files: map[string]string{"cyanotype.yaml": minimalSpec}, tool: "set_selections", isError: true, codes: []string{"usage"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := newProject(t, tt.files)
			cs := connect(t, dir)
			res, out := call(t, cs, tt.tool, tt.args)
			if res.IsError != tt.isError {
				t.Errorf("isError = %v, want %v\n%v", res.IsError, tt.isError, out)
			}
			if got := errorCodes(out); !slices.Equal(got, tt.codes) {
				t.Errorf("error codes = %v, want %v\n%v", got, tt.codes, out)
			}
			if out["version"] != float64(contract.Version) {
				t.Errorf("version = %v", out["version"])
			}
			if tt.check != nil {
				tt.check(t, dir, out)
			}
			for _, name := range slices.Sorted(maps.Keys(tt.after)) {
				data, err := os.ReadFile(filepath.Join(dir, name)) //nolint:gosec // test temp dir
				if err != nil || string(data) != tt.after[name] {
					t.Errorf("%s afterwards = %q, %v; want %q", name, data, err, tt.after[name])
				}
			}
			for _, name := range tt.absent {
				if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
					t.Errorf("%s was written", name)
				}
			}
		})
	}
}

// TestSeveralFolders checks dir with more than one allowed folder.
func TestSeveralFolders(t *testing.T) {
	a := newProject(t, map[string]string{"cyanotype.yaml": minimalSpec})
	b := newProject(t, map[string]string{"cyanotype.yaml": strings.Replace(minimalSpec, "tiny", "other", 1)})
	cs := connect(t, a, b)
	if _, out := call(t, cs, "get_spec", nil); !slices.Equal(errorCodes(out), []string{"usage"}) {
		t.Errorf("without dir: %v, want a usage error", out)
	}
	_, out := call(t, cs, "get_spec", map[string]any{"dir": b})
	if name := out["spec"].(map[string]any)["project"].(map[string]any)["name"]; name != "other" {
		t.Errorf("project name = %v, want the one in the second folder", name)
	}
}

func TestImportPrompt(t *testing.T) {
	tests := []struct {
		name    string
		files   map[string]string
		args    map[string]string
		want    []string // parts of the prompt
		wantErr string
	}{
		{
			name:  "with a spec",
			files: map[string]string{"cyanotype.yaml": minimalSpec, "AGENTS.md": "# Our rules\n"},
			want:  []string{"# Convert AGENTS.md into cyanotype.yaml", "update cyanotype.yaml, which already exists"},
		},
		{
			name:  "without a spec",
			files: map[string]string{"CLAUDE.md": "# Rules\n", "go.mod": "module example.com/x\n\ngo 1.27\n"},
			want:  []string{"# Convert CLAUDE.md into cyanotype.yaml", "write cyanotype.yaml at the project root", "- go"},
		},
		{name: "nothing to import", files: map[string]string{"cyanotype.yaml": minimalSpec}, wantErr: "no instruction files written by hand"},
		{name: "outside", files: map[string]string{"CLAUDE.md": "# Rules\n"}, args: map[string]string{"dir": ".."}, wantErr: "outside the folders"},
		{name: "unknown argument", files: map[string]string{"CLAUDE.md": "# Rules\n"}, args: map[string]string{"path": "."}, wantErr: "unknown argument"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs := connect(t, newProject(t, tt.files))
			res, err := cs.GetPrompt(context.Background(), &mcp.GetPromptParams{Name: "import_instructions", Arguments: tt.args})
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("GetPrompt() error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			text := res.Messages[0].Content.(*mcp.TextContent).Text
			for _, w := range tt.want {
				if !strings.Contains(text, w) {
					t.Errorf("prompt lacks %q:\n%s", w, text)
				}
			}
		})
	}
}
