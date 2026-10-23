package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// mcpSession runs the mcp command in this process with --allow dir, and
// returns a client session to it and the command's stderr, to read once
// the session is closed.
func mcpSession(t *testing.T, dir string) (*mcp.ClientSession, *lockedBuffer) {
	t.Helper()
	serverIn, clientOut := io.Pipe()
	clientIn, serverOut := io.Pipe()
	stderr := &lockedBuffer{}
	cmd := newRootCmdWithConsole("test", func() console { return nonInteractive })
	cmd.SetArgs([]string{"mcp", "--allow", dir})
	cmd.SetIn(serverIn)
	cmd.SetOut(serverOut)
	cmd.SetErr(stderr)
	done := make(chan error, 1)
	go func() {
		done <- cmd.Execute()
		_ = serverOut.Close()
	}()
	transport := &mcp.IOTransport{Reader: clientIn, Writer: clientOut}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "test"}, nil).Connect(context.Background(), transport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cs.Close()
		if err := <-done; err != nil {
			t.Errorf("mcp command: %v\n%s", err, stderr)
		}
	})
	return cs, stderr
}

// lockedBuffer is a bytes.Buffer safe for the server goroutine to write
// while the test reads it.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// toolText calls a tool and returns the text of its result.
func toolText(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) string {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	text, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("%s: content is %T, want text", name, res.Content[0])
	}
	return text.Text
}

// TestMCPParity checks that each MCP tool returns, byte for byte, what the
// matching command prints with --json, on two copies of the same project:
// one for the command, one for the tool, since some of them write.
func TestMCPParity(t *testing.T) {
	handWritten := func(t *testing.T) string {
		return newProject(t, map[string]string{"cyanotype.yaml": minimalSpec, "AGENTS.md": "# Our rules\n"})
	}
	spec := func(t *testing.T) string { return newProject(t, map[string]string{"cyanotype.yaml": minimalSpec}) }
	drift := func(t *testing.T) string {
		dir := jsonProject(t)
		writeFile(t, dir, "AGENTS.md", readFile(t, dir, "AGENTS.md")+"- A rule added by hand.\n")
		writeFile(t, dir, "go.mod", "module example.com/tiny\n\ngo 1.27\n")
		return dir
	}
	tests := []struct {
		name  string
		setup func(t *testing.T) string
		cli   []string
		tool  string
		args  map[string]any
	}{
		{"search", newEmptyProject, []string{"catalog", "search", "package", "manager"}, "search_catalog", map[string]any{"query": "package manager"}},
		{"search stack", newEmptyProject, []string{"catalog", "search", "--category", "ui-components", "--stack", "tailwind,nextjs"},
			"search_catalog", map[string]any{"category": "ui-components", "stack": []string{"tailwind", "nextjs"}}},
		{"spec", spec, []string{"spec"}, "get_spec", nil},
		{"spec missing", newEmptyProject, []string{"spec"}, "get_spec", nil},
		{"detect", func(t *testing.T) string {
			return newProject(t, map[string]string{"package.json": `{"dependencies": {"next": "16.0.0"}}`, "pnpm-lock.yaml": "lockfileVersion: '9.0'\n"})
		}, []string{"detect"}, "detect_stack", nil},
		{"validate", jsonProject, []string{"validate"}, "validate", nil},
		{"validate errors", func(t *testing.T) string {
			return newProject(t, map[string]string{"cyanotype.yaml": strings.Replace(minimalSpec, "[conventional-commits]", "[conventional-commits, pnpm]", 1)})
		}, []string{"validate"}, "validate", nil},
		{"preview", spec, []string{"generate", "--dry-run"}, "preview", nil},
		{"generate without write", spec, []string{"generate"}, "generate", nil},
		{"generate write", spec, []string{"generate", "--yes"}, "generate", map[string]any{"write": true}},
		{"generate up to date", jsonProject, []string{"generate"}, "generate", nil},
		{"preview hand-written", handWritten, []string{"generate", "--dry-run"}, "preview", nil},
		{"generate hand-written", handWritten, []string{"generate", "--yes"}, "generate", map[string]any{"write": true}},
		{"preview keep", handWritten, []string{"generate", "--dry-run", "--existing", "keep"}, "preview", map[string]any{"existing": "keep"}},
		{"generate move", handWritten, []string{"generate", "--yes", "--existing", "move"}, "generate", map[string]any{"write": true, "existing": "move"}},
		{"generate import", handWritten, []string{"generate", "--existing", "import"}, "generate", map[string]any{"existing": "import"}},
		{"check", drift, []string{"check"}, "check", nil},
		{"check strict", drift, []string{"check", "--strict"}, "check", map[string]any{"strict": true}},
		{"select dry run", spec, []string{"select", "--add", "small-commits", "--remove", "conventional-commits", "--dry-run"},
			"set_selections", map[string]any{"add": []string{"small-commits"}, "remove": []string{"conventional-commits"}}},
		{"select write", spec, []string{"select", "--add", "small-commits", "--yes"}, "set_selections", map[string]any{"add": []string{"small-commits"}, "write": true}},
		{"select refused", spec, []string{"select", "--add", "pnpm,pnpn", "--yes"}, "set_selections", map[string]any{"add": []string{"pnpm", "pnpn"}, "write": true}},
		{"select locked", func(t *testing.T) string {
			return newProject(t, map[string]string{"cyanotype.yaml": minimalSpec + "locked: [conventional-commits]\n"})
		}, []string{"select", "--remove", "conventional-commits", "--yes"}, "set_selections", map[string]any{"remove": []string{"conventional-commits"}, "write": true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cliDir, mcpDir := tt.setup(t), tt.setup(t)
			args := append(append([]string{}, tt.cli...), "--json")
			if tt.cli[0] != "catalog" {
				args = append(args, "--dir", cliDir)
			}
			r := runCLI(t, nonInteractive, failReader{t}, args...)
			if r.stderr != "" {
				t.Fatalf("stderr is not empty:\n%s", r.stderr)
			}
			cs, _ := mcpSession(t, mcpDir)
			got := normalizeDir(toolText(t, cs, tt.tool, tt.args), mcpDir)
			want := normalizeDir(r.stdout, cliDir)
			if got != want {
				t.Errorf("%s returns:\n%s\ncyanotype %s --json prints:\n%s", tt.tool, got, strings.Join(tt.cli, " "), want)
			}
			if a, b := tree(t, cliDir), tree(t, mcpDir); !sameTree(a, b) {
				t.Errorf("the projects differ afterwards: command %q, tool %q", keys(a), keys(b))
			}
		})
	}
}

// newEmptyProject is a project folder with no files.
func newEmptyProject(t *testing.T) string { return newProject(t, nil) }

// sameTree reports whether two trees, as tree returns them, hold the same
// files with the same content.
func sameTree(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if w, ok := b[k]; !ok || w != v {
			return false
		}
	}
	return true
}

// TestMCPAllowRefused checks that a folder --allow cannot use stops the
// command before it speaks the protocol: stdout stays empty.
func TestMCPAllowRefused(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	for _, args := range [][]string{
		{"mcp", "--allow", missing},
		{"mcp", "--allow", `\\server\share`},
		{"mcp", "extra"},
	} {
		r := runCLI(t, nonInteractive, failReader{t}, args...)
		if r.code != codeUsage {
			t.Errorf("%q: exit code %d, want %d", args, r.code, codeUsage)
		}
		if r.stdout != "" {
			t.Errorf("%q: stdout is not empty:\n%s", args, r.stdout)
		}
	}
}

// TestMCPEndToEnd builds the cyanotype binary, starts cyanotype mcp in a
// project as a client would, and checks that every line it writes to
// stdout is a JSON-RPC message, and that its logs go to stderr.
func TestMCPEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the binary")
	}
	bin := filepath.Join(t.TempDir(), "cyanotype")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil { //nolint:gosec // fixed arguments
		t.Fatalf("go build: %v\n%s", err, out)
	}
	dir := newProject(t, map[string]string{"cyanotype.yaml": minimalSpec, "AGENTS.md": "# Rules\n"})

	cmd := exec.Command(bin, "mcp") //nolint:gosec // the binary built above
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	var raw bytes.Buffer // every byte the server writes to stdout
	transport := &mcp.IOTransport{Reader: io.NopCloser(io.TeeReader(stdout, &raw)), Writer: stdin}
	ctx := context.Background()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "test"}, nil).Connect(ctx, transport, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cs.ListTools(ctx, nil); err != nil {
		t.Error(err)
	}
	if _, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "preview", Arguments: map[string]any{}}); err != nil {
		t.Error(err)
	}
	if _, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "get_spec", Arguments: map[string]any{"dir": ".."}}); err != nil {
		t.Error(err)
	}
	if _, err := cs.GetPrompt(ctx, &mcp.GetPromptParams{Name: "import_instructions"}); err != nil {
		t.Error(err)
	}
	if err := cs.Close(); err != nil {
		t.Error(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Errorf("cyanotype mcp: %v\n%s", err, stderr.String())
	}

	lines := 0
	sc := bufio.NewScanner(&raw)
	sc.Buffer(nil, 10<<20)
	for sc.Scan() {
		lines++
		var msg map[string]any
		if err := json.Unmarshal(sc.Bytes(), &msg); err != nil || msg["jsonrpc"] != "2.0" {
			t.Errorf("stdout line %d is not a JSON-RPC message: %q", lines, sc.Text())
		}
	}
	if lines < 5 {
		t.Errorf("stdout has %d lines, want an answer to each request", lines)
	}
	for _, want := range []string{"cyanotype mcp started", "tool=preview", "tool=get_spec", "errors=[outside-allowed-folders]"} {
		if !strings.Contains(stderr.String(), want) {
			t.Errorf("stderr lacks %q:\n%s", want, stderr.String())
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "cyanotype.lock")); err == nil {
		t.Error("preview wrote cyanotype.lock")
	}
}
