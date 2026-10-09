// Package cli is the UI every command shares: where a command reads and
// writes, the lines on stderr, the --json object, and the one place an error
// becomes an exit code.
//
// The domain's errors are a sealed set of plain types carrying no exit code,
// rule ID or wording. Here each kind has one switch giving it its code
// (docs of count's command, "Exit codes"), its rule ID for --json, its line
// for people and what to do about it, every kind classified and none with a
// default: a kind no case classifies is the 70 of an unclassified bug, never
// a child process's status passed through.
package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/donvargax/itos-container/internal/count/port"
)

// The exit codes, as count's command's help states them.
const (
	CodeUsage       = 2
	CodeEnvironment = 3
	CodeInternal    = 70
)

// UI is a command's ends: what it reads and where it writes. Nothing here
// asks a question, so there is no terminal to know of.
type UI struct {
	In     io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

// Problem is one thing wrong, as --json gives it: its rule ID, its sentence
// for people, and what to do about it.
type Problem struct {
	Rule    string `json:"rule"`
	Message string `json:"message"`
	Fix     string `json:"fix"`
}

// Fail reports err, a command failing, and returns its exit code: with
// withJSON the failure's object on stdout first, then each problem a line on
// stderr.
func (u *UI) Fail(err error, withJSON bool) int {
	problems, code := report(err)
	return u.fail(problems, code, withJSON)
}

// Usage reports err, a command line kong could not parse, as Fail does: a
// usage error, exit 2, in the words of the person who wrote the line. The
// command is named as it was written, so the sentence says what was read,
// never kong's own such as "EOL".
func (u *UI) Usage(command string, err error, withJSON bool) int {
	message := err.Error()
	if command != "" {
		message = command + ": " + message
	}
	return u.fail([]Problem{{
		Rule:    "usage",
		Message: message,
		Fix:     "run itos-container --help for the commands and the flags each takes",
	}}, CodeUsage, withJSON)
}

func (u *UI) fail(problems []Problem, code int, withJSON bool) int {
	if withJSON {
		u.JSON(struct {
			Schema   int       `json:"schema"`
			OK       bool      `json:"ok"`
			Problems []Problem `json:"problems"`
		}{1, false, problems})
	}
	for _, p := range problems {
		u.Line(p.Message)
	}
	return code
}

// JSON writes v on stdout as one line of JSON, as --json prints it; a
// failure to is a line on stderr.
func (u *UI) JSON(v any) {
	enc := json.NewEncoder(u.Stdout)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		u.Line(err.Error())
	}
}

// Line writes message on stderr as a line for people, itos-container:
// first. A stderr that cannot be written leaves nowhere else to say so.
func (u *UI) Line(message string) {
	_, _ = fmt.Fprintf(u.Stderr, "itos-container: %s\n", message)
}

// Code is err's exit code.
func Code(err error) int {
	_, code := report(err)
	return code
}

// Message is err as people read it: each problem's sentence, a line each.
func Message(err error) string {
	problems, _ := report(err)
	lines := make([]string, len(problems))
	for i, p := range problems {
		lines[i] = p.Message
	}
	return strings.Join(lines, "\n")
}

// report is every problem of err, joined ones each on its own, and the exit
// code of them all: the highest.
func report(err error) ([]Problem, int) {
	var problems []Problem
	code := 0
	for _, e := range leaves(err) {
		c, p := classify(e)
		problems = append(problems, p...)
		code = max(code, c)
	}
	return problems, code
}

// leaves are the errors err joins (errors.Join), each in turn, or err.
func leaves(err error) []error {
	joined, ok := err.(interface{ Unwrap() []error })
	if !ok {
		return []error{err}
	}
	var all []error
	for _, e := range joined.Unwrap() {
		all = append(all, leaves(e)...)
	}
	return all
}

// one is a problem of its rule, its sentence and its fix.
func one(rule, format, fix string, args ...any) []Problem {
	return []Problem{{Rule: rule, Message: fmt.Sprintf(format, args...), Fix: fix}}
}

// internal is err as an internal error, a defect of ours.
func internal(err error) []Problem {
	return []Problem{{Rule: "internal", Message: err.Error(), Fix: "report this as a bug"}}
}

// classify is err's exit code and problems, from the sealed set it is of:
// only an error of none is an internal one, exit 70.
//
// The switch below starts from an internal error and names every kind, so a
// kind added without a case here would still exit 70, as a bug, rather than
// take another kind's code by accident.
func classify(err error) (int, []Problem) {
	var failure port.ReadFailure
	if errors.As(err, &failure) {
		return readProblem(failure)
	}
	return CodeInternal, internal(err)
}
