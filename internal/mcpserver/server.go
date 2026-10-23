package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/cyanotype-dev/cyanotype/internal/catalog"
	"github.com/cyanotype-dev/cyanotype/internal/contract"
	"github.com/cyanotype-dev/cyanotype/internal/preset"
)

// instructions tell the connected agent how to use the tools. The rules
// that matter are repeated in the tools' descriptions, since a client may
// not show the instructions to the model.
const instructions = "Cyanotype generates the instruction files of this project (CLAUDE.md, AGENTS.md) and their constraints from cyanotype.yaml and a catalog of rules. " +
	"Use preview to see what generate would change, as diffs, before generate writes anything. " +
	"Pass write: true to generate or set_selections only when the user asked for that change. " +
	"Change cyanotype.yaml, its selections included, only when the user explicitly asks. " +
	"Every result is the JSON that cyanotype prints with --json; its errors array says what went wrong."

// Options configure the server.
type Options struct {
	// Folders are the folders the tools may use.
	Folders *Folders
	// Version is cyanotype's version, which the server reports to clients.
	Version string
	// Logger gets one line per call. It must never write to stdout, which
	// carries the protocol.
	Logger *slog.Logger
}

// server holds what the handlers share.
type server struct {
	cat     *catalog.Catalog
	presets []preset.Preset
	folders *Folders
	log     *slog.Logger
	// writeMu runs the calls that write one at a time: the SDK may handle
	// calls concurrently, and two writes to one project must not mix.
	writeMu sync.Mutex
}

// New returns the MCP server, with its tools and the import_instructions
// prompt. Run it with Server.Run on a transport, such as
// mcp.StdioTransport.
//
// Each tool calls the same code as the matching CLI command and returns
// the bytes that command prints with --json, both as text and as
// structured content. No tool writes unless its write argument is true,
// and every project is opened through Folders, inside the allowed folders.
func New(c *catalog.Catalog, presets []preset.Preset, opts Options) (*mcp.Server, error) {
	if opts.Folders == nil {
		return nil, errors.New("no allowed folders")
	}
	log := opts.Logger
	if log == nil {
		log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	s := &server{cat: c, presets: presets, folders: opts.Folders, log: log}
	srv := mcp.NewServer(&mcp.Implementation{Name: "cyanotype", Title: "Cyanotype", Version: opts.Version}, &mcp.ServerOptions{
		Instructions: instructions,
		Logger:       log,
		// An empty set, not the SDK's default, which offers logging, a
		// feature the server does not use.
		Capabilities: &mcp.ServerCapabilities{},
	})
	for _, t := range s.tools() {
		schema, err := outputSchema(t.output)
		if err != nil {
			return nil, fmt.Errorf("tool %s: %w", t.tool.Name, err)
		}
		t.tool.OutputSchema = schema
		srv.AddTool(t.tool, s.handler(t))
	}
	srv.AddPrompt(importPrompt, s.importInstructions)
	return srv, nil
}

// tool is one tool: its definition, the output.schema.json definition of
// its result, and what it does.
type tool struct {
	tool   *mcp.Tool
	output string
	// writes reports that a call may write, so it holds writeMu.
	writes bool
	// run decodes the arguments and does the work.
	run func(args json.RawMessage) contract.Output
}

// handler returns the SDK handler for t: it runs the tool, logs the call
// without its arguments, and returns the JSON as text and as structured
// content. A result with errors is a failed call, so the model sees it.
func (s *server) handler(t tool) mcp.ToolHandler {
	return func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if t.writes {
			s.writeMu.Lock()
			defer s.writeMu.Unlock()
		}
		start := time.Now()
		out := t.run(req.Params.Arguments)
		data, err := contract.Marshal(out)
		if err != nil {
			return nil, err
		}
		codes := contract.ErrorCodes(out)
		s.log.Info("tool call", "tool", t.tool.Name, "duration", time.Since(start).Round(time.Millisecond), "errors", codes)
		return &mcp.CallToolResult{
			Content:           []mcp.Content{&mcp.TextContent{Text: string(data)}},
			StructuredContent: json.RawMessage(data),
			IsError:           len(codes) > 0,
		}, nil
	}
}

// decode reads the arguments of a call into v, refusing unknown fields
// and anything after the object, so a mistyped argument is an error
// rather than silently ignored.
func decode(args json.RawMessage, v any) error {
	if len(bytes.TrimSpace(args)) == 0 {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(args))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("invalid arguments: %w", err)
	}
	if dec.More() {
		return errors.New("invalid arguments: more than one JSON value")
	}
	return nil
}

// usage returns the diagnostics of arguments a tool cannot use.
func usage(err error) contract.Diagnostics {
	return contract.Diagnostics{Errors: []contract.Diagnostic{contract.Error(contract.CodeUsage, err.Error())}}
}

// open opens the project folder dir through the allowed folders, or
// returns the diagnostics of a folder a tool may not use.
func (s *server) open(dir string) (*os.Root, contract.Diagnostics, bool) {
	root, err := s.folders.Open(dir)
	switch {
	case err == nil:
		return root, contract.Diagnostics{}, true
	case errors.Is(err, ErrOutside):
		msg := fmt.Sprintf("%v; the server may use only %s, as cyanotype mcp --allow sets them", err, strings.Join(s.folders.Names(), ", "))
		return nil, contract.Diagnostics{Errors: []contract.Diagnostic{contract.Error(contract.CodeOutsideAllowedFolders, msg)}}, false
	case errors.Is(err, ErrDirRequired), errors.Is(err, ErrInvalidPath):
		return nil, usage(err), false
	}
	return nil, contract.Diagnostics{Errors: contract.Errors(err)}, false
}

// dirArgs are the arguments of a tool that works on a project.
type dirArgs interface{ dir() string }

// inProject returns the run function of a tool that works on a project:
// it decodes the arguments into A, refusing unknown ones, opens dir
// through the allowed folders and calls do with the project. fail builds
// the tool's output type for an error that stops it before do.
func inProject[A dirArgs](s *server, fail func(contract.Diagnostics) contract.Output, do func(root *os.Root, a A) contract.Output) func(json.RawMessage) contract.Output {
	return func(raw json.RawMessage) contract.Output {
		var a A
		if err := decode(raw, &a); err != nil {
			return fail(usage(err))
		}
		root, d, ok := s.open(a.dir())
		if !ok {
			return fail(d)
		}
		defer root.Close()
		return do(root, a)
	}
}
