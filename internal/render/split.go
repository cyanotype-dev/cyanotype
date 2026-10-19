package render

import (
	"bytes"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"

	"github.com/cyanotype-dev/cyanotype/internal/resolve"
	"github.com/cyanotype-dev/cyanotype/internal/spec"
)

// mainTopics are the topics whose entries stay in the main file when output
// is split. Safety rules must never depend on the agent opening another
// file, and tooling rules such as "use pnpm, never npm" apply to almost
// every task: an agent may run npm install without ever opening a topic
// file. Entries without a topic also stay, since they have no file to go to.
var mainTopics = []string{"safety", "tooling"}

// topicFile describes the file a known topic moves to when output is split.
type topicFile struct {
	// title is the file's "# " heading.
	title string
	// trigger says when to read the file, in the main file's pointer to it:
	// "<trigger>, read docs/agents/testing.md."
	trigger string
}

// topicFiles holds the known topics that can move out of the main file.
var topicFiles = map[string]topicFile{
	"architecture": {"Architecture", "Before adding a dependency or changing how the app is structured"},
	"style":        {"Code style", "Before writing code"},
	"i18n":         {"Internationalization", "Before adding or changing user-facing text"},
	"testing":      {"Testing", "Before writing or changing tests"},
	"git":          {"Git workflow", "Before committing or opening a pull request"},
	"workflow":     {"Agent workflow", "Before starting a task"},
}

// topicOrder is the order of the known topics in topicFiles, following the
// sections of the main file, so pointers read in the same order as the
// unsplit file.
var topicOrder = []string{"architecture", "style", "i18n", "testing", "git", "workflow"}

// layout says how the content is spread over files.
type layout struct {
	// split moves every entry outside mainTopics to a file per topic.
	split bool
	// topicDir is the folder for topic files, relative to the project root.
	topicDir string
}

// inMain reports whether entries of topic stay in the main file.
func (l layout) inMain(topic string) bool {
	return !l.split || topic == "" || slices.Contains(mainTopics, topic)
}

// splitTopics returns the topics of entries that move to topic files: the
// known ones in section order, then unknown ones sorted by name.
func (l layout) splitTopics(entries []resolve.Entry) []string {
	var known, unknown []string
	for _, e := range entries {
		switch {
		case l.inMain(e.Topic):
		case slices.Contains(topicOrder, e.Topic):
			if !slices.Contains(known, e.Topic) {
				known = append(known, e.Topic)
			}
		case !slices.Contains(unknown, e.Topic):
			unknown = append(unknown, e.Topic)
		}
	}
	slices.SortFunc(known, func(a, b string) int {
		return slices.Index(topicOrder, a) - slices.Index(topicOrder, b)
	})
	slices.Sort(unknown)
	return append(known, unknown...)
}

func (l layout) topicPath(topic string) string {
	return path.Join(l.topicDir, topic+".md")
}

// pointers returns one line per topic file telling the agent when to read
// it. Topic files are read on demand, never imported: splitting exists to
// shorten what the agent reads, and an import loads the file anyway.
func (l layout) pointers(entries []resolve.Entry) []string {
	var out []string
	for _, topic := range l.splitTopics(entries) {
		trigger := "For " + strings.ReplaceAll(topic, "-", " ") + " rules"
		if f, ok := topicFiles[topic]; ok {
			trigger = f.trigger
		}
		out = append(out, "- "+trigger+", read "+l.topicPath(topic)+".")
	}
	return out
}

// topicDocument builds the topic file for topic from the entries that have
// it, in selection order.
func topicDocument(topic string, entries []resolve.Entry) document {
	title := topicTitle(topic)
	if f, ok := topicFiles[topic]; ok {
		title = f.title
	}
	lines := []string{"# " + title, ""}
	for _, e := range entries {
		if e.Topic == topic {
			lines = append(lines, bullet(e.Snippet.Rule, e.Snippet.Why))
		}
	}
	return document{preamble: lines}
}

// chooseLayout applies output.split. For auto it renders the main file
// unsplit, protected sections included, and splits only when that file is
// longer than output.max_lines. Splitting is all or nothing, rather than
// moving topics out one by one until the file fits, so the layout stays
// predictable and adding one entry never reshuffles the files.
func chooseLayout(r *resolve.Result, existing fs.FS, mainPath string) (layout, error) {
	out := r.Spec.Output
	l := layout{topicDir: out.TopicDir}
	switch out.Split {
	case spec.SplitAlways:
		l.split = true
	case spec.SplitAuto:
		f, err := renderMarkdown(existing, mainPath, mainDocument(r, l))
		if err != nil {
			return layout{}, err
		}
		l.split = lineCount(f.Content) > out.MaxLines
	}
	return l, nil
}

// lengthWarning reports a main file longer than output.max_lines, or nil.
func lengthWarning(f File, l layout, maxLines int) []resolve.Warning {
	n := lineCount(f.Content)
	if n <= maxLines {
		return nil
	}
	msg := fmt.Sprintf("%s has %d lines, more than %d; set output.split to auto or always to move topics into %s",
		f.Path, n, maxLines, l.topicDir)
	if l.split {
		msg = fmt.Sprintf("%s still has %d lines after splitting, more than %d", f.Path, n, maxLines)
	}
	return []resolve.Warning{{Field: "output.max_lines", Msg: msg}}
}

// lineCount counts the lines of generated content, which always ends with a
// newline.
func lineCount(content []byte) int {
	return bytes.Count(content, []byte("\n"))
}
