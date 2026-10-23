package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	bundled "github.com/cyanotype-dev/cyanotype/catalog"
	"github.com/cyanotype-dev/cyanotype/internal/adopt"
	"github.com/cyanotype-dev/cyanotype/internal/catalog"
	"github.com/cyanotype-dev/cyanotype/internal/detect"
	"github.com/cyanotype-dev/cyanotype/internal/generate"
	"github.com/cyanotype-dev/cyanotype/internal/preset"
	"github.com/cyanotype-dev/cyanotype/internal/render"
	"github.com/cyanotype-dev/cyanotype/internal/resolve"
	"github.com/cyanotype-dev/cyanotype/internal/simple"
	"github.com/cyanotype-dev/cyanotype/internal/spec"
)

// initOptions holds the flags of the init command: one per question of
// simple mode, so agents and scripts can run init without a terminal.
type initOptions struct {
	dir         string
	name        string
	goal        string
	projectType string
	language    string
	preset      string
	learning    string
	autonomy    string
	i18n        string
	locales     []string
	replies     string
	agents      []string
	// detect, with and without answer the questions about a stack found
	// in an existing project.
	detect  string
	with    []string
	without []string
	// existing says what to do with instruction files written by hand.
	existing string
}

// Answers to "Use the detected stack?".
const (
	detectYes  = "yes"
	detectNo   = "no"
	detectEdit = "edit" // interactive only: use it without some entries
)

// newInitCmd builds the init command. detect tells it about the terminal
// when it runs.
func newInitCmd(detect func() console) *cobra.Command {
	var opts initOptions
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create cyanotype.yaml by answering a few questions",
		Long: "Init asks what you are building and how you work with coding agents, picks\n" +
			"a starter preset and writes cyanotype.yaml. In an existing project it first\n" +
			"detects the stack from its files and dependencies, shows what it found and\n" +
			"why, and starts from it once you confirm. Every question can be answered\n" +
			"with a flag; without a terminal, init never asks and reports the missing\n" +
			"flags instead. It never overwrites an existing cyanotype.yaml.\n\n" +
			"Exit codes: 0 written, 1 error, 2 wrong usage or missing answers,\n" +
			"3 files not written by the generate step that init offers to run.",
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			r := &initRun{
				opts:    opts,
				changed: cmd.Flags().Changed,
				in:      bufio.NewReader(cmd.InOrStdin()),
				out:     cmd.OutOrStdout(),
				errOut:  cmd.ErrOrStderr(),
				console: detect(),
			}
			return r.run()
		},
	}
	f := cmd.Flags()
	f.StringVar(&opts.dir, "dir", ".", "project folder to create cyanotype.yaml in")
	f.StringVar(&opts.name, "name", "", "project name (default: the folder name)")
	f.StringVar(&opts.goal, "goal", "", "the project in one or two sentences")
	f.StringVar(&opts.projectType, "type", "", "what to build: "+values(simple.ProjectTypeOptions()))
	f.StringVar(&opts.language, "language", "", "a programming language you know: typescript, python, go, other or none")
	f.StringVar(&opts.preset, "preset", "", "starter preset id, such as nextjs-supabase; implies --type")
	f.StringVar(&opts.learning, "learning", "", "learning mode: off, explain, guided or hands-on (default off)")
	f.StringVar(&opts.autonomy, "autonomy", "", "how much the agent does before asking: "+values(simple.AutonomyOptions()))
	f.StringVar(&opts.i18n, "i18n", "", "whether the app supports more than one language: "+values(simple.I18nOptions()))
	f.StringSliceVar(&opts.locales, "locales", nil, "the app's locales, the default one first, such as en,it")
	f.StringVar(&opts.replies, "replies", "", "language of the agent's replies (default english)")
	f.StringSliceVar(&opts.agents, "agents", nil, "coding agents you work with: "+values(simple.AgentOptions()))
	f.StringVar(&opts.detect, "detect", "", "in an existing project: yes to start from the detected stack, no to ignore it")
	f.StringSliceVar(&opts.with, "with", nil, "catalog entries to add to the detected stack, or to choose where detection found several, such as pnpm")
	f.StringSliceVar(&opts.without, "without", nil, "detected catalog entries the project does not use")
	f.StringVar(&opts.existing, "existing", "", existingFlagHelp)
	return cmd
}

// initRun is one run of the init command.
type initRun struct {
	opts initOptions
	// changed reports whether a flag was given, so an empty value can be
	// told apart from a missing one.
	changed     func(name string) bool
	in          *bufio.Reader
	out, errOut io.Writer
	console     console

	cat     *catalog.Catalog
	presets []preset.Preset
	// ask is the prompter, nil when stdin is not a terminal.
	ask *prompter
	// found is what detection found in the project folder.
	found detect.Result
	// handWritten are the main instruction files written by hand, and
	// choices what to do with each, which the generate step applies.
	handWritten []string
	choices     generate.Choices
}

// start is where the spec's stack comes from: a starter preset, or the
// stack detected in an existing project. Exactly one is set once the
// answers are collected.
type start struct {
	preset   *preset.Preset
	detected *simple.Detected
	// projectType is the preset's type, or the detected stack's.
	projectType preset.ProjectType
}

// stack returns the stack the spec is built from.
func (r *initRun) stack(s start) (simple.Stack, error) {
	if s.preset != nil {
		return simple.PresetStack(*s.preset), nil
	}
	return s.detected.Stack(r.cat, r.presets, s.projectType)
}

func (r *initRun) run() error {
	root, err := os.OpenRoot(r.opts.dir)
	if err != nil {
		return fmt.Errorf("open the project folder: %w", err)
	}
	defer root.Close()

	// Refuse before asking anything, so nobody answers questions for nothing.
	if err := generate.CheckNoSpec(root); err != nil {
		return refuseExisting(err)
	}
	if r.cat, err = catalog.LoadRoot(bundled.FS()); err != nil {
		return fmt.Errorf("the bundled catalog is invalid, which is a bug in cyanotype: %w", err)
	}
	if r.presets, err = preset.LoadRoot(bundled.FS(), r.cat); err != nil {
		return fmt.Errorf("the bundled presets are invalid, which is a bug in cyanotype: %w", err)
	}
	if err := r.checkFlags(); err != nil {
		return usageError(err)
	}
	if r.console.interactive {
		r.ask = newPrompter(r.in, r.errOut)
	}

	// Detection only reads; nothing is written before the spec at the end.
	r.found = detect.Detect(r.cat, root.FS())
	for _, p := range r.found.Problems {
		fmt.Fprintf(r.errOut, "%s: %s; detection skipped it\n", paint(r.console.colorErr, yellow, "warning"), p.Error())
	}
	// Instruction files written by hand come first: importing them with an
	// agent makes every other question moot.
	if r.handWritten = handWrittenFiles(root.FS()); len(r.handWritten) > 0 {
		done, err := r.chooseExisting(root)
		if err != nil || done {
			return err
		}
	}
	detected, err := r.useDetected()
	if err != nil {
		return err
	}

	answers, from, err := r.collect(detected)
	if err != nil {
		return err
	}
	st, err := r.stack(from)
	if err != nil {
		return err
	}
	s, err := simple.BuildStack(r.cat, st, answers)
	if err != nil {
		return err
	}
	f := spec.New(s)
	_, warnings, err := resolve.Resolve(r.cat, f)
	switch {
	case err != nil && from.preset != nil:
		return fmt.Errorf("the spec for preset %q does not resolve, which is a bug in cyanotype: %w", from.preset.ID, err)
	case err != nil:
		return fmt.Errorf("the spec for the detected stack does not resolve: %w", err)
	}
	for _, w := range warnings {
		fmt.Fprintf(r.errOut, "%s: %s\n", paint(r.console.colorErr, yellow, "warning"), w)
	}

	r.printSummary(from, st, answers, s)
	if err := generate.CreateSpec(root, f); err != nil {
		if errors.Is(err, generate.ErrSpecExists) {
			return refuseExisting(err)
		}
		return err
	}
	fmt.Fprintf(r.out, "\nWrote %s; edit it to change any of these choices.\n", generate.SpecFile)

	if r.ask == nil {
		fmt.Fprintf(r.out, "Next: run \"%s\" to write the instruction files.\n", r.generateCommand())
		return nil
	}
	fmt.Fprintln(r.errOut)
	now, err := r.ask.yesNo("Generate the instruction files now?", true)
	if err != nil {
		return err
	}
	if !now {
		fmt.Fprintf(r.out, "Run \"%s\" when you are ready.\n", r.generateCommand())
		return nil
	}
	g := &generation{
		opts:     generateOptions{dir: r.opts.dir},
		in:       r.in,
		out:      r.out,
		errOut:   r.errOut,
		console:  r.console,
		dirFlag:  r.changed("dir"),
		specPath: filepath.Join(r.opts.dir, generate.SpecFile),
		choices:  r.choices,
	}
	return g.run()
}

// chooseExisting decides what to do with the instruction files written by
// hand, before any other question. Import prints the prompt, with a draft
// spec from detection, and ends init: done is true and nothing is written.
// Without a terminal, a missing --existing is reported together with a
// missing --detect, so one more run with both flags is enough.
func (r *initRun) chooseExisting(root *os.Root) (done bool, err error) {
	if r.opts.existing == "" && r.ask == nil {
		missing := []missingFlag{{"existing", fmt.Sprintf("what to do with %s, written by hand: import (recommended: prints a prompt for your coding agent and writes nothing), keep, move or replace",
			andList(r.handWritten))}}
		if r.usesDetection() && r.opts.detect == "" {
			missing = append(missing, missingFlag{"detect", "yes to start from the detected stack, no to ignore it and pick a starter"})
		}
		return true, usageError(missingError(missing))
	}
	x := existingChooser{flag: r.opts.existing, ask: r.ask, dir: root.Name(), topicDir: spec.DefaultTopicDir, errOut: r.errOut}
	if r.choices, err = x.choose(r.handWritten); err != nil {
		return true, err
	}
	imports := importPaths(r.choices)
	if len(imports) == 0 {
		return false, nil
	}
	draft, stack, err := r.draft()
	if err != nil {
		return true, err
	}
	found := r.found
	if !r.usesDetection() {
		found = detect.Result{}
	}
	prompt := adopt.Prompt(r.cat, adopt.PromptInput{Files: imports, Draft: draft, Found: found, Stack: stack})
	g := generation{opts: generateOptions{dir: r.opts.dir}, dirFlag: r.changed("dir")}
	printImportPrompt(r.out, r.errOut, prompt, imports, r.importCommand(), g.command("--existing", "replace"))
	return true, nil
}

// importCommand spells out the init command that prints the same import
// prompt, for the user's agent to run: it repeats the flags that shape the
// draft, and --existing import, which a choice made on a terminal needs.
func (r *initRun) importCommand() string {
	var args []string
	for _, f := range []struct {
		name  string
		value string
	}{
		{"dir", r.opts.dir},
		{"name", r.opts.name},
		{"preset", r.opts.preset},
		{"detect", r.opts.detect},
		{"with", strings.Join(r.opts.with, ",")},
		{"without", strings.Join(r.opts.without, ",")},
		{"agents", strings.Join(r.opts.agents, ",")},
	} {
		if r.changed(f.name) {
			args = append(args, "--"+f.name, f.value)
		}
	}
	return commandLine("init", append(args, "--existing", "import")...)
}

// usesDetection reports whether the spec may start from the detected
// stack: something was found, and no flag says to ignore it.
func (r *initRun) usesDetection() bool {
	return !r.found.Empty() && r.opts.preset == "" && r.opts.detect != detectNo
}

// draft returns the spec cyanotype can fill in for the import prompt
// before any question: the targets of --agents, or of the hand-written
// files, the project name, and the detected stack with its commands. It
// also returns the stack, which decides the entries the prompt lists.
func (r *initRun) draft() ([]byte, []string, error) {
	var targets []spec.Target
	if len(r.opts.agents) > 0 {
		targets = simple.Targets(agentsOf(r.opts.agents))
	} else {
		for _, t := range []struct {
			file   string
			target spec.Target
		}{{"CLAUDE.md", spec.TargetClaude}, {"AGENTS.md", spec.TargetAgentsMD}} {
			if slices.Contains(r.handWritten, t.file) {
				targets = append(targets, t.target)
			}
		}
	}
	name := strings.TrimSpace(r.opts.name)
	if name == "" {
		name = folderName(r.opts.dir)
	}
	// The goal is required and only the files can tell it, so the draft
	// holds a placeholder the prompt asks the agent to replace.
	s := spec.Spec{Version: spec.CurrentVersion, CatalogVersion: r.cat.Version(), Targets: targets,
		Project: spec.Project{Name: name, Goal: adopt.GoalPlaceholder}}
	var stack []string
	if r.usesDetection() {
		stack = r.found.IDs()
		if d, err := simple.DetectStack(r.cat, r.presets, r.found, r.opts.with, r.opts.without); err == nil {
			stack, s.Commands = d.Selections, d.Commands
		}
		s.Selections = stack
	}
	var buf bytes.Buffer
	if err := spec.New(s).Save(&buf); err != nil {
		return nil, nil, fmt.Errorf("write the draft spec: %w", err)
	}
	return buf.Bytes(), stack, nil
}

// agentsOf turns --agents values into agents.
func agentsOf(values []string) []simple.Agent {
	agents := make([]simple.Agent, len(values))
	for i, v := range values {
		agents[i] = simple.Agent(v)
	}
	return agents
}

// refuseExisting explains why init stops when the project has a spec.
func refuseExisting(err error) error {
	if errors.Is(err, generate.ErrSpecExists) {
		return fmt.Errorf("%w; init never overwrites it. Edit it, or run cyanotype generate", err)
	}
	return err
}

// generateCommand spells out the generate command for this project,
// with the choice made for files written by hand.
func (r *initRun) generateCommand() string {
	g := generation{opts: generateOptions{dir: r.opts.dir}, dirFlag: r.changed("dir")}
	if len(r.handWritten) > 0 && r.opts.existing != "" {
		return g.command("--existing", r.opts.existing)
	}
	return g.command()
}

// checkFlags rejects flag values that no question accepts, before anything
// is asked. All problems are reported together.
func (r *initRun) checkFlags() error {
	var errs []error
	fail := func(format string, args ...any) { errs = append(errs, fmt.Errorf(format, args...)) }
	oneOf := func(flag, value string, opts []simple.Option) {
		if value == "" && !r.changed(flag) {
			return
		}
		i := indexOf(opts, value)
		switch {
		case i >= 0 && opts[i].Later:
			fail("--%s %s: %s is coming in a later version; choose %s", flag, value, strings.ToLower(opts[i].Label), values(opts))
		case i < 0:
			fail("--%s %q: must be %s", flag, value, values(opts))
		}
	}

	oneOf("type", r.opts.projectType, simple.ProjectTypeOptions())
	oneOf("language", r.opts.language, simple.LanguageOptions(r.cat))
	oneOf("learning", r.opts.learning, simple.LearningOptions(r.cat))
	oneOf("autonomy", r.opts.autonomy, simple.AutonomyOptions())
	oneOf("i18n", r.opts.i18n, simple.I18nOptions())

	if r.changed("preset") {
		p, ok := r.findPreset(r.opts.preset)
		switch {
		case !ok:
			fail("--preset %q: unknown preset; choose %s", r.opts.preset, presetIDs(r.presets))
		case r.opts.projectType != "" && string(p.ProjectType) != r.opts.projectType:
			fail("--preset %s builds a %s, but --type is %s; drop --type or pick another preset", p.ID, p.ProjectType, r.opts.projectType)
		}
	}

	for i, l := range r.opts.locales {
		r.opts.locales[i] = simple.CanonicalLocale(l)
		if !spec.ValidLocale(r.opts.locales[i]) {
			fail("--locales: %q is not a locale tag such as en or pt-BR", l)
		}
	}
	switch {
	case len(r.opts.locales) > 0 && r.opts.i18n == string(simple.I18nNo):
		fail("--locales needs --i18n later or yes")
	case len(r.opts.locales) == 1 && r.opts.i18n == string(simple.I18nYes):
		fail("--i18n yes needs at least two --locales, the default one first, such as en,it")
	}

	if r.changed("replies") {
		r.opts.replies = simple.CanonicalLanguage(r.opts.replies)
		if !spec.ValidLanguageName(r.opts.replies) {
			fail("--replies %q: must be a language name such as english or italian", r.opts.replies)
		}
	}
	if r.changed("goal") && strings.TrimSpace(r.opts.goal) == "" {
		fail("--goal is empty")
	}
	if r.changed("name") && (strings.TrimSpace(r.opts.name) == "" || strings.Contains(r.opts.name, "\n")) {
		fail("--name must be one line of text")
	}
	if r.changed("agents") && len(r.opts.agents) == 0 {
		fail("--agents is empty; name at least one agent")
	}
	for _, a := range r.opts.agents {
		oneOf("agents", a, simple.AgentOptions())
	}

	oneOf("detect", r.opts.detect, detectOptions())
	if err := checkExistingFlag(r.opts.existing, r.changed("existing")); err != nil {
		errs = append(errs, err)
	}
	adjusts := r.changed("with") || r.changed("without")
	switch {
	case r.changed("preset") && r.opts.detect == detectYes:
		fail("--preset starts from a starter instead of the detected stack; drop --detect yes or --preset")
	case r.changed("preset") && adjusts:
		fail("--with and --without change the detected stack, which --preset does not use; drop them or --preset")
	case r.opts.detect == detectNo && adjusts:
		fail("--with and --without change the detected stack, which --detect no ignores; drop them or --detect no")
	}
	return errors.Join(errs...)
}

// detectOptions returns the answers --detect accepts.
func detectOptions() []simple.Option {
	return []simple.Option{
		{Value: detectYes, Label: "Yes", Help: "start from the detected stack"},
		{Value: detectNo, Label: "No", Help: "ignore it and pick a starter"},
	}
}

// useDetected decides whether the spec starts from the stack detection
// found, and returns it adjusted by --with and --without, or nil to start
// from a preset. It shows what detection found and why before asking, and
// without a terminal it needs --detect: starting from the files of a
// project is a choice the user makes, never one cyanotype makes alone.
func (r *initRun) useDetected() (*simple.Detected, error) {
	if r.found.Empty() || r.opts.preset != "" || r.opts.detect == detectNo {
		return nil, nil
	}
	without := slices.Clone(r.opts.without)
	d, err := simple.DetectStack(r.cat, r.presets, r.found, r.opts.with, without)
	if err != nil {
		return nil, usageError(err)
	}
	r.printDetected()

	choice := r.opts.detect
	switch {
	case choice != "":
	case r.ask == nil:
		return nil, usageError(missingError([]missingFlag{{"detect", "yes to start from the stack detected above, no to ignore it and pick a starter"}}))
	default:
		o, err := r.ask.choose("Start from the detected stack?", []simple.Option{
			{Value: detectYes, Label: "Yes"},
			{Value: detectEdit, Label: "Yes, without some of it", Help: "for a file that revealed something the project does not use"},
			{Value: detectNo, Label: "No", Help: "ignore it and pick a starter"},
		}, detectYes)
		if err != nil {
			return nil, err
		}
		choice = o.Value
	}

	switch choice {
	case detectNo:
		return nil, nil
	case detectEdit:
		var opts []simple.Option
		for _, f := range r.found.Findings {
			if !slices.Contains(without, f.ID) && !slices.Contains(r.opts.with, f.ID) {
				e, _ := r.cat.Get(f.ID)
				opts = append(opts, simple.Option{Value: f.ID, Label: e.Name, Help: evidenceList(f.Evidence)})
			}
		}
		drop, err := r.ask.chooseMany("Which of these does the project not use?", opts)
		if err != nil {
			return nil, err
		}
		for _, o := range drop {
			without = append(without, o.Value)
		}
		if d, err = simple.DetectStack(r.cat, r.presets, r.found, r.opts.with, without); err != nil {
			return nil, err
		}
	}
	r.opts.without = without
	if len(d.Selections) == 0 {
		fmt.Fprintln(r.errOut, "Nothing is left of the detected stack, so init starts from a starter instead.")
		return nil, nil
	}
	return &d, nil
}

// printDetected shows what detection found, and why, on stdout.
func (r *initRun) printDetected() {
	printDetected(r.out, r.console.colorOut, r.opts.dir, r.cat, r.found)
}

// printDetected shows what detection found in the project folder dir, and
// why: the block init shows before asking, and cyanotype detect prints.
func printDetected(w io.Writer, color bool, dir string, c *catalog.Catalog, found detect.Result) {
	heading := func(s string) string { return paint(color, bold, s) }
	where := "this project"
	if dir != "." {
		where = dir
	}
	fmt.Fprintf(w, "%s\n", heading("Detected in "+where+":"))
	for _, f := range found.Findings {
		e, _ := c.Get(f.ID)
		fmt.Fprintf(w, "  - %s (%s)\n", e.Name, evidenceList(f.Evidence))
	}
	for _, cl := range found.Clashes {
		fmt.Fprintf(w, "Only one of %s can be selected: the project uses one of them.\n", entryNames(c, cl.IDs, "and"))
	}
	if len(found.Scripts) > 0 {
		fmt.Fprintf(w, "Scripts in package.json: %s\n", strings.Join(found.Scripts, ", "))
	}
}

// evidenceList joins the reasons an entry was detected: "package.json: next, next.config.ts".
func evidenceList(ev []detect.Evidence) string {
	parts := make([]string, len(ev))
	for i, e := range ev {
		parts[i] = e.String()
	}
	return strings.Join(parts, ", ")
}

// entryNames names entries as an English list: "npm or pnpm".
func (r *initRun) entryNames(ids []string, conj string) string {
	return entryNames(r.cat, ids, conj)
}

// entryNames names the entries of c with the given ids as an English list.
func entryNames(c *catalog.Catalog, ids []string, conj string) string {
	names := make([]string, len(ids))
	for i, id := range ids {
		e, _ := c.Get(id)
		names[i] = e.Name
	}
	return orList(names, conj)
}

// choiceQuestion says why a detected stack needs the user to choose.
func (r *initRun) choiceQuestion(ch simple.Choice) string {
	switch {
	case ch.Kind == simple.ChoiceGroup:
		by, _ := r.cat.Get(ch.By)
		return fmt.Sprintf("%s needs %s, and detection found neither. Which one does the project use?",
			by.Name, r.entryNames(ch.IDs, "or"))
	case ch.Category != "":
		return fmt.Sprintf("Detection found %s, but a project uses only one %s. Which one does it use?",
			r.entryNames(ch.IDs, "and"), ch.Category)
	default:
		return fmt.Sprintf("Detection found %s, which conflict with each other. Which one does the project use?",
			r.entryNames(ch.IDs, "and"))
	}
}

// choiceOptions offers the entries of a choice, each with why it was
// detected.
func (r *initRun) choiceOptions(ch simple.Choice) []simple.Option {
	opts := make([]simple.Option, len(ch.IDs))
	for i, id := range ch.IDs {
		e, _ := r.cat.Get(id)
		help := "not detected"
		if f, ok := r.found.Find(id); ok {
			help = "found " + evidenceList(f.Evidence)
		}
		opts[i] = simple.Option{Value: id, Label: e.Name, Help: help}
	}
	return opts
}

// choiceHelp explains a choice in the list of missing flags.
func (r *initRun) choiceHelp(ch simple.Choice) string {
	var reason string
	if ch.Kind == simple.ChoiceGroup {
		by, _ := r.cat.Get(ch.By)
		reason = by.Name + " needs one, and detection found none"
	} else {
		var found []string
		for _, o := range r.choiceOptions(ch) {
			found = append(found, o.Value+" "+o.Help)
		}
		reason = strings.Join(found, ", ") + "; only one can be selected"
	}
	return orList(ch.IDs, "or") + ": the one the project uses (" + reason + ")"
}

// missingFlag is a question nobody answered, named by its flag.
type missingFlag struct {
	flag, help string
}

// collect gathers the answers, from flags first, then by asking on the
// terminal. Without a terminal it never reads stdin: every missing answer
// is collected into one usage error naming its flag.
//
// With a detected stack, the questions detection answers are skipped: the
// starter always, and what to build when the closest presets agree. The
// choices the stack leaves open, such as two lockfiles, come first.
func (r *initRun) collect(d *simple.Detected) (simple.Answers, start, error) {
	var (
		a       simple.Answers
		missing []missingFlag
		chosen  *preset.Preset
		pt      preset.ProjectType
	)
	miss := func(flag, help string) { missing = append(missing, missingFlag{flag, help}) }
	fail := func(err error) (simple.Answers, start, error) { return a, start{}, err }

	// 0. The choices a detected stack leaves open, each answered by adding
	// the chosen entry as --with would.
	if d != nil {
		with := slices.Clone(r.opts.with)
		for len(d.Choices) > 0 {
			if r.ask == nil {
				for _, ch := range d.Choices {
					miss("with", r.choiceHelp(ch))
				}
				break
			}
			ch := d.Choices[0]
			o, err := r.ask.choose(r.choiceQuestion(ch), r.choiceOptions(ch), ch.Default)
			if err != nil {
				return fail(err)
			}
			with = append(with, o.Value)
			next, err := simple.DetectStack(r.cat, r.presets, r.found, with, r.opts.without)
			if err != nil {
				return fail(err)
			}
			d = &next
		}
	}

	// 1. What to build, which --preset implies, and a detected stack too
	// when the presets closest to it build one kind of project.
	switch {
	case r.opts.preset != "":
		p, _ := r.findPreset(r.opts.preset) // checked by checkFlags
		chosen, pt = &p, p.ProjectType
	case r.opts.projectType != "":
		pt = preset.ProjectType(r.opts.projectType)
	case d != nil && d.ProjectType != "":
		pt = d.ProjectType
	case r.ask != nil:
		o, err := r.ask.choose("What do you want to build?", simple.ProjectTypeOptions(), "")
		if err != nil {
			return fail(err)
		}
		pt = preset.ProjectType(o.Value)
	default:
		miss("type", "what to build: "+values(simple.ProjectTypeOptions()))
	}

	// 2. The language the user knows, which picks among presets and
	// decides whether learning mode is offered.
	language := r.opts.language
	if language == "" && r.ask != nil {
		o, err := r.ask.choose("Do you already know a programming language?", simple.LanguageOptions(r.cat), "")
		if err != nil {
			return fail(err)
		}
		language = o.Value
	}

	// The starter preset, asked only when several fit; a detected stack
	// needs none.
	if chosen == nil && pt != "" && d == nil {
		candidates := simple.Candidates(r.cat, r.presets, pt, language)
		switch {
		case len(candidates) == 0:
			return fail(fmt.Errorf("no preset builds a %s, which is a bug in cyanotype", pt))
		case len(candidates) == 1:
			chosen = &candidates[0]
		case r.ask != nil:
			o, err := r.ask.choose("Which starter fits best?", presetOptions(candidates), candidates[0].ID)
			if err != nil {
				return fail(err)
			}
			p, _ := r.findPreset(o.Value)
			chosen = &p
		default:
			miss("preset", "the starter, since several fit: "+presetIDs(candidates))
		}
	}

	// Learning mode: offered only to users who do not know the stack's
	// languages, and never on by default.
	var stackIDs []string
	switch {
	case chosen != nil:
		stackIDs = chosen.Selections
	case d != nil:
		stackIDs = d.Selections
	}
	langs := simple.Languages(r.cat, stackIDs)
	a.Learning = r.opts.learning
	if a.Learning == "" && r.ask != nil && len(langs) > 0 && simple.OffersLearningFor(r.cat, stackIDs, language) {
		question := fmt.Sprintf("Turn on learning mode, to learn %s while the agent works?", r.entryNames(langs, "and"))
		o, err := r.ask.choose(question, simple.LearningOptions(r.cat), simple.LearningOff)
		if err != nil {
			return fail(err)
		}
		a.Learning = o.Value
	}

	// 3. Autonomy.
	a.Autonomy = simple.Autonomy(r.opts.autonomy)
	if a.Autonomy == "" {
		if r.ask == nil {
			miss("autonomy", "how much the agent does before asking: "+values(simple.AutonomyOptions()))
		} else {
			o, err := r.ask.choose("How much autonomy should the agent have?", simple.AutonomyOptions(), string(simple.Balanced))
			if err != nil {
				return fail(err)
			}
			a.Autonomy = simple.Autonomy(o.Value)
		}
	}

	// 4. Translations, for apps with a user interface only.
	if err := r.collectI18n(&a, pt, d != nil && d.Multilingual, miss); err != nil {
		return fail(err)
	}

	// 5. The language of the agent's replies; English unless asked otherwise.
	a.AgentReplies = r.opts.replies
	if a.AgentReplies == "" && r.ask != nil {
		answer, err := r.ask.text("Language for the agent's replies", spec.DefaultLanguage, checkLanguage)
		if err != nil {
			return fail(err)
		}
		a.AgentReplies = simple.CanonicalLanguage(answer)
	}

	// 6. The goal.
	a.Goal = strings.TrimSpace(r.opts.goal)
	if a.Goal == "" {
		if r.ask == nil {
			miss("goal", "the project in one or two sentences")
		} else {
			answer, err := r.ask.text("Describe the project in one or two sentences", "", nil)
			if err != nil {
				return fail(err)
			}
			a.Goal = answer
		}
	}

	// 7. The agents, which decide the targets.
	for _, v := range r.opts.agents {
		a.Agents = append(a.Agents, simple.Agent(v))
	}
	if len(a.Agents) == 0 {
		if r.ask == nil {
			miss("agents", "the coding agents you work with, separated by commas: "+values(simple.AgentOptions()))
		} else {
			opts, err := r.ask.chooseMany("Which coding agents do you work with?", simple.AgentOptions())
			if err != nil {
				return fail(err)
			}
			for _, o := range opts {
				a.Agents = append(a.Agents, simple.Agent(o.Value))
			}
		}
	}

	// The name is not asked: the folder name is almost always right.
	a.Name = strings.TrimSpace(r.opts.name)
	if a.Name == "" {
		if a.Name = folderName(r.opts.dir); a.Name == "" {
			miss("name", "the project name; the folder has no name to use")
		}
	}

	if len(missing) > 0 {
		return fail(usageError(missingError(missing)))
	}
	return a, start{preset: chosen, detected: d, projectType: pt}, nil
}

// collectI18n answers the translation question when it applies to pt.
// translated reports that the detected stack has a translation library, so
// the app is translated already and only its locales are left to ask.
func (r *initRun) collectI18n(a *simple.Answers, pt preset.ProjectType, translated bool, miss func(flag, help string)) error {
	if pt == "" {
		return nil // what to build is itself missing
	}
	if !simple.AsksI18n(pt) {
		if r.opts.i18n != "" || len(r.opts.locales) > 0 {
			return usageError(fmt.Errorf("--i18n and --locales apply only to websites and mobile apps, not to a %s", pt))
		}
		return nil
	}

	if translated && r.opts.i18n == "" {
		a.Locales = r.opts.locales
		if len(a.Locales) == 0 {
			if r.ask == nil {
				miss("locales", "the app's locales, the default one first, such as en,it; the detected stack has a translation library")
				return nil
			}
			answer, err := r.ask.text("The app uses a translation library. Which locales does it support? Locale tags separated by commas, the default one first",
				simple.DefaultLocale, checkSomeLocales)
			if err != nil {
				return err
			}
			a.Locales = splitLocales(answer)
		}
		// One locale is an app ready for more, several a multilingual one.
		a.I18n = simple.I18nLater
		if len(a.Locales) > 1 {
			a.I18n = simple.I18nYes
		}
		return nil
	}

	a.I18n = simple.I18n(r.opts.i18n)
	if a.I18n == "" {
		if r.ask == nil {
			miss("i18n", "whether the app supports more than one language: "+values(simple.I18nOptions()))
			return nil
		}
		o, err := r.ask.choose("Should the app support more than one language?", simple.I18nOptions(), "")
		if err != nil {
			return err
		}
		a.I18n = simple.I18n(o.Value)
	}

	a.Locales = r.opts.locales
	switch {
	case len(a.Locales) > 0 || a.I18n == simple.I18nNo:
	case a.I18n == simple.I18nYes && r.ask == nil:
		miss("locales", "the app's locales, the default one first, such as en,it")
	case a.I18n == simple.I18nYes:
		answer, err := r.ask.text("Which languages? Locale tags separated by commas, the default one first, such as en,it", "", checkLocales)
		if err != nil {
			return err
		}
		a.Locales = splitLocales(answer)
	case r.ask != nil: // later: one locale now
		answer, err := r.ask.text("Locale of the app's first language", simple.DefaultLocale, checkLocale)
		if err != nil {
			return err
		}
		a.Locales = []string{simple.CanonicalLocale(answer)}
	}
	return nil
}

// missingError lists the answers a non-interactive run lacks.
func missingError(missing []missingFlag) error {
	width := 0
	for _, m := range missing {
		width = max(width, len(m.flag))
	}
	var b strings.Builder
	b.WriteString("missing answers, and stdin is not a terminal, so cyanotype cannot ask for them:\n")
	for _, m := range missing {
		fmt.Fprintf(&b, "  --%-*s  %s\n", width, m.flag, m.help)
	}
	b.WriteString("Run cyanotype init again with these flags.")
	return errors.New(b.String())
}

// printSummary shows the chosen preset with a short explanation of every
// entry, grouped by where it comes from, and the files the targets produce.
func (r *initRun) printSummary(from start, st simple.Stack, a simple.Answers, s spec.Spec) {
	type group struct {
		title string
		ids   []string
	}
	// Each group lists the entries it adds to the spec; an entry the
	// stack already has, such as conventional-commits detected from a
	// commitlint config, is listed once, in the stack.
	listed := slices.Clone(st.Selections)
	adds := func(ids []string) []string {
		var out []string
		for _, id := range ids {
			if !slices.Contains(listed, id) {
				out = append(out, id)
				listed = append(listed, id)
			}
		}
		return out
	}
	groups := []group{{"Stack", st.Selections}}
	if s.I18n != nil {
		title := fmt.Sprintf("Translations (default locale %s)", s.I18n.DefaultLocale)
		if len(s.I18n.Locales) > 0 {
			title = fmt.Sprintf("Translations (%s; %s is the default)", strings.Join(s.I18n.Locales, ", "), s.I18n.DefaultLocale)
		}
		groups = append(groups, group{title, adds(append([]string{preset.GenericI18n}, st.I18nSelections...))})
	}
	groups = append(groups, group{
		fmt.Sprintf("Agent rules (autonomy: %s)", strings.ToLower(label(simple.AutonomyOptions(), string(a.Autonomy)))),
		adds(simple.AutonomySelections(a.Autonomy)),
	})
	if a.Learning != "" && a.Learning != simple.LearningOff {
		groups = append(groups, group{"Learning mode", []string{"learning-" + a.Learning}})
	}

	heading := func(s string) string { return paint(r.console.colorOut, bold, s) }
	if p := from.preset; p != nil {
		fmt.Fprintf(r.out, "\n%s %s\n%s.\n", heading("Starter:"), p.Name, p.Summary)
	} else {
		fmt.Fprintf(r.out, "\n%s the stack detected in the project\n%s %s\n", heading("Starting from:"),
			heading("Project type:"), label(simple.ProjectTypeOptions(), string(from.projectType)))
	}
	for _, g := range groups {
		if len(g.ids) == 0 {
			continue
		}
		fmt.Fprintf(r.out, "\n%s\n", heading(g.title))
		for _, id := range g.ids {
			e, _ := r.cat.Get(id)
			fmt.Fprintf(r.out, "  - %s: %s%s\n", e.Name, e.Summary, r.origin(from, id))
		}
	}
	if from.detected != nil {
		r.printDetectedCommands(*from.detected)
	}

	// The name is never asked, so say where it comes from and how to change
	// it: a folder named "app" or "tmp" is rarely the name people want.
	fmt.Fprintf(r.out, "\n%s %s", heading("Project name:"), s.Project.Name)
	if strings.TrimSpace(r.opts.name) == "" {
		fmt.Fprint(r.out, " (the folder name; to change it, pass --name or edit project.name in cyanotype.yaml)")
	}
	fmt.Fprintf(r.out, "\n%s %s\n", heading("Goal:"), s.Project.Goal)
	if s.Languages.AgentReplies != "" {
		fmt.Fprintf(r.out, "%s %s\n", heading("Agent replies:"), s.Languages.AgentReplies)
	}
	var agents, files, pending []string
	for _, ag := range a.Agents {
		agents = append(agents, label(simple.AgentOptions(), string(ag)))
	}
	for _, t := range s.Targets {
		if render.Supported(t) {
			files = append(files, targetFiles[t])
		} else {
			pending = append(pending, string(t))
		}
	}
	fmt.Fprintf(r.out, "%s %s\n", heading("Agents:"), strings.Join(agents, ", "))
	fmt.Fprintf(r.out, "%s %s\n", heading("Instruction files:"), strings.Join(files, ", "))
	if len(pending) > 0 {
		fmt.Fprintf(r.out, "Cyanotype cannot generate files for %s yet; they stay in targets for a later version, and AGENTS.md covers them meanwhile.\n",
			strings.Join(pending, ", "))
	}
}

// origin says where an entry of a detected stack comes from, as a suffix
// for the summary: what revealed it, the entry that requires it, or the
// user's choice. Entries of a preset need no explanation.
func (r *initRun) origin(from start, id string) string {
	d := from.detected
	if d == nil || !slices.Contains(d.Selections, id) {
		return ""
	}
	if f, ok := r.found.Find(id); ok && !slices.Contains(r.opts.without, id) {
		return " (found " + evidenceList(f.Evidence) + ")"
	}
	for _, req := range d.Added {
		if req.ID == id {
			by, _ := r.cat.Get(req.By)
			return " (required by " + by.Name + ")"
		}
	}
	return " (your choice)"
}

// printDetectedCommands shows the commands taken from package.json, or
// says there are none, since a spec without commands leaves the agent
// guessing how to build and test.
func (r *initRun) printDetectedCommands(d simple.Detected) {
	heading := func(s string) string { return paint(r.console.colorOut, bold, s) }
	if len(d.CommandScripts) == 0 {
		fmt.Fprintf(r.out, "\n%s none detected; add them under commands in %s.\n", heading("Commands:"), generate.SpecFile)
		return
	}
	fmt.Fprintf(r.out, "\n%s\n", heading("Commands (from package.json scripts)"))
	cmds := map[string]string{"dev": d.Commands.Dev, "build": d.Commands.Build, "test": d.Commands.Test, "lint": d.Commands.Lint}
	for _, name := range d.CommandScripts {
		fmt.Fprintf(r.out, "  - %s: %s\n", name, cmds[name])
	}
}

// targetFiles names the main file each supported target produces.
var targetFiles = map[spec.Target]string{
	spec.TargetClaude:   "CLAUDE.md",
	spec.TargetAgentsMD: "AGENTS.md",
}

// findPreset returns the bundled preset with the given id.
func (r *initRun) findPreset(id string) (preset.Preset, bool) {
	i := slices.IndexFunc(r.presets, func(p preset.Preset) bool { return p.ID == id })
	if i < 0 {
		return preset.Preset{}, false
	}
	return r.presets[i], true
}

// presetOptions turns presets into the options of the starter question.
func presetOptions(presets []preset.Preset) []simple.Option {
	opts := make([]simple.Option, len(presets))
	for i, p := range presets {
		opts[i] = simple.Option{Value: p.ID, Label: p.Name, Help: p.Summary}
	}
	return opts
}

// presetIDs lists preset ids for messages: "a, b or c".
func presetIDs(presets []preset.Preset) string {
	ids := make([]string, len(presets))
	for i, p := range presets {
		ids[i] = p.ID
	}
	return orList(ids, "or")
}

// values lists the values that can be chosen among opts: "a, b or c".
func values(opts []simple.Option) string {
	var vs []string
	for _, o := range opts {
		if !o.Later {
			vs = append(vs, o.Value)
		}
	}
	return orList(vs, "or")
}

// label returns the label of the option with the given value, or the value
// itself when no option has it.
func label(opts []simple.Option, value string) string {
	if i := indexOf(opts, value); i >= 0 {
		return opts[i].Label
	}
	return value
}

// orList joins items as an English list: "a", "a or b", "a, b or c".
func orList(items []string, conj string) string {
	if len(items) <= 1 {
		return strings.Join(items, "")
	}
	return strings.Join(items[:len(items)-1], ", ") + " " + conj + " " + items[len(items)-1]
}

// folderName returns the name of the project folder, or "" when it has
// none, such as the root of a drive.
func folderName(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return ""
	}
	name := filepath.Base(abs)
	if name == "." || strings.ContainsAny(name, `/\:`) {
		return ""
	}
	return name
}

// checkLanguage rejects a reply language spec would refuse.
func checkLanguage(answer string) error {
	if !spec.ValidLanguageName(simple.CanonicalLanguage(answer)) {
		return errors.New("name a language in letters, such as english or italian")
	}
	return nil
}

// checkLocale rejects an answer that is not one locale tag.
func checkLocale(answer string) error {
	if !spec.ValidLocale(simple.CanonicalLocale(answer)) {
		return errors.New("enter a locale tag such as en or pt-BR")
	}
	return nil
}

// checkLocales rejects an answer that is not at least two locale tags.
func checkLocales(answer string) error {
	locales := splitLocales(answer)
	for _, l := range locales {
		if !spec.ValidLocale(l) {
			return fmt.Errorf("enter locale tags such as en or pt-BR; %q is not one", l)
		}
	}
	if len(locales) < 2 {
		return errors.New("enter at least two locales, the default one first, such as en,it")
	}
	return nil
}

// checkSomeLocales rejects an answer that is not one or more locale tags.
func checkSomeLocales(answer string) error {
	locales := splitLocales(answer)
	if len(locales) == 0 {
		return errors.New("enter at least one locale tag, such as en")
	}
	for _, l := range locales {
		if !spec.ValidLocale(l) {
			return fmt.Errorf("enter locale tags such as en or pt-BR; %q is not one", l)
		}
	}
	return nil
}

// splitLocales splits an answer such as "en, it pt_br" into canonical tags.
func splitLocales(answer string) []string {
	var out []string
	for _, f := range strings.FieldsFunc(answer, func(r rune) bool { return r == ',' || r == ' ' }) {
		out = append(out, simple.CanonicalLocale(f))
	}
	return out
}
