package adopt

import (
	"fmt"
	"slices"
	"strings"

	"github.com/cyanotype-dev/cyanotype/internal/catalog"
	"github.com/cyanotype-dev/cyanotype/internal/detect"
)

// GoalPlaceholder is the project.goal of a draft spec, which only the
// instruction files can fill in. A goal is required, so a draft without
// one would not be a valid spec; the placeholder says what to put there,
// and stays visible in generated files if nobody replaces it.
const GoalPlaceholder = "TODO: the goal of the project in one or two sentences, from the instruction files"

// PromptInput is what Prompt needs to know about the project.
type PromptInput struct {
	// Files are the hand-written instruction files to convert, such as
	// CLAUDE.md, relative to the project root.
	Files []string
	// Spec is the project's cyanotype.yaml when it has one, which the
	// agent updates instead of writing a new one.
	Spec []byte
	// Draft is a cyanotype.yaml cyanotype filled in from what it detected,
	// for a project without one. The agent starts from it.
	Draft []byte
	// Found is what detection found, shown with the reason for each entry.
	Found detect.Result
	// Stack holds the entry ids the project is known to use, from Found
	// or Spec: the catalog entries the prompt lists are those whose
	// requirements the stack meets. With an empty stack every entry is
	// listed.
	Stack []string
}

// Prompt returns a prompt for the user's coding agent that converts
// hand-written instruction files into cyanotype.yaml: the task, the spec
// format, the catalog entries that fit the project, and what cyanotype
// already found. It tells the agent to map each rule to an equivalent
// catalog entry when one exists, to turn the rest into custom rules with
// their reason, to move commands and the project description to their
// fields, to invent nothing, to leave the original files alone, and to end
// with what it could not convert.
//
// The agent does the conversion, not cyanotype, because only a language
// model can tell that "never touch the tests to get green" is the catalog's
// no-test-tampering. Interfaces print the prompt; the MCP server will
// return the same text as a prompt.
func Prompt(c *catalog.Catalog, in PromptInput) string {
	var b strings.Builder
	files := andList(in.Files)
	target := "write cyanotype.yaml at the project root"
	if in.Spec != nil {
		target = "update cyanotype.yaml, which already exists"
	}

	fmt.Fprintf(&b, "# Convert %s into cyanotype.yaml\n\n", files)
	fmt.Fprintf(&b, "This project has instruction files for coding agents written by hand: %s. "+
		"Cyanotype generates these files from cyanotype.yaml, which records choices from its catalog and the project's own rules. "+
		"Your task: %s, so that the files cyanotype generates from it carry the same rules as %s.\n\n", files, target, files)

	b.WriteString("## Steps\n\n")
	steps := []string{
		fmt.Sprintf("Read %s completely.", files),
		"For each rule, look for an equivalent entry in the catalog list below. When there is one, add its id to `selections` and do not repeat the rule anywhere else. An entry is equivalent when its summary says the same thing; a rule that is stricter or narrower than the entry is not, and becomes a custom rule.",
		"Turn every rule with no equivalent entry into an item of `custom_rules`, with `rule` and `why`. Take the reason from the file. When the file gives none, ask me for it; never invent a reason.",
		"Move the commands the file gives for running, building, testing and linting the project to `commands`, which has only the keys `dev`, `build`, `test` and `lint`.",
		"Move the description of the project to `project`: `goal` in one or two sentences, and `users`, `core_features` and `out_of_scope` where the file says them. Business rules no entry covers go in `answers.domain_notes`.",
		"Invent nothing: no entry, command, reason, feature or language the files or the detected stack below do not support. When the files and the detected stack disagree, ask me.",
		fmt.Sprintf("Do not edit, move or delete %s. Write only cyanotype.yaml.", files),
		"Check the result with `cyanotype generate --dry-run --existing keep`, which reports any error in cyanotype.yaml with its line and writes nothing; exit code 3 means the spec is valid. Fix every error it reports.",
		fmt.Sprintf("End your reply with a list of everything in %s you could not convert, each with the reason, then tell me to review cyanotype.yaml and run `cyanotype generate --existing replace` to replace %s with the generated files.", files, files),
	}
	for i, s := range steps {
		fmt.Fprintf(&b, "%d. %s\n", i+1, s)
	}

	if !in.Found.Empty() {
		b.WriteString("\n## What cyanotype detected\n\n")
		b.WriteString("Detected from the project's files; check each one against the instruction files:\n\n")
		for _, f := range in.Found.Findings {
			e, _ := c.Get(f.ID)
			parts := make([]string, len(f.Evidence))
			for i, ev := range f.Evidence {
				parts[i] = ev.String()
			}
			fmt.Fprintf(&b, "- `%s` (%s): %s\n", f.ID, e.Name, strings.Join(parts, ", "))
		}
		// Whether the project really uses each entry is the user's choice,
		// as --detect and --without are in init: the agent asks, never picks.
		b.WriteString("\nA file can reveal something the project no longer uses. When the instruction files do not mention a detected entry and you are not sure the project uses it, ask me; never decide for me.\n")
		for _, cl := range in.Found.Clashes {
			fmt.Fprintf(&b, "\nOnly one of %s can be selected. Keep the one the instruction files name; if they name none, ask me.\n", andList(backtick(cl.IDs)))
		}
	}

	switch {
	case in.Spec != nil:
		b.WriteString("\n## The current cyanotype.yaml\n\n")
		b.WriteString("Keep its choices unless the instruction files contradict them, and keep its comments.\n\n")
		writeYAML(&b, in.Spec)
	case in.Draft != nil:
		b.WriteString("\n## Draft\n\n")
		b.WriteString("Start from this draft, which cyanotype filled in from the project's files. Replace the placeholder in `project.goal`, which is required, with the goal the instruction files give.\n\n")
		writeYAML(&b, in.Draft)
	}

	b.WriteString("\n## The cyanotype.yaml format\n\n")
	b.WriteString(specFormat(c.Version()))

	b.WriteString("\n## Catalog entries\n\n")
	b.WriteString("Each line is `id`: name, then what the entry makes the agent do. Only these ids can go in `selections`. ")
	b.WriteString("Entries marked radio are exclusive within their category: select at most one of them.\n")
	writeEntries(&b, c, in.Stack)
	return b.String()
}

// specExample is an annotated cyanotype.yaml. A test loads it, so it never
// drifts from the real format; %s is the catalog version.
const specExample = `version: 1
catalog_version: "%s"
targets: [claude, agents-md]   # claude writes CLAUDE.md, agents-md AGENTS.md
project:
  name: shop                   # one line
  goal: Online shop for handmade furniture.   # required, one or two sentences
  users: Furniture buyers, mostly on mobile.  # optional
  core_features:               # optional
    - Browse and buy products
  out_of_scope:                # optional
    - A marketplace for other sellers
languages:                     # optional; each is english by default
  agent_replies: italian
commands:                      # optional; only these four keys, each one line
  dev: pnpm dev
  build: pnpm build
  test: pnpm test
  lint: pnpm lint
selections:                    # catalog entry ids, in the order their rules should appear
  - typescript
  - node
  - pnpm
  - no-test-tampering
answers:
  domain_notes: Prices include VAT.   # optional: business rules no entry covers
custom_rules:                  # rules with no equivalent entry
  - rule: Store prices in cents, as integers.
    why: Floating-point sums lose cents.
`

// specFormat explains the spec format with the annotated example.
func specFormat(catalogVersion string) string {
	var b strings.Builder
	b.WriteString("An example with every field the conversion needs:\n\n")
	writeYAML(&b, fmt.Appendf(nil, specExample, catalogVersion))
	b.WriteString("\nRules:\n\n")
	for _, r := range []string{
		"`version`, `targets`, `project.name` and `project.goal` are required. Keep `version: 1` and `catalog_version` as they are.",
		"`targets` lists the agents the files are for: `claude` for CLAUDE.md, `agents-md` for AGENTS.md, which Codex and most other agents read.",
		"Unknown fields are errors. Leave out fields with nothing to say instead of writing them empty.",
		"An entry's requirements must be selected too: the catalog list says what each entry requires.",
		"Every custom rule needs both `rule` and `why`. Write the rule as an instruction to the agent, and the reason as one short sentence.",
	} {
		fmt.Fprintf(&b, "- %s\n", r)
	}
	return b.String()
}

// writeEntries lists the catalog entries that fit stack, grouped by
// category in catalog order: those whose requirements stack meets, or
// every entry when stack is empty. Listing a FastAPI rule for a Node.js
// project would only invite a wrong match.
func writeEntries(b *strings.Builder, c *catalog.Catalog, stack []string) {
	var categories []string
	byCategory := map[string][]catalog.Entry{}
	for _, e := range c.Entries() {
		if len(stack) > 0 && !e.RequirementsMet(stack) {
			continue
		}
		if _, ok := byCategory[e.Category]; !ok {
			categories = append(categories, e.Category)
		}
		byCategory[e.Category] = append(byCategory[e.Category], e)
	}
	slices.Sort(categories)
	for _, cat := range categories {
		entries := byCategory[cat]
		kind := "checkbox: any number"
		if entries[0].Select == catalog.SelectRadio {
			kind = "radio: at most one"
		}
		fmt.Fprintf(b, "\n### %s (%s)\n\n", cat, kind)
		for _, e := range entries {
			fmt.Fprintf(b, "- `%s`: %s. %s.%s\n", e.ID, e.Name, strings.TrimSuffix(e.Summary, "."), requirementsNote(e))
		}
	}
}

// requirementsNote says what an entry requires, for the catalog list.
func requirementsNote(e catalog.Entry) string {
	var parts []string
	if len(e.Requires) > 0 {
		parts = append(parts, strings.Join(backtick(e.Requires), " and "))
	}
	for _, group := range e.RequiresOneOf {
		parts = append(parts, "one of "+strings.Join(backtick(group), " or "))
	}
	if len(parts) == 0 {
		return ""
	}
	return " Requires " + strings.Join(parts, ", and ") + "."
}

// writeYAML writes data as a fenced YAML block.
func writeYAML(b *strings.Builder, data []byte) {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	b.WriteString("```yaml\n" + strings.TrimRight(text, "\n") + "\n```\n")
}

func backtick(ids []string) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = "`" + id + "`"
	}
	return out
}

// andList joins items as an English list: "a", "a and b", "a, b and c".
func andList(items []string) string {
	if len(items) <= 1 {
		return strings.Join(items, "")
	}
	return strings.Join(items[:len(items)-1], ", ") + " and " + items[len(items)-1]
}
