package render

// claudeImportDocument builds CLAUDE.md for a project that also targets
// AGENTS.md. It imports AGENTS.md instead of repeating it: in Claude Code a
// CLAUDE.md that is present stops AGENTS.md from being read, so without the
// import Claude would see none of the shared rules. Only Claude-specific
// content belongs here.
func claudeImportDocument() document {
	return document{preamble: []string{"@" + agentsPath}}
}
