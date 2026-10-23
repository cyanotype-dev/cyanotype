package ops

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/cyanotype-dev/cyanotype/internal/adopt"
	"github.com/cyanotype-dev/cyanotype/internal/catalog"
	"github.com/cyanotype-dev/cyanotype/internal/check"
	"github.com/cyanotype-dev/cyanotype/internal/contract"
	"github.com/cyanotype-dev/cyanotype/internal/detect"
	"github.com/cyanotype-dev/cyanotype/internal/generate"
	"github.com/cyanotype-dev/cyanotype/internal/preset"
	"github.com/cyanotype-dev/cyanotype/internal/spec"
)

// GenerateOptions are the choices of one Generate run, as the flags of
// cyanotype generate and the arguments of the MCP tools preview and
// generate give them.
type GenerateOptions struct {
	// Existing is what to do with every hand-written instruction file;
	// "" leaves them waiting for a choice.
	Existing adopt.Choice
	// DryRun only shows the changes: files waiting for a choice are no
	// error then, since nothing would replace them.
	DryRun bool
	// Write writes the changes. Without it nothing is written.
	Write bool
}

// Outcome is how a run ended, which interfaces turn into an exit code or
// an MCP error flag.
type Outcome int

// The outcomes of Generate.
const (
	// Done: written, already up to date, or an import prompt returned.
	Done Outcome = iota
	// NotWritten: there are changes, and nothing was written.
	NotWritten
	// ChoiceRequired: hand-written files wait for a choice, so nothing can
	// be written; a usage error, since the caller must add one.
	ChoiceRequired
	// Failed: an error stopped the run; the output says which.
	Failed
)

// Generate compares the files the spec at root generates with the project
// and, with opts.Write, writes them. It never asks: a hand-written
// instruction file is replaced only with a choice in opts.Existing, and
// replace only when git can give the file back. With import, the output
// holds the prompt that converts the hand-written files, and nothing is
// written.
func Generate(c *catalog.Catalog, root *os.Root, opts GenerateOptions) (*contract.Generate, Outcome) {
	out := &contract.Generate{}
	fail := func(err error) (*contract.Generate, Outcome) {
		out.Errors = append(out.Errors, contract.Errors(err)...)
		return out, Failed
	}
	f, err := generate.LoadSpec(root)
	if err != nil {
		return fail(err)
	}
	changes, warnings, err := generate.Preview(c, f, root.FS())
	out.Warnings = contract.Warnings(warnings)
	if err != nil {
		return fail(err)
	}
	if hw := generate.HandWritten(changes); len(hw) > 0 {
		switch opts.Existing {
		case "":
			out = contract.NewGenerate(changes, warnings)
			if opts.DryRun {
				return out, NotWritten
			}
			topicDir := f.Spec.WithDefaults().Output.TopicDir
			out.Errors = append(out.Errors, contract.Error(contract.CodeChoiceRequired, choiceRequired(hw, topicDir)))
			return out, ChoiceRequired
		case adopt.Import:
			prompt, err := ImportPrompt(c, root, f, hw)
			if err != nil {
				return fail(err)
			}
			out = contract.NewGenerate(changes, warnings)
			out.Import = &contract.Import{Files: hw, Prompt: prompt}
			return out, Done
		}
		choices := generate.Choices{}
		for _, p := range hw {
			if opts.Existing == adopt.Replace {
				if err := (adopt.Git{}).CheckRecoverable(root.Name(), p); err != nil {
					return fail(fmt.Errorf("refusing to replace %s: %w", p, err))
				}
			}
			choices[p] = opts.Existing
		}
		if changes, _, err = generate.PreviewChoices(c, f, root.FS(), choices); err != nil {
			return fail(err)
		}
	}

	out = contract.NewGenerate(changes, warnings)
	pending := slices.ContainsFunc(changes, func(ch generate.Change) bool {
		return ch.Status != generate.Unchanged && (!ch.HandWritten || ch.Choice != "")
	})
	switch {
	case !pending:
		return out, Done
	case !opts.Write:
		return out, NotWritten
	}
	written, err := generate.Write(root, changes)
	out.Written = append(out.Written, written...)
	if err != nil {
		return fail(err)
	}
	return out, Done
}

// choiceRequired explains the choices for hand-written files. It names
// the choice both ways, since the CLI and the MCP tools share it.
func choiceRequired(paths []string, topicDir string) string {
	one := len(paths) == 1
	pick := func(singular, plural string) string {
		if one {
			return singular
		}
		return plural
	}
	return fmt.Sprintf("%s %s no cyanotype header, so %s written by hand, and cyanotype never replaces %s without a choice.\n"+
		"Choose one with --existing on the command line, or the existing argument of the MCP tools:\n"+
		"  import   returns a prompt that has your coding agent turn its rules into cyanotype.yaml; nothing is written\n"+
		"  keep     the whole file goes in a cyanotype:custom block, which regeneration keeps\n"+
		"  move     its content goes to %s/original-<name>, which the main file tells the agent to read\n"+
		"  replace  overwrite it with the generated file; only when git has it committed, so it can be recovered",
		andList(paths), pick("has", "have"), pick("it was", "they were"), pick("it", "them"), topicDir)
}

// ImportPrompt returns the prompt that converts the hand-written files at
// paths into the project's cyanotype.yaml, f, which it gives the agent to
// update. The stack it lists entries for is the spec's selections and
// what detection finds.
func ImportPrompt(c *catalog.Catalog, root *os.Root, f *spec.File, paths []string) (string, error) {
	current, err := root.ReadFile(generate.SpecFile)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", generate.SpecFile, err)
	}
	found := detect.Detect(c, root.FS())
	stack := slices.Clone(f.Spec.Selections)
	for _, id := range found.IDs() {
		if !slices.Contains(stack, id) {
			stack = append(stack, id)
		}
	}
	return adopt.Prompt(c, adopt.PromptInput{Files: paths, Spec: current, Found: found, Stack: stack}), nil
}

// ErrNothingToImport is returned by ImportInstructions for a project
// without hand-written instruction files.
var ErrNothingToImport = errors.New("no instruction files written by hand to import")

// ImportInstructions returns the prompt that converts the project's
// hand-written instruction files into cyanotype.yaml, the one init and
// generate --existing import print. With a spec, the files are those
// generate would replace, and the agent updates the spec. Without one,
// they are AGENTS.md and CLAUDE.md, and the agent starts from a draft
// with the detected stack, as init without flags would fill it in.
func ImportInstructions(c *catalog.Catalog, presets []preset.Preset, root *os.Root) (string, error) {
	f, err := generate.LoadSpec(root)
	switch {
	case err == nil:
		changes, _, err := generate.Preview(c, f, root.FS())
		if err != nil {
			return "", err
		}
		hw := generate.HandWritten(changes)
		if len(hw) == 0 {
			return "", ErrNothingToImport
		}
		return ImportPrompt(c, root, f, hw)
	case !errors.Is(err, generate.ErrNoSpec):
		return "", err
	}
	hw := adopt.HandWrittenFiles(root.FS())
	if len(hw) == 0 {
		return "", ErrNothingToImport
	}
	// The folder's name, unless it has none, like the root of a drive.
	name := filepath.Base(root.Name())
	if name == "." || strings.ContainsAny(name, `/\:`) {
		name = ""
	}
	found := detect.Detect(c, root.FS())
	draft, stack, err := adopt.Draft(c, adopt.DraftInput{HandWritten: hw, Name: name, Found: found, Presets: presets})
	if err != nil {
		return "", err
	}
	return adopt.Prompt(c, adopt.PromptInput{Files: hw, Draft: draft, Found: found, Stack: stack}), nil
}

// Check compares the project at root with its spec and reports drift,
// as cyanotype check does. With strict, warnings fail the check too. It
// never writes.
func Check(c *catalog.Catalog, root *os.Root, strict bool) *contract.Check {
	f, err := generate.LoadSpec(root)
	if err != nil {
		return &contract.Check{Diagnostics: failed(err)}
	}
	report, err := check.Run(c, f, root.FS(), root.Name(), check.ExecGit{})
	if err != nil {
		return &contract.Check{Diagnostics: failed(err)}
	}
	return contract.NewCheck(report, strict)
}

// andList joins items as an English list: "a", "a and b", "a, b and c".
func andList(items []string) string {
	if len(items) <= 1 {
		return strings.Join(items, "")
	}
	return strings.Join(items[:len(items)-1], ", ") + " and " + items[len(items)-1]
}
