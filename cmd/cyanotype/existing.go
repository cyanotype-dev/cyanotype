package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"slices"
	"strings"

	"github.com/cyanotype-dev/cyanotype/internal/adopt"
	"github.com/cyanotype-dev/cyanotype/internal/generate"
	"github.com/cyanotype-dev/cyanotype/internal/render"
	"github.com/cyanotype-dev/cyanotype/internal/simple"
)

// existingFlagHelp is the help of --existing, shared by init and generate.
const existingFlagHelp = "what to do with instruction files written by hand: import (recommended), keep, move or replace"

// checkExistingFlag rejects an --existing value that is not a choice.
func checkExistingFlag(value string, changed bool) error {
	if !changed {
		return nil
	}
	if _, err := adopt.ParseChoice(value); err != nil {
		return fmt.Errorf("--existing %w", err)
	}
	return nil
}

// existingChooser decides what to do with each instruction file written by
// hand: from --existing for every file, or by asking about each one.
type existingChooser struct {
	// flag is the value of --existing, "" when it was not given.
	flag string
	// ask is the prompter, nil when cyanotype must not ask: no terminal,
	// or --yes.
	ask *prompter
	// dir is the project folder, where git checks a file before replace.
	dir string
	// topicDir is where move puts a file.
	topicDir string
	errOut   io.Writer
	git      adopt.Git
}

// choose returns a choice for every path. Without --existing and without a
// way to ask, it is a usage error that explains each choice: a
// hand-written file is never replaced silently, not even with --yes.
// Replace is chosen only for a file git can give back, and on a terminal
// only after a confirmation of its own.
func (x existingChooser) choose(paths []string) (generate.Choices, error) {
	choices := generate.Choices{}
	if x.flag != "" {
		c, err := adopt.ParseChoice(x.flag)
		if err != nil {
			return nil, usageError(fmt.Errorf("--existing %w", err))
		}
		for _, p := range paths {
			if c == adopt.Replace {
				if err := x.git.CheckRecoverable(x.dir, p); err != nil {
					return nil, fmt.Errorf("refusing to replace %s: %w (--existing keep or --existing move)", p, err)
				}
			}
			choices[p] = c
		}
		return choices, nil
	}
	if x.ask == nil {
		return nil, usageError(x.missingError(paths))
	}
	for _, p := range paths {
		c, err := x.askOne(p)
		if err != nil {
			return nil, err
		}
		choices[p] = c
	}
	return choices, nil
}

// options returns the choices offered for the file at path.
func (x existingChooser) options(path string) []simple.Option {
	return []simple.Option{
		{Value: string(adopt.Import), Label: "Import it with your coding agent (recommended)",
			Help: "print a prompt that has your agent turn its rules into cyanotype.yaml; nothing is written"},
		{Value: string(adopt.Keep), Label: "Keep it as a custom block",
			Help: "the whole file goes in a cyanotype:custom block, which regeneration keeps"},
		{Value: string(adopt.Move), Label: "Move it to a separate file",
			Help: "its content goes to " + adopt.MovePath(x.topicDir, path) + ", which the main file tells the agent to read"},
		{Value: string(adopt.Replace), Label: "Replace it",
			Help: "overwrite it with the generated file; only when git has it committed, so it can be recovered"},
	}
}

// askOne asks what to do with one file. A replace that git cannot back is
// explained and the question asked again without it; a replace the user
// does not confirm asks again too.
func (x existingChooser) askOne(path string) (adopt.Choice, error) {
	opts := x.options(path)
	question := fmt.Sprintf("%s has no cyanotype header, so it was written by hand. What should cyanotype do with it?", path)
	for {
		o, err := x.ask.choose(question, opts, string(adopt.Import))
		if err != nil {
			return "", err
		}
		c := adopt.Choice(o.Value)
		if c != adopt.Replace {
			return c, nil
		}
		if err := x.git.CheckRecoverable(x.dir, path); err != nil {
			fmt.Fprintf(x.errOut, "Cannot replace %s: %v.\n", path, err)
		} else {
			fmt.Fprintf(x.errOut, "\n%s is committed in git with no changes, so \"git show HEAD:%s\" gives it back.\n", path, path)
			ok, err := x.ask.yesNo(fmt.Sprintf("Replace %s with the generated file?", path), false)
			if err != nil {
				return "", err
			}
			if ok {
				return c, nil
			}
		}
		opts = slices.DeleteFunc(opts, func(o simple.Option) bool { return o.Value == string(adopt.Replace) })
	}
}

// missingError explains the choices when cyanotype cannot ask.
func (x existingChooser) missingError(paths []string) error {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s no cyanotype header, so %s written by hand, and cyanotype never replaces %s without your choice, not even with --yes.\n",
		andList(paths), byCount(len(paths), "has", "have"), byCount(len(paths), "it was", "they were"), byCount(len(paths), "it", "them"))
	b.WriteString("Run the command again with one of:\n")
	for _, o := range x.options(paths[0]) {
		help := o.Help
		if o.Value == string(adopt.Move) {
			help = "its content goes to " + x.topicDir + "/original-<name>, which the main file tells the agent to read"
		}
		fmt.Fprintf(&b, "  --existing %-8s %s\n", o.Value, help)
	}
	return errors.New(strings.TrimSuffix(b.String(), "\n"))
}

// handWrittenFiles returns the main instruction files at the root of the
// project that cyanotype did not generate, for init, which looks for them
// before asking anything.
func handWrittenFiles(project fs.FS) []string {
	var out []string
	for _, p := range []string{"AGENTS.md", "CLAUDE.md"} {
		data, err := fs.ReadFile(project, p)
		if err == nil && !render.HasHeader(data) {
			out = append(out, p)
		}
	}
	return out
}

// printImportPrompt prints the import prompt on stdout, where it can be
// redirected or piped to an agent, and what to do with it on stderr.
func printImportPrompt(out, errOut io.Writer, prompt string, paths []string, regenerate string) {
	fmt.Fprint(out, prompt)
	fmt.Fprintf(errOut, "\nNothing was written. The prompt above, on stdout, asks your coding agent to turn %s into cyanotype.yaml.\n"+
		"Give it to the agent, for example by piping this command into it, then review cyanotype.yaml and run \"%s\".\n",
		andList(paths), regenerate)
}

// choiceNote says, in the summary, what a choice does to a file.
func choiceNote(ch generate.Change, topicDir string) string {
	switch {
	case ch.MovedFrom != "":
		return " (the content of " + ch.MovedFrom + ", byte for byte)"
	case !ch.HandWritten:
		return ""
	}
	switch ch.Choice {
	case adopt.Keep:
		return " (written by hand: kept whole in a cyanotype:custom block)"
	case adopt.Move:
		return " (written by hand: its content moves to " + adopt.MovePath(topicDir, ch.Path) + ")"
	case adopt.Replace:
		return " (written by hand: replaced; git show HEAD:" + ch.Path + " gives it back)"
	}
	return " (written by hand: waiting for a choice)"
}

// andList joins items as an English list: "a", "a and b", "a, b and c".
func andList(items []string) string {
	return orList(items, "and")
}

// byCount picks the singular or plural form for n.
func byCount(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
