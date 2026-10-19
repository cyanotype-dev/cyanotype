package render

import (
	"slices"
	"strings"

	"github.com/cyanotype-dev/cyanotype/internal/resolve"
	"github.com/cyanotype-dev/cyanotype/internal/spec"
)

// Section titles of the main instruction file. Sections always appear in the
// standard order of docs/DESIGN.md (Generated file quality), and a section
// with nothing to say is left out. Project structure has no source in the
// spec yet, so it is never generated; a protected block can hold it.
const (
	titleOverview  = "Project overview"
	titleLanguages = "Language policy"
	titleStack     = "Tech stack"
	titleCommands  = "Commands"
	titleStyle     = "Code style"
	titleTesting   = "Testing"
	titleGit       = "Git workflow"
	titleWorkflow  = "Agent workflow"
	titleOther     = "Other rules"
	titleSafety    = "Safety rules"
	titleDomain    = "Domain notes"
	titleReading   = "Further reading"
)

// topicSections maps each catalog topic the renderer knows to the section
// its entries go in. An unknown topic gets a section of its own, titled from
// the topic, so a new topic in the catalog needs no code change.
var topicSections = map[string]string{
	"architecture": titleStack,
	"tooling":      titleStack,
	"style":        titleStyle,
	"i18n":         titleStyle,
	"testing":      titleTesting,
	"git":          titleGit,
	"workflow":     titleWorkflow,
	"safety":       titleSafety,
}

// mainDocument builds the main instruction file: everything the spec and
// the selected entries say, in the standard section order.
func mainDocument(r *resolve.Result) document {
	s := r.Spec
	rules, extra := entryBullets(r.Entries)

	var sections []section
	add := func(title string, body []string) {
		if len(body) > 0 {
			sections = append(sections, section{title: title, body: body})
		}
	}
	add(titleOverview, overview(s.Project))
	add(titleLanguages, languagePolicy(s.Languages))
	add(titleStack, techStack(r.Entries, rules[titleStack]))
	add(titleCommands, commands(s.Commands))
	add(titleStyle, slices.Concat(rules[titleStyle], i18nRules(s.I18n)))
	add(titleTesting, rules[titleTesting])
	add(titleGit, rules[titleGit])
	add(titleWorkflow, rules[titleWorkflow])
	for _, title := range extra {
		add(title, rules[title])
	}
	add(titleOther, rules[titleOther])
	add(titleSafety, rules[titleSafety])
	add(titleDomain, domainNotes(s.Answers["domain_notes"], s.CustomRules))
	add(titleReading, furtherReading(s.Project))

	return document{preamble: []string{"# " + oneLine(s.Project.Name)}, sections: sections}
}

// entryBullets turns each entry into a bullet and groups the bullets by
// section title, keeping the order of the entries. extra lists the titles
// of sections made for unknown topics, sorted by topic, so their position
// does not depend on which entry was selected first.
func entryBullets(entries []resolve.Entry) (rules map[string][]string, extra []string) {
	rules = make(map[string][]string)
	var unknown []string
	for _, e := range entries {
		title, known := topicSections[e.Topic]
		switch {
		case known:
		case e.Topic == "":
			title = titleOther
		default:
			title = topicTitle(e.Topic)
			if !slices.Contains(unknown, e.Topic) {
				unknown = append(unknown, e.Topic)
			}
		}
		rules[title] = append(rules[title], bullet(e.Snippet.Rule, e.Snippet.Why))
	}
	slices.Sort(unknown)
	for _, topic := range unknown {
		extra = append(extra, topicTitle(topic))
	}
	return rules, extra
}

// bullet formats a rule and its reason as one list item. Multi-line rules
// are joined into one line: the shortest form keeps the file under its line
// budget.
func bullet(rule, why string) string {
	return "- " + oneLine(rule) + " Why: " + oneLine(why)
}

func overview(p spec.Project) []string {
	body := []string{oneLine(p.Goal)}
	if p.Users != "" {
		body = append(body, "", "Users: "+oneLine(p.Users))
	}
	if len(p.CoreFeatures) > 0 {
		body = append(body, "", "Core features:")
		body = append(body, listItems(p.CoreFeatures)...)
	}
	if len(p.OutOfScope) > 0 {
		body = append(body, "", "Out of scope, do not build:")
		body = append(body, listItems(p.OutOfScope)...)
	}
	return body
}

// languagePolicy groups the kinds of text written in the same language on
// one line, such as "Code, comments and docs: English.", in the order of
// their first appearance. Replies always get their own line, since they are
// the setting people change most.
func languagePolicy(l spec.Languages) []string {
	kinds := []struct{ label, lang string }{
		{"code", l.Code},
		{"comments", l.Comments},
		{"docs", l.Docs},
		{"commit messages", l.Commits},
	}
	var order []string
	labels := make(map[string][]string)
	for _, k := range kinds {
		if _, seen := labels[k.lang]; !seen {
			order = append(order, k.lang)
		}
		labels[k.lang] = append(labels[k.lang], k.label)
	}
	body := make([]string, 0, len(order)+1)
	for _, lang := range order {
		body = append(body, "- "+capitalize(joinAnd(labels[lang]))+": "+languageName(lang)+".")
	}
	return append(body, "- Replies to the user: "+languageName(l.AgentReplies)+".")
}

// practiceTopics hold ways of working rather than technologies, so their
// entries stay out of the Tech stack list even when they have a url, such
// as Conventional Commits.
var practiceTopics = []string{"git", "workflow", "safety"}

// techStack opens with the names of the selected technologies, the entries
// with a url outside practiceTopics, then lists the architecture and tooling
// rules.
func techStack(entries []resolve.Entry, rules []string) []string {
	var names []string
	for _, e := range entries {
		if e.URL != "" && !slices.Contains(practiceTopics, e.Topic) {
			names = append(names, e.Name)
		}
	}
	var body []string
	if len(names) > 0 {
		body = append(body, "Uses: "+strings.Join(names, ", ")+".")
	}
	if len(body) > 0 && len(rules) > 0 {
		body = append(body, "")
	}
	return append(body, rules...)
}

func commands(c spec.Commands) []string {
	all := []struct{ label, cmd string }{
		{"Dev", c.Dev},
		{"Build", c.Build},
		{"Test", c.Test},
		{"Lint", c.Lint},
	}
	var body []string
	for _, x := range all {
		if x.cmd != "" {
			body = append(body, "- "+x.label+": "+codeSpan(x.cmd))
		}
	}
	return body
}

// i18nRules states the app's locales and who translates them. i is the
// spec's i18n settings with defaults applied, or nil.
func i18nRules(i *spec.I18n) []string {
	if i == nil {
		return nil
	}
	if len(i.Locales) <= 1 {
		return []string{"- App locale: " + i.DefaultLocale + "."}
	}
	locales := make([]string, len(i.Locales))
	for j, l := range i.Locales {
		locales[j] = l
		if l == i.DefaultLocale {
			locales[j] += " (default)"
		}
	}
	var who string
	switch i.TranslationsBy {
	case spec.TranslationsAgentDefaultOnly:
		who = "Write new text in the default locale only and mark it as untranslated in the others."
	case spec.TranslationsHuman:
		who = "Write new text in the default locale only and mark it as untranslated in the others; human translators fill them in."
	default:
		who = "Translate new text into every locale yourself."
	}
	return []string{"- App locales: " + joinAnd(locales) + ". " + who}
}

// domainNotes keeps the notes' own line breaks, since they may hold several
// paragraphs, and lists the custom rules after them: both are project
// knowledge no catalog can have.
func domainNotes(notes string, custom []spec.CustomRule) []string {
	var body []string
	if notes != "" {
		body = append(body, textLines(notes)...)
	}
	if len(body) > 0 && len(custom) > 0 {
		body = append(body, "")
	}
	for _, c := range custom {
		body = append(body, bullet(c.Rule, c.Why))
	}
	return body
}

// furtherReading lists documents the agent reads on demand, so they cost no
// context until a task needs them.
func furtherReading(p spec.Project) []string {
	if p.Brief == "" {
		return nil
	}
	return []string{"- Before adding a feature, read " + p.Brief + "."}
}

func listItems(items []string) []string {
	out := make([]string, len(items))
	for i, item := range items {
		out[i] = "- " + oneLine(item)
	}
	return out
}

// oneLine joins the lines of s with single spaces, dropping blank lines and
// the spaces around each line.
func oneLine(s string) string {
	var parts []string
	for line := range strings.SplitSeq(s, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			parts = append(parts, line)
		}
	}
	return strings.Join(parts, " ")
}

// textLines splits s into lines without trailing spaces or surrounding blank
// lines, keeping blank lines inside as paragraph breaks.
func textLines(s string) []string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t\r")
	}
	return lines
}

// codeSpan formats s as inline code, with a longer fence when s itself
// contains backticks.
func codeSpan(s string) string {
	longest, run := 0, 0
	for _, r := range s {
		if r == '`' {
			run++
			longest = max(longest, run)
		} else {
			run = 0
		}
	}
	fence := strings.Repeat("`", longest+1)
	if longest > 0 {
		return fence + " " + s + " " + fence
	}
	return fence + s + fence
}

// joinAnd joins items as an English list without a serial comma: "a",
// "a and b", "a, b and c".
func joinAnd(items []string) string {
	if len(items) <= 1 {
		return strings.Join(items, "")
	}
	return strings.Join(items[:len(items)-1], ", ") + " and " + items[len(items)-1]
}

// topicTitle turns a kebab-case topic such as "code-review" into a section
// title such as "Code review".
func topicTitle(topic string) string {
	return capitalize(strings.ReplaceAll(topic, "-", " "))
}

// languageName turns a spec language such as "brazilian-portuguese" into
// its display name, "Brazilian Portuguese".
func languageName(lang string) string {
	words := strings.Split(lang, "-")
	for i, w := range words {
		words[i] = capitalize(w)
	}
	return strings.Join(words, " ")
}

// capitalize upper-cases the first letter of s. Spec values are ASCII, so
// one byte is one letter.
func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
