// Package cli is the UI every command shares: where a command reads and
// writes, the lines on stderr, the --json object, and the one place an error
// becomes an exit code.
//
// Commands supply classifiers for their typed failures. This UI owns output,
// joined-error traversal and the fallback for an unclassified internal error.
package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// The exit codes, as count's command's help states them.
const (
	CodeUsage       = 2
	CodeEnvironment = 3
	CodeInternal    = 70
)

// UI is a command's ends: what it reads and where it writes. Nothing here
// asks a question, so there is no terminal to know of.
// Classifier gives a known failure its code and problems, or zero and nil.
type Classifier func(error) (int, []Problem)

type UI struct {
	Classify Classifier
	In       io.Reader
	Stdout   io.Writer
	Stderr   io.Writer
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
	problems, code := report(err, u.Classify)
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
func Code(err error, classify Classifier) int {
	_, code := report(err, classify)
	return code
}

// Message is err as people read it: each problem's sentence, a line each.
func Message(err error, classify Classifier) string {
	problems, _ := report(err, classify)
	lines := make([]string, len(problems))
	for i, p := range problems {
		lines[i] = p.Message
	}
	return strings.Join(lines, "\n")
}

// report is every problem of err, joined ones each on its own, and the exit
// code of them all: the highest.
func report(err error, classifier Classifier) ([]Problem, int) {
	var problems []Problem
	code := 0
	for _, e := range leaves(err) {
		c, p := classify(e, classifier)
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

// internal is err as an internal error, a defect of ours.
func internal(err error) []Problem {
	return []Problem{{Rule: "internal", Message: err.Error(), Fix: "report this as a bug"}}
}

// classify delegates known failures and keeps the unknown-error fallback here.
func classify(err error, classifier Classifier) (int, []Problem) {
	if classifier != nil {
		if code, problems := classifier(err); code != 0 {
			return code, problems
		}
	}
	return CodeInternal, internal(err)
}
