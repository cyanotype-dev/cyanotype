package render

import (
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strings"

	"github.com/cyanotype-dev/cyanotype/internal/resolve"
	"github.com/cyanotype-dev/cyanotype/internal/spec"
)

// File is one generated file, kept in memory until the caller decides what
// to do with it. Renderers never write to disk: showing a diff, asking for
// confirmation and writing belong to the CLI and the other interfaces.
type File struct {
	// Path is relative to the project root, with forward slashes, such as
	// "AGENTS.md" or "docs/agents/testing.md".
	Path string
	// Content is the exact bytes to write: LF line endings and one trailing
	// newline.
	Content []byte
}

// Paths of the main instruction files.
const (
	agentsPath = "AGENTS.md"
	claudePath = "CLAUDE.md"
)

// Render returns every file the targets of r produce, sorted by path. The
// same input always gives byte-identical output.
//
// existing holds the project as it is now, rooted at the project folder: the
// CLI passes os.DirFS(root), tests pass an fstest.MapFS. Renderers read it
// to keep what regeneration must not lose, such as protected sections, but
// never write to it. It is an fs.FS rather than a list of files because the
// renderer, not the caller, knows which files it generates. A nil existing
// means a project with no files yet.
//
// Targets that have no renderer yet are skipped with a warning, so a spec
// that also lists them still gets its other files. The .gitignore block is
// generated whatever the targets, since it protects personal files from
// every collaborator's commits.
func Render(r *resolve.Result, existing fs.FS) ([]File, []resolve.Warning, error) {
	targets := r.Spec.Targets
	agents := slices.Contains(targets, spec.TargetAgentsMD)
	claude := slices.Contains(targets, spec.TargetClaude)

	var warnings []resolve.Warning
	for i, t := range targets {
		if t != spec.TargetAgentsMD && t != spec.TargetClaude {
			warnings = append(warnings, resolve.Warning{
				Field: fmt.Sprintf("targets[%d]", i),
				Msg:   fmt.Sprintf("cyanotype cannot generate files for %q yet; they are skipped", t),
			})
		}
	}

	var files []File
	if agents || claude {
		instructions, w, err := renderInstructions(r, existing, agents, claude)
		if err != nil {
			return nil, nil, err
		}
		files = append(files, instructions...)
		warnings = append(warnings, w...)
	}

	gitignore, err := renderGitignore(existing)
	if err != nil {
		return nil, nil, err
	}
	files = append(files, gitignore)

	if claude {
		f, ok, err := renderSettings(r, existing)
		if err != nil {
			return nil, nil, err
		}
		if ok {
			files = append(files, f)
		}
	}

	slices.SortFunc(files, func(a, b File) int { return strings.Compare(a.Path, b.Path) })
	return files, warnings, nil
}

// renderInstructions returns the Markdown instruction files: the main file,
// CLAUDE.md importing it when both targets are set, and the topic files of
// a split output.
func renderInstructions(r *resolve.Result, existing fs.FS, agents, claude bool) ([]File, []resolve.Warning, error) {
	// The main file holds the shared content: AGENTS.md when it is a
	// target, since CLAUDE.md then only imports it.
	mainPath := agentsPath
	if !agents {
		mainPath = claudePath
	}
	// enforced reports that .claude/settings.json gets deny rules, which
	// CLAUDE.md then explains in its Claude Code section.
	enforced := claude && len(denyGroups(r)) > 0
	build := func(l layout) document {
		d := mainDocument(r, l)
		if mainPath == claudePath && enforced {
			d = withClaudeSection(d)
		}
		return d
	}

	l, err := chooseLayout(r, existing, mainPath, build)
	if err != nil {
		return nil, nil, err
	}
	main, err := renderMarkdown(existing, mainPath, build(l))
	if err != nil {
		return nil, nil, err
	}
	files := []File{main}
	warnings := lengthWarning(main, l, r.Spec.Output.MaxLines)

	if agents && claude {
		// CLAUDE.md only imports AGENTS.md, so both agents read the same
		// rules from one file.
		f, err := renderMarkdown(existing, claudePath, claudeImportDocument(enforced))
		if err != nil {
			return nil, nil, err
		}
		files = append(files, f)
	}

	for _, topic := range l.splitTopics(r.Entries) {
		f, err := renderMarkdown(existing, l.topicPath(topic), topicDocument(topic, r.Entries))
		if err != nil {
			return nil, nil, err
		}
		files = append(files, f)
	}
	return files, warnings, nil
}

// renderMarkdown writes doc as the Markdown file at path, putting back the
// protected sections of the file that exists there now.
func renderMarkdown(existing fs.FS, path string, doc document) (File, error) {
	old, err := readExisting(existing, path)
	if err != nil {
		return File{}, err
	}
	blocks, err := parseCustomBlocks(old)
	if err != nil {
		return File{}, fmt.Errorf("render %s: %w", path, err)
	}
	return File{Path: path, Content: doc.render(blocks)}, nil
}

// readExisting returns the content of path in existing, or nil when the file
// does not exist.
func readExisting(existing fs.FS, path string) ([]byte, error) {
	if existing == nil {
		return nil, nil
	}
	data, err := fs.ReadFile(existing, path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read existing %s: %w", path, err)
	}
	return data, nil
}
