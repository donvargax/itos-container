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
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/alecthomas/kong"
	"golang.org/x/term"

	"github.com/donvargax/itos-container/internal/cli"
	approvalcanonical "github.com/donvargax/itos-container/internal/engine/approval/canonical"
	approvaldisk "github.com/donvargax/itos-container/internal/engine/approval/disk"
	approvaldomain "github.com/donvargax/itos-container/internal/engine/approval/domain"
	approvalhost "github.com/donvargax/itos-container/internal/engine/approval/host"
	approvalport "github.com/donvargax/itos-container/internal/engine/approval/port"
	approvalpresentation "github.com/donvargax/itos-container/internal/engine/approval/presentation"
	manifestdisk "github.com/donvargax/itos-container/internal/engine/manifest/disk"
	"github.com/donvargax/itos-container/internal/engine/manifest/jsonc"
	"github.com/donvargax/itos-container/internal/engine/manifest/reference"
	"github.com/donvargax/itos-container/internal/engine/policy/disk"
	policyidna "github.com/donvargax/itos-container/internal/engine/policy/idna"
	policyyaml "github.com/donvargax/itos-container/internal/engine/policy/yaml"
	readinessdisk "github.com/donvargax/itos-container/internal/engine/readiness/disk"
	"github.com/donvargax/itos-container/internal/engine/readiness/docker"
	readiness "github.com/donvargax/itos-container/internal/engine/readiness/domain"
	readinessport "github.com/donvargax/itos-container/internal/engine/readiness/port"
	readinessreference "github.com/donvargax/itos-container/internal/engine/readiness/reference"
	sessioncodec "github.com/donvargax/itos-container/internal/engine/session/codec"
	sessiondisk "github.com/donvargax/itos-container/internal/engine/session/disk"
	sessiondocker "github.com/donvargax/itos-container/internal/engine/session/docker"
	sessiondomain "github.com/donvargax/itos-container/internal/engine/session/domain"
	sessionentropy "github.com/donvargax/itos-container/internal/engine/session/entropy"
	sessionport "github.com/donvargax/itos-container/internal/engine/session/port"
	sessionpresentation "github.com/donvargax/itos-container/internal/engine/session/presentation"
	approve "github.com/donvargax/itos-container/internal/slices/approve/command"
	"github.com/donvargax/itos-container/internal/slices/count/command"
	countdisk "github.com/donvargax/itos-container/internal/slices/count/disk"
	"github.com/donvargax/itos-container/internal/slices/count/input"
	doctor "github.com/donvargax/itos-container/internal/slices/doctor/command"
	"github.com/donvargax/itos-container/internal/slices/doctor/inspection"
	runcommand "github.com/donvargax/itos-container/internal/slices/run/command"
	"github.com/donvargax/itos-container/internal/slices/run/signals"
	statuscommand "github.com/donvargax/itos-container/internal/slices/status/command"
	stopcommand "github.com/donvargax/itos-container/internal/slices/stop/command"
	"github.com/donvargax/itos-container/internal/version"
)

// commandLine is the command line: its flags and commands.
type commandLine struct {
	// An action, not a switch: it prints and exits, so it has no --no- pair
	// and no environment variable.
	ShowVersion kong.VersionFlag `name:"version" help:"Print the version and the commit it was built from, and exit."`

	Approve    approve.CLI       `cmd:"" help:"Review and remember exact offline settings."`
	Count      command.CLI       `cmd:"" help:"Count the lines and words of a file, or of standard input."`
	Doctor     doctor.CLI        `cmd:"" help:"Inspect the policy file and restricted worker manifest."`
	Run        runcommand.CLI    `cmd:"" help:"Run a literal command in an approved offline worker."`
	Status     statuscommand.CLI `cmd:"" help:"Inspect owned sessions in the effective project."`
	Stop       stopcommand.CLI   `cmd:"" help:"Stop and clean one registered project session."`
	Version    struct{}          `cmd:"" help:"Print the version and the commit it was built from."`
	Completion cli.Completion    `cmd:"" help:"Print a shell completion script."`
	Complete   struct{}          `cmd:"" name:"__complete" hidden:""`
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
		if name := commandName(runOptions(args)); name == "status" || name == "stop" {
			return (sessionpresentation.UI{Out: stdout, Err: stderr}).Print(sessionport.RecoveryResult{Failures: []sessionport.RecoveryFailure{{Outcome: sessionport.Usage}}}, recoveryJSON(args))
		}
		return ui.Usage(commandName(args), err, wantsJSON(args))
	}
	c.Count.Files = input.Files{Disk: countdisk.Files{}, In: in}
	switch command, _, _ := strings.Cut(ctx.Command(), " "); command {
	case "count":
		return c.Count.Run(ui)
	case "doctor":
		c.Doctor.Policies = inspection.Policies{Source: policyyaml.Source{Files: disk.Files{}}, Names: policyidna.Names{}}
		c.Doctor.Manifests = inspection.Manifests{Files: manifestdisk.Files{}, Parser: jsonc.Parser{}, References: reference.References{}}
		service, presentation := approvalServices(in, stdout, stderr)
		c.Doctor.Readiness = service.Readiness
		c.Doctor.Approval = service
		c.Doctor.Presentation = presentation
		root := os.Getenv("ITOS_ROOT")
		if root == "" {
			root, _ = os.Getwd()
		}
		state := os.Getenv("XDG_STATE_HOME")
		if state == "" && filepath.IsAbs(os.Getenv("HOME")) {
			state = filepath.Join(os.Getenv("HOME"), ".local", "state")
		}
		if filepath.IsAbs(state) {
			state = filepath.Join(state, "itos-container")
		} else {
			state = ""
		}

		c.Doctor.Credentials = inspection.Credentials{FS: &readinessdisk.CredentialFiles{}, Input: readinessport.CredentialInput{
			Root: root, State: state, OS: runtime.GOOS, Architecture: runtime.GOARCH, UID: int64(os.Geteuid()),
		}, Account: &approvalhost.Process{}}
		return c.Doctor.Run(ui)
	case "approve":
		service, presentation := approvalServices(in, stdout, stderr)
		c.Approve.Service = service
		c.Approve.Presentation = presentation
		return c.Approve.Run(ui)
	case "run":
		service, _ := approvalServices(in, stdout, stderr)
		c.Run.Approval = approvaldomain.Preflight{Service: service, Local: service.Readiness.(approvalport.StagedReadiness), Canonical: approvalcanonical.JSON{}}
		c.Run.Snapshots = approvaldisk.Snapshots{Store: service.Store.(approvaldisk.Store), Records: approvalcanonical.JSON{}}
		runner, _ := sessionServices(service)
		c.Run.Runner = runner
		interrupts, release := signals.Capture(context.Background())
		defer release()
		c.Run.Context = interrupts
		ui.Classify = runcommand.Classify
		c.Run.InheritedJSON = os.Getenv("ITOS_JSON") == "1"
		return c.Run.Run(ui)
	case "status":
		service, _ := approvalServices(in, stdout, stderr)
		_, c.Status.Service = sessionServices(service)
		c.Status.Presentation = sessionpresentation.UI{Out: stdout, Err: stderr}
		c.Status.JSON = recoveryJSON(args)
		return c.Status.Run()
	case "stop":
		service, _ := approvalServices(in, stdout, stderr)
		_, c.Stop.Service = sessionServices(service)
		c.Stop.Presentation = sessionpresentation.UI{Out: stdout, Err: stderr}
		c.Stop.JSON = recoveryJSON(args)
		return c.Stop.Run()
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

func sessionServices(service approvaldomain.Service) (sessiondomain.Runner, sessiondomain.Recovery) {
	source := service.Readiness.(inspection.Readiness)
	codec := sessioncodec.JSON{Approval: approvalcanonical.JSON{}, Parser: jsonc.Parser{}}
	protected := service.Store.(approvaldisk.Store)
	store := sessiondisk.Store{Storage: approvaldisk.Storage{Store: protected}, Codec: codec, Host: service.Host, Checkout: &readinessdisk.Checkout{}, Input: source.Input}
	registry := sessiondisk.Registry{Store: store, Storage: approvaldisk.Registry{Store: protected}, Codec: codec, Identity: approvalcanonical.JSON{}}
	engine := sessiondocker.Engine{Readiness: docker.Engine{}, Selection: source.Input.Host}
	return sessiondomain.Runner{Store: store, Registry: registry, Engine: engine, Entropy: sessionentropy.Random{}, Workspace: &sessiondisk.Workspace{}}, sessiondomain.Recovery{Registry: registry, Engine: engine}
}

// recoveryJSON serves parsed commands and usage refusals alike. Only literal 1
// inherits JSON; Kong's boolean environment decoder accepts other values too.
func recoveryJSON(args []string) bool {
	want := os.Getenv("ITOS_JSON") == "1"
	for _, a := range args {
		switch a {
		case "--json", "--json=true":
			want = true
		case "--no-json", "--json=false":
			want = false
		case "--":
			return want
		}
	}
	return want
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

// approvalServices assembles shared inspection and approval adapters. Construction
// defers policy, engine and approval inspection until the command requests it.
func approvalServices(in io.Reader, out, errout io.Writer) (approvaldomain.Service, approvalpresentation.UI) {
	checkout := &readinessdisk.Checkout{}
	root := os.Getenv("ITOS_ROOT")
	if root == "" {
		root, _ = os.Getwd()
	}
	source := inspection.Readiness{Inspector: readiness.Inspector{
		Checkout: checkout, Policy: policyyaml.Source{Files: disk.Files{OpenRoot: checkout.OpenRoot}}, Names: policyidna.Names{},
		Files: manifestdisk.Files{OpenRoot: checkout.OpenRoot}, Parser: jsonc.Parser{}, References: reference.References{}, Engine: docker.Engine{},
		Digests: readinessreference.Digests{},
	}, Input: readinessport.Input{Root: root, OS: runtime.GOOS, Architecture: runtime.GOARCH,
		Host: os.Getenv("DOCKER_HOST"), Context: os.Getenv("DOCKER_CONTEXT"), TLSVerify: os.Getenv("DOCKER_TLS_VERIFY"), CertPath: os.Getenv("DOCKER_CERT_PATH"), APIVersion: os.Getenv("DOCKER_API_VERSION"),
	}}
	codec := approvalcanonical.JSON{}
	service := approvaldomain.Service{Readiness: source, Host: approvalhost.Process{}, Codec: codec, Store: approvaldisk.Store{StateHome: os.Getenv("XDG_STATE_HOME"), Home: os.Getenv("HOME"), Records: codec}}
	service = coordinatedApproval(service, accountLookup)
	return service, approvalpresentation.UI{In: in, Out: out, Err: errout}
}

// Only cmd binds the session ownership validator to the approval engine's
// account-wide protected storage. Neither engine imports the other's adapter.
func coordinatedApproval(service approvaldomain.Service, account approvalport.Account) approvaldomain.Service {
	protected := service.Store.(approvaldisk.Store)
	protected.Control = nil
	source := service.Readiness.(inspection.Readiness)
	codec := sessioncodec.JSON{Approval: approvalcanonical.JSON{}, Parser: jsonc.Parser{}}
	evidence := sessiondisk.Reservations{
		Locations: approvaldisk.Locations{Store: protected},
		Store:     sessiondisk.Store{Codec: codec, Host: service.Host, Input: source.Input},
		Codec:     codec,
	}
	protected.Control = approvaldisk.Control{Account: account, Store: protected, Reservations: evidence}
	service.Store = protected
	return service
}

var accountLookup approvalport.Account = approvalhost.Process{}
