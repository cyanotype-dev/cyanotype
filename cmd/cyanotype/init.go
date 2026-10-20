package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	bundled "github.com/cyanotype-dev/cyanotype/catalog"
	"github.com/cyanotype-dev/cyanotype/internal/catalog"
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
}

// newInitCmd builds the init command. detect tells it about the terminal
// when it runs.
func newInitCmd(detect func() console) *cobra.Command {
	var opts initOptions
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create cyanotype.yaml by answering a few questions",
		Long: "Init asks what you are building and how you work with coding agents, picks\n" +
			"a starter preset and writes cyanotype.yaml. Every question can be answered\n" +
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

	answers, chosen, err := r.collect()
	if err != nil {
		return err
	}
	s, err := simple.Build(r.cat, chosen, answers)
	if err != nil {
		return err
	}
	f := spec.New(s)
	_, warnings, err := resolve.Resolve(r.cat, f)
	if err != nil {
		return fmt.Errorf("the spec for preset %q does not resolve, which is a bug in cyanotype: %w", chosen.ID, err)
	}
	for _, w := range warnings {
		fmt.Fprintf(r.errOut, "%s: %s\n", paint(r.console.colorErr, yellow, "warning"), w)
	}

	r.printSummary(chosen, answers, s)
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
	}
	return g.run()
}

// refuseExisting explains why init stops when the project has a spec.
func refuseExisting(err error) error {
	if errors.Is(err, generate.ErrSpecExists) {
		return fmt.Errorf("%w; init never overwrites it. Edit it, or run cyanotype generate", err)
	}
	return err
}

// generateCommand spells out the generate command for this project.
func (r *initRun) generateCommand() string {
	g := generation{opts: generateOptions{dir: r.opts.dir}, dirFlag: r.changed("dir")}
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
	return errors.Join(errs...)
}

// missingFlag is a question nobody answered, named by its flag.
type missingFlag struct {
	flag, help string
}

// collect gathers the answers, from flags first, then by asking on the
// terminal. Without a terminal it never reads stdin: every missing answer
// is collected into one usage error naming its flag.
func (r *initRun) collect() (simple.Answers, preset.Preset, error) {
	var (
		a       simple.Answers
		missing []missingFlag
		chosen  *preset.Preset
		pt      preset.ProjectType
	)
	miss := func(flag, help string) { missing = append(missing, missingFlag{flag, help}) }

	// 1. What to build, which --preset implies.
	switch {
	case r.opts.preset != "":
		p, _ := r.findPreset(r.opts.preset) // checked by checkFlags
		chosen, pt = &p, p.ProjectType
	case r.opts.projectType != "":
		pt = preset.ProjectType(r.opts.projectType)
	case r.ask != nil:
		o, err := r.ask.choose("What do you want to build?", simple.ProjectTypeOptions(), "")
		if err != nil {
			return a, preset.Preset{}, err
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
			return a, preset.Preset{}, err
		}
		language = o.Value
	}

	// The starter preset, asked only when several fit.
	if chosen == nil && pt != "" {
		candidates := simple.Candidates(r.cat, r.presets, pt, language)
		switch {
		case len(candidates) == 0:
			return a, preset.Preset{}, fmt.Errorf("no preset builds a %s, which is a bug in cyanotype", pt)
		case len(candidates) == 1:
			chosen = &candidates[0]
		case r.ask != nil:
			o, err := r.ask.choose("Which starter fits best?", presetOptions(candidates), candidates[0].ID)
			if err != nil {
				return a, preset.Preset{}, err
			}
			p, _ := r.findPreset(o.Value)
			chosen = &p
		default:
			miss("preset", "the starter, since several fit: "+presetIDs(candidates))
		}
	}

	// Learning mode: offered only to users who do not know the preset's
	// languages, and never on by default.
	a.Learning = r.opts.learning
	if a.Learning == "" && r.ask != nil && chosen != nil && simple.OffersLearning(r.cat, *chosen, language) {
		question := fmt.Sprintf("Turn on learning mode, to learn %s while the agent works?", r.languageNames(*chosen))
		o, err := r.ask.choose(question, simple.LearningOptions(r.cat), simple.LearningOff)
		if err != nil {
			return a, preset.Preset{}, err
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
				return a, preset.Preset{}, err
			}
			a.Autonomy = simple.Autonomy(o.Value)
		}
	}

	// 4. Translations, for apps with a user interface only.
	if err := r.collectI18n(&a, pt, miss); err != nil {
		return a, preset.Preset{}, err
	}

	// 5. The language of the agent's replies; English unless asked otherwise.
	a.AgentReplies = r.opts.replies
	if a.AgentReplies == "" && r.ask != nil {
		answer, err := r.ask.text("Language for the agent's replies", spec.DefaultLanguage, checkLanguage)
		if err != nil {
			return a, preset.Preset{}, err
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
				return a, preset.Preset{}, err
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
				return a, preset.Preset{}, err
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
		return a, preset.Preset{}, usageError(missingError(missing))
	}
	return a, *chosen, nil
}

// collectI18n answers the translation question when it applies to pt.
func (r *initRun) collectI18n(a *simple.Answers, pt preset.ProjectType, miss func(flag, help string)) error {
	if pt == "" {
		return nil // what to build is itself missing
	}
	if !simple.AsksI18n(pt) {
		if r.opts.i18n != "" || len(r.opts.locales) > 0 {
			return usageError(fmt.Errorf("--i18n and --locales apply only to websites and mobile apps, not to a %s", pt))
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
func (r *initRun) printSummary(p preset.Preset, a simple.Answers, s spec.Spec) {
	type group struct {
		title string
		ids   []string
	}
	groups := []group{
		{"Stack", p.Selections},
		{fmt.Sprintf("Agent rules (autonomy: %s)", strings.ToLower(label(simple.AutonomyOptions(), string(a.Autonomy)))), simple.AutonomySelections(a.Autonomy)},
	}
	if s.I18n != nil {
		title := fmt.Sprintf("Translations (default locale %s)", s.I18n.DefaultLocale)
		if len(s.I18n.Locales) > 0 {
			title = fmt.Sprintf("Translations (%s; %s is the default)", strings.Join(s.I18n.Locales, ", "), s.I18n.DefaultLocale)
		}
		groups = slices.Insert(groups, 1, group{title, p.MultilingualSelections()})
	}
	if a.Learning != "" && a.Learning != simple.LearningOff {
		groups = append(groups, group{"Learning mode", []string{"learning-" + a.Learning}})
	}

	heading := func(s string) string { return paint(r.console.colorOut, bold, s) }
	fmt.Fprintf(r.out, "\n%s %s\n%s.\n", heading("Starter:"), p.Name, p.Summary)
	for _, g := range groups {
		fmt.Fprintf(r.out, "\n%s\n", heading(g.title))
		for _, id := range g.ids {
			e, _ := r.cat.Get(id)
			fmt.Fprintf(r.out, "  - %s: %s\n", e.Name, e.Summary)
		}
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

// languageNames names the languages of a preset as an English list.
func (r *initRun) languageNames(p preset.Preset) string {
	var names []string
	for _, id := range p.Languages(r.cat) {
		e, _ := r.cat.Get(id)
		names = append(names, e.Name)
	}
	return orList(names, "and")
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

// splitLocales splits an answer such as "en, it pt_br" into canonical tags.
func splitLocales(answer string) []string {
	var out []string
	for _, f := range strings.FieldsFunc(answer, func(r rune) bool { return r == ',' || r == ' ' }) {
		out = append(out, simple.CanonicalLocale(f))
	}
	return out
}
