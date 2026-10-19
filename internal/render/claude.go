package render

import "slices"

// titleClaude is the section holding Claude-only content. It goes in
// CLAUDE.md only, so AGENTS.md stays readable by any agent.
const titleClaude = "Claude Code"

// claudeSection tells Claude what the generated deny rules are for. A
// denied action invites a workaround, such as writing the file through the
// shell instead; the deny rules only work if the agent stops instead.
func claudeSection() section {
	return section{title: titleClaude, body: []string{
		"- Deny rules in .claude/settings.json back the safety rules; when one blocks an action, stop and ask instead of reaching the same result another way.",
	}}
}

// claudeImportDocument builds CLAUDE.md for a project that also targets
// AGENTS.md. It imports AGENTS.md instead of repeating it: in Claude Code a
// CLAUDE.md that is present stops AGENTS.md from being read, so without the
// import Claude would see none of the shared rules. Only Claude-specific
// content belongs here: the Claude Code section when enforced reports that
// deny rules are generated.
func claudeImportDocument(enforced bool) document {
	d := document{preamble: []string{"@" + agentsPath}}
	if enforced {
		d.sections = []section{claudeSection()}
	}
	return d
}

// withClaudeSection adds the Claude Code section to a full CLAUDE.md, right
// after the safety rules it backs, or before Domain notes and Further
// reading when there are no safety rules.
func withClaudeSection(d document) document {
	at := slices.IndexFunc(d.sections, func(s section) bool { return s.title == titleDomain || s.title == titleReading })
	if i := slices.IndexFunc(d.sections, func(s section) bool { return s.title == titleSafety }); i >= 0 {
		at = i + 1
	}
	if at < 0 {
		at = len(d.sections)
	}
	d.sections = slices.Insert(slices.Clone(d.sections), at, claudeSection())
	return d
}
