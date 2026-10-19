package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/cyanotype-dev/cyanotype/internal/simple"
)

// errInputEnded reports that stdin ended before a question was answered.
var errInputEnded = errors.New("input ended before every question was answered; nothing was written")

// prompter asks questions on a terminal with plain numbered choices and
// lines of text. It writes to stderr, as the confirmation of generate does,
// so stdout keeps only results. Only commands that know stdin is a terminal
// may use it: on a pipe it would wait for answers nobody gives.
type prompter struct {
	in  *bufio.Reader
	out io.Writer
}

// newPrompter returns a prompter reading answers from in. The reader is
// shared with anything that reads later, such as generate's confirmation,
// so no buffered answer is lost.
func newPrompter(in *bufio.Reader, out io.Writer) *prompter {
	return &prompter{in: in, out: out}
}

// readLine returns the next line of input without its line ending.
func (p *prompter) readLine() (string, error) {
	line, err := p.in.ReadString('\n')
	if err != nil && line == "" {
		fmt.Fprintln(p.out)
		return "", errInputEnded
	}
	return strings.TrimSpace(line), nil
}

// choose asks question and returns the chosen option. Options marked Later
// are shown without a number and cannot be chosen. An option can be chosen
// by number or by value; an empty answer picks def, a Value, unless def is
// empty. A wrong answer is explained and the question asked again.
func (p *prompter) choose(question string, opts []simple.Option, def string) (simple.Option, error) {
	numbered := p.list(question, opts)
	hint := fmt.Sprintf("Choose 1-%d", len(numbered))
	if def != "" {
		hint += fmt.Sprintf(" [%d]", indexOf(numbered, def)+1)
	}
	for {
		fmt.Fprint(p.out, hint+": ")
		answer, err := p.readLine()
		if err != nil {
			return simple.Option{}, err
		}
		if answer == "" && def != "" {
			answer = def
		}
		if o, ok := pick(numbered, answer); ok {
			return o, nil
		}
		fmt.Fprintf(p.out, "Please answer with a number from 1 to %d.\n", len(numbered))
	}
}

// chooseMany asks question and returns one or more options, given as
// numbers or values separated by commas or spaces.
func (p *prompter) chooseMany(question string, opts []simple.Option) ([]simple.Option, error) {
	numbered := p.list(question, opts)
	for {
		fmt.Fprint(p.out, "Choose one or more, separated by commas: ")
		answer, err := p.readLine()
		if err != nil {
			return nil, err
		}
		var chosen []simple.Option
		ok := answer != ""
		for _, a := range strings.FieldsFunc(answer, func(r rune) bool { return r == ',' || r == ' ' }) {
			o, found := pick(numbered, a)
			if !found {
				ok = false
				break
			}
			if indexOf(chosen, o.Value) < 0 {
				chosen = append(chosen, o)
			}
		}
		if ok {
			return chosen, nil
		}
		fmt.Fprintf(p.out, "Please answer with numbers from 1 to %d, such as 1,2.\n", len(numbered))
	}
}

// text asks question and returns a line of text. An empty answer gives def,
// or is asked again when def is empty. check, when not nil, rejects an
// answer with the reason to show.
func (p *prompter) text(question, def string, check func(string) error) (string, error) {
	fmt.Fprintln(p.out)
	for {
		if def != "" {
			fmt.Fprintf(p.out, "%s [%s]: ", question, def)
		} else {
			fmt.Fprintf(p.out, "%s: ", question)
		}
		answer, err := p.readLine()
		if err != nil {
			return "", err
		}
		if answer == "" {
			answer = def
		}
		if answer == "" {
			fmt.Fprintln(p.out, "An answer is required.")
			continue
		}
		if check != nil {
			if err := check(answer); err != nil {
				msg := err.Error()
				fmt.Fprintln(p.out, strings.ToUpper(msg[:1])+msg[1:]+".")
				continue
			}
		}
		return answer, nil
	}
}

// yesNo asks a yes or no question; an empty answer gives def.
func (p *prompter) yesNo(question string, def bool) (bool, error) {
	hint := "[y/N]"
	if def {
		hint = "[Y/n]"
	}
	for {
		fmt.Fprintf(p.out, "%s %s ", question, hint)
		answer, err := p.readLine()
		if err != nil {
			return false, err
		}
		switch strings.ToLower(answer) {
		case "":
			return def, nil
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		}
		fmt.Fprintln(p.out, "Please answer y or n.")
	}
}

// list prints question and its options, numbering those that can be
// chosen, and returns them.
func (p *prompter) list(question string, opts []simple.Option) []simple.Option {
	fmt.Fprintf(p.out, "\n%s\n", question)
	var numbered []simple.Option
	for _, o := range opts {
		label := o.Label
		if o.Help != "" {
			label += ": " + o.Help
		}
		if o.Later {
			fmt.Fprintf(p.out, "   -  %s (coming later)\n", label)
			continue
		}
		numbered = append(numbered, o)
		fmt.Fprintf(p.out, "  %2d. %s\n", len(numbered), label)
	}
	return numbered
}

// pick returns the option an answer names, by number or by value.
func pick(opts []simple.Option, answer string) (simple.Option, bool) {
	if n, err := strconv.Atoi(answer); err == nil {
		if n >= 1 && n <= len(opts) {
			return opts[n-1], true
		}
		return simple.Option{}, false
	}
	if i := indexOf(opts, strings.ToLower(answer)); i >= 0 {
		return opts[i], true
	}
	return simple.Option{}, false
}

// indexOf returns the position of the option with the given value, or -1.
func indexOf(opts []simple.Option, value string) int {
	for i, o := range opts {
		if o.Value == value {
			return i
		}
	}
	return -1
}
