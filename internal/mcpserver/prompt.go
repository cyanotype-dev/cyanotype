package mcpserver

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/cyanotype-dev/cyanotype/internal/ops"
)

// importPrompt is the definition of the import_instructions prompt.
var importPrompt = &mcp.Prompt{
	Name:  "import_instructions",
	Title: "Import hand-written instruction files",
	Description: "Converts the project's hand-written instruction files, such as CLAUDE.md, into cyanotype.yaml: " +
		"the same prompt cyanotype init and cyanotype generate --existing import print. It writes nothing.",
	Arguments: []*mcp.PromptArgument{{
		Name:        "dir",
		Description: "The project folder, as for the tools. Leave it out when the server may use only one folder.",
	}},
}

// importInstructions returns the import prompt for the project in the dir
// argument. A problem is a protocol error: prompts have no result to hold
// it, unlike tools.
func (s *server) importInstructions(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	for name := range req.Params.Arguments {
		if name != "dir" {
			return nil, fmt.Errorf("unknown argument %q; import_instructions takes only dir", name)
		}
	}
	root, err := s.folders.Open(req.Params.Arguments["dir"])
	if err != nil {
		return nil, err
	}
	defer root.Close()
	prompt, err := ops.ImportInstructions(s.cat, s.presets, root)
	s.log.Info("prompt", "prompt", importPrompt.Name, "failed", err != nil)
	if err != nil {
		return nil, err
	}
	return &mcp.GetPromptResult{
		Description: importPrompt.Description,
		Messages:    []*mcp.PromptMessage{{Role: "user", Content: &mcp.TextContent{Text: prompt}}},
	}, nil
}
