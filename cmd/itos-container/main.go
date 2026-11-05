// Command itos-container counts the lines and words of a file or of
// standard input. It runs alone, as itos-container.
//
// kong declares the command line: the flags and commands are the fields of
// commandLine, each switch with its --no- pair and its environment variable.
// The main output goes to stdout; logs, structured, and errors go to stderr.
//
// main only assembles: the count command is a slice, its kong struct and its
// handler in a package of its own (internal/slices/count/command), and main parses
// the command line, runs the one it names through the UI the slices share
// (internal/cli) and exits with the code that slice returns. A usage error,
// which kong finds before any slice runs, is the one failure main reports
// itself, through that UI; internal/cli's Flags hold kong to the rules on
// flags it does not hold by itself.
package main

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime"
	"strings"

	"github.com/alecthomas/kong"
	"golang.org/x/term"

	"github.com/donvargax/itos-container/internal/cli"
	manifestdisk "github.com/donvargax/itos-container/internal/engine/manifest/disk"
	"github.com/donvargax/itos-container/internal/engine/manifest/jsonc"
	"github.com/donvargax/itos-container/internal/engine/manifest/reference"
	"github.com/donvargax/itos-container/internal/engine/policy/disk"
	policyyaml "github.com/donvargax/itos-container/internal/engine/policy/yaml"
	readinessdisk "github.com/donvargax/itos-container/internal/engine/readiness/disk"
	"github.com/donvargax/itos-container/internal/engine/readiness/docker"
	readiness "github.com/donvargax/itos-container/internal/engine/readiness/domain"
	readinessport "github.com/donvargax/itos-container/internal/engine/readiness/port"
	"github.com/donvargax/itos-container/internal/slices/count/command"
	countdisk "github.com/donvargax/itos-container/internal/slices/count/disk"
	"github.com/donvargax/itos-container/internal/slices/count/input"
	doctor "github.com/donvargax/itos-container/internal/slices/doctor/command"
	"github.com/donvargax/itos-container/internal/slices/doctor/inspection"
	runcommand "github.com/donvargax/itos-container/internal/slices/run/command"
	"github.com/donvargax/itos-container/internal/version"
)

// commandLine is the command line: its flags and commands.
type commandLine struct {
	// An action, not a switch: it prints and exits, so it has no --no- pair
	// and no environment variable.
	ShowVersion kong.VersionFlag `name:"version" help:"Print the version and the commit it was built from, and exit."`

	Count      command.CLI    `cmd:"" help:"Count the lines and words of a file, or of standard input."`
	Doctor     doctor.CLI     `cmd:"" help:"Inspect the strict offline policy and restricted worker manifest."`
	Run        runcommand.CLI `cmd:"" help:"Validate a run invocation; offline startup is not implemented."`
	Version    struct{}       `cmd:"" help:"Print the version and the commit it was built from."`
	Completion cli.Completion `cmd:"" help:"Print a shell completion script."`
	Complete   struct{}       `cmd:"" name:"__complete" hidden:""`
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// run parses args and runs what they name, returning the exit code. With no
// arguments at all there is nothing to run and no implicit action to run in
// its place, so the program says what it can do, on stdout, and succeeds.
//
// kong's own exit is taken over and becomes run's return: --help and
// --version print and ask kong to stop, and a run that returns the code is
// what main exits with, so a test can read what a run printed.
func run(args []string, in io.Reader, stdout, stderr io.Writer, options ...kong.Option) int {
	slog.SetDefault(logger(stderr))
	ui := &cli.UI{In: in, Stdout: stdout, Stderr: stderr, Classify: command.Classify}
	if len(args) == 0 {
		args = []string{"--help"}
	}
	stopped := -1
	var c commandLine
	defaults := append(cli.Flags(),
		kong.Name("itos-container"),
		kong.Description("Count the lines and words of a file, or of standard input."),
		kong.Writers(stdout, stderr),
		kong.Exit(func(code int) { stopped = code }),
		kong.Vars{"version": version.Text("itos-container")},
	)
	parser, err := kong.New(&c, append(defaults, options...)...)
	if err != nil {
		// A model kong refuses is a bug, reported as one.
		return ui.Fail(err, wantsJSON(args))
	}
	if len(args) > 0 && args[0] == "__complete" {
		for _, line := range cli.Complete(parser.Model, args[1:]) {
			_, _ = fmt.Fprintln(stdout, line)
		}
		return 0
	}
	ctx, err := parser.Parse(runOptions(args))
	if stopped >= 0 {
		return stopped
	}
	if err != nil {
		// With --json, the failure's object too.
		return ui.Usage(commandName(args), err, wantsJSON(args))
	}
	c.Count.Files = input.Files{Disk: countdisk.Files{}, In: in}
	switch command, _, _ := strings.Cut(ctx.Command(), " "); command {
	case "count":
		return c.Count.Run(ui)
	case "doctor":
		c.Doctor.Policies = inspection.Policies{Source: policyyaml.Source{Files: disk.Files{}}}
		c.Doctor.Manifests = inspection.Manifests{Files: manifestdisk.Files{}, Parser: jsonc.Parser{}, References: reference.References{}}
		checkout := &readinessdisk.Checkout{}
		root := os.Getenv("ITOS_ROOT")
		if root == "" {
			root, _ = os.Getwd()
		}
		c.Doctor.Readiness = inspection.Readiness{Inspector: readiness.Inspector{
			Checkout: checkout, Policy: policyyaml.Source{Files: disk.Files{OpenRoot: checkout.OpenRoot}},
			Files: manifestdisk.Files{OpenRoot: checkout.OpenRoot}, Parser: jsonc.Parser{}, References: reference.References{}, Engine: docker.Engine{},
		}, Input: readinessport.Input{Root: root, OS: runtime.GOOS, Architecture: runtime.GOARCH,
			Host: os.Getenv("DOCKER_HOST"), Context: os.Getenv("DOCKER_CONTEXT"), TLSVerify: os.Getenv("DOCKER_TLS_VERIFY"), CertPath: os.Getenv("DOCKER_CERT_PATH"), APIVersion: os.Getenv("DOCKER_API_VERSION"),
		}}
		return c.Doctor.Run(ui)
	case "run":
		ui.Classify = runcommand.Classify
		c.Run.InheritedJSON = os.Getenv("ITOS_JSON") == "1"
		return c.Run.Run(ui)
	case "version":
		// What --version prints, as kong prints it.
		_, _ = fmt.Fprintln(stdout, parser.Model.Vars()["version"])
		return 0
	case "completion":
		return c.Completion.Run(ui)
	}
	// A command kong took that has no case here: a bug.
	return ui.Fail(fmt.Errorf("no case runs the command %q", ctx.Command()), wantsJSON(args))
}

// commandName is the command args name, the first word that is not a flag
// and not a flag's value, as the person wrote it; "" where the line names
// none, so a usage error about such a line says what was wrong and nothing
// more.
func commandName(args []string) string {
	for i := 0; i < len(args); i++ {
		word := args[i]
		switch {
		case word == "--":
			return ""
		case strings.HasPrefix(word, "-") && word != "-":
			// A flag's value is the next word unless the flag took it as
			// --flag=value, which is one word.
			if !strings.Contains(word, "=") {
				i++
			}
		default:
			return word
		}
	}
	return ""
}

// wantsJSON is whether args ask for --json before any --, as a command line
// kong could not parse is read.
func wantsJSON(args []string) bool {
	json := false
	for _, a := range args {
		switch a {
		case "--":
			return json
		case "--json", "--json=true":
			json = true
		case "--no-json", "--json=false":
			json = false
		}
	}
	return json
}

// logger writes structured logs to w: JSON when w is not a terminal, text
// when it is, warnings and errors only, so a run is quiet by default.
func logger(w io.Writer) *slog.Logger {
	return loggerFor(w, term.IsTerminal)
}

// loggerFor takes terminal detection separately so redirected test runs can
// exercise the same text output as an interactive run.
func loggerFor(w io.Writer, isTerminal func(int) bool) *slog.Logger {
	opts := &slog.HandlerOptions{Level: slog.LevelWarn}
	if f, ok := w.(*os.File); ok {
		if isTerminal(int(f.Fd())) {
			return slog.New(slog.NewTextHandler(w, opts))
		}
	}
	return slog.New(slog.NewJSONHandler(w, opts))
}
