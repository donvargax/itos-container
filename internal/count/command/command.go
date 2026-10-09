// Package command is the count command: what its kong struct carries and
// what it does with the counts the domain found.
//
// It is the thin application handler the architecture describes: it carries
// the path and the switch, hands the path to the domain through the Files
// port it was given, and writes what came back. It counts nothing itself,
// reads no file, and knows no exit code: a failure is the domain's, and the
// UI gives it one.
package command

import (
	"fmt"

	"github.com/donvargax/itos-container/internal/cli"
	"github.com/donvargax/itos-container/internal/count/domain"
	"github.com/donvargax/itos-container/internal/count/port"
)

// CLI is the count command: the file it counts, and whether it prints the
// counts or the one object a script reads.
type CLI struct {
	// Files is the port the domain reads through, given by main, never a
	// flag: no value of a command line is read as a global one.
	Files port.Files `kong:"-"`

	File string `arg:"" help:"The file to count, or - for standard input."`
	JSON bool   `name:"json" negatable:"" env:"ITOS_CONTAINER_JSON" help:"Print one JSON object instead of the counts."`
}

// Help is count's own help: the shape of its --json object and every exit
// code it can return, so the two agree without a reader holding the source.
// The wording is what the help must hold, because a scenario checks it and a
// script reads it.
//
// The block is indented, so it is preformatted and holds its lines whole at
// any terminal width; only the sentence above it is re-wrapped.
func (CLI) Help() string {
	return "\nThe counts are for a person to read; a script reads --json and the exit code.\n\n" +
		"    --json prints one object: schema 1, ok true, file the argument, lines and words.\n" +
		"    Exit codes:\n" +
		"      0  success\n" +
		"      2  the command line is wrong\n" +
		"      3  the input is missing or cannot be read\n"
}

// counts is the one object --json prints on success, the contract a script
// reads: the argument as given, - for standard input, and what the domain
// counted of it.
type counts struct {
	Schema int    `json:"schema"`
	OK     bool   `json:"ok"`
	File   string `json:"file"`
	Lines  int    `json:"lines"`
	Words  int    `json:"words"`
}

// Run counts the file the command names, or reports the failure it met, and
// returns the exit code the UI gives it.
func (c CLI) Run(ui *cli.UI) int {
	result, failure := domain.File(c.Files, c.File)
	if failure != nil {
		return ui.Fail(failure, c.JSON)
	}
	if c.JSON {
		ui.JSON(counts{Schema: 1, OK: true, File: c.File, Lines: result.Lines, Words: result.Words})
		return 0
	}
	// The counts, and the path as given, never what was read: an input
	// holding a secret must not reach the output.
	_, _ = fmt.Fprintf(ui.Stdout, "%s: lines %d, words %d\n", c.File, result.Lines, result.Words)
	ui.Line("for scripts, use --json")
	return 0
}
