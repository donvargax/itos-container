package cli

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
)

// completionModel is a command line of every shape the completion walks: a
// command with its own flags and positionals, an alias, an enum, hidden
// commands and flags, a negatable switch and a flag that may repeat.
type completionModel struct {
	Verbose bool `name:"verbose" negatable:""`

	Search struct {
		Path    string   `arg:""`
		Target  string   `arg:"" optional:""`
		Mode    string   `name:"mode" enum:"fast,slow" short:"m" required:""`
		Output  string   `name:"output" short:"o"`
		Enabled bool     `name:"enabled" negatable:""`
		Secret  bool     `name:"secret" hidden:""`
		Repeat  []string `sep:"none"`
	} `cmd:""`

	Inspect struct {
		Path string `arg:""`
	} `cmd:"" aliases:"i"`

	Shell struct {
		Name string `arg:"" enum:"bash,zsh,fish,powershell" required:""`
	} `cmd:""`

	Hidden struct{} `cmd:"" hidden:""`
}

func completionModelForTest(t *testing.T) *kong.Application {
	t.Helper()
	var model completionModel
	app, err := kong.New(&model, kong.Name("test"))
	if err != nil {
		t.Fatal(err)
	}
	return app.Model
}

func TestCompleteUsesKongCommandAndFlagModel(t *testing.T) {
	model := completionModelForTest(t)
	tests := []struct {
		name  string
		words []string
		want  []string
	}{
		{name: "top-level commands omit hidden commands", want: []string{"inspect", "search", "shell", ":none"}},
		{name: "command prefix", words: []string{"s"}, want: []string{"search", "shell", ":none"}},
		{name: "command flags", words: []string{"search", "src", "dst", "--mo"}, want: []string{"--mode", ":none"}},
		{name: "short flags", words: []string{"search", "src", "dst", "-m"}, want: []string{"-m", ":none"}},
		{name: "enum flag value after equals", words: []string{"search", "src", "dst", "--mode=f"}, want: []string{"fast", ":none"}},
		{name: "non-enum switch value is not treated as enum completion", words: []string{"search", "src", "dst", "--enabled=x"}, want: []string{":none"}},
		{name: "non-enum flag value in next word offers no values", words: []string{"search", "--output", ""}, want: []string{":none"}},
		{name: "enum flag value in next word", words: []string{"search", "--mode", "f"}, want: []string{"fast", ":none"}},
		{name: "flag awaiting a value", words: []string{"search", "--mode", ""}, want: []string{"fast", "slow", ":none"}},
		{name: "short flag awaiting a value", words: []string{"search", "-m", ""}, want: []string{"fast", "slow", ":none"}},
		{name: "consumed flag value leaves the positional available", words: []string{"search", "--mode", "fast", ""}, want: []string{":files"}},
		{name: "parent flag after command arguments", words: []string{"search", "src", "dst", "--verb"}, want: []string{"--verbose", ":none"}},
		{name: "negatable switches", words: []string{"search", "src", "dst", "--no-"}, want: []string{"--no-enabled", "--no-verbose", ":none"}},
		{name: "hidden flags are omitted", words: []string{"search", "src", "dst", "--sec"}, want: []string{":none"}},
		{name: "first positional argument uses files", words: []string{"search", ""}, want: []string{":files"}},
		{name: "end of options leaves a positional to files", words: []string{"search", "--", ""}, want: []string{":files"}},
		{name: "end of options keeps a dash-prefixed word positional", words: []string{"search", "--", "--mode"}, want: []string{":files"}},
		{name: "end of options after positionals has no fallback", words: []string{"search", "src", "dst", "--", ""}, want: []string{":none"}},
		{name: "positional enum values", words: []string{"shell", "p"}, want: []string{"powershell", ":none"}},
		{name: "command aliases follow the model", words: []string{"i", ""}, want: []string{":files"}},
		{name: "unknown complete flag falls through to positional", words: []string{"search", "--unknown", ""}, want: []string{":files"}},
		{name: "no positional remains", words: []string{"search", "src", "dst", ""}, want: []string{":none"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := Complete(model, test.words); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("Complete(%q) = %q, want %q", test.words, got, test.want)
			}
		})
	}
}

func TestCompleteSingleCommandAndFlagWithoutShortName(t *testing.T) {
	var model struct {
		Only struct {
			Path string `arg:""`
			Name string `name:"name"`
		} `cmd:""`
	}
	app, err := kong.New(&model, kong.Name("test"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := Complete(app.Model, nil), []string{"only", ":none"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Complete(nil) = %q, want %q", got, want)
	}
	if got, want := Complete(app.Model, []string{"only", "-"}), []string{"--help", "--name", "-h", ":none"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Complete(only, -) = %q, want %q", got, want)
	}
}

// A short flag of one rune, as a rune rather than a byte, is what kong's
// model records and what the candidates must be named from.
func TestCompleteRecognizesRuneOneShortFlagFromKongModel(t *testing.T) {
	text := reflect.TypeOf("")
	command := reflect.StructOf([]reflect.StructField{
		{Name: "Mode", Type: text, Tag: `enum:"fast,slow" name:"mode" short:"\x01" required:""`},
	})
	modelType := reflect.StructOf([]reflect.StructField{
		{Name: "Search", Type: command, Tag: `cmd:""`},
	})
	app, err := kong.New(reflect.New(modelType).Interface(), kong.Name("test"))
	if err != nil {
		t.Fatalf("build Kong model with rune-one short flag: %v", err)
	}
	if got := app.Model.Node.Children[0].Flags[0].Short; got != 1 {
		t.Fatalf("Kong model short flag = %U, want U+0001", got)
	}

	words := []string{"search", "-" + string(rune(1)), "f"}
	if got, want := Complete(app.Model, words), []string{"fast", ":none"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Complete(%q) = %q, want %q", words, got, want)
	}
}

// A flag that may repeat stays a flag after the word before it, so a value
// is never read as a command the model knows.
func TestCompleteTreatsARepeatableFlagAsAFlag(t *testing.T) {
	model := completionModelForTest(t)
	words := []string{"search", "--repeat", "inspect", ""}
	if got, want := Complete(model, words), []string{":files"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Complete(%q) = %q, want %q", words, got, want)
	}
}

// A shell with no script is refused as a usage error naming it, exit 2: the
// enum kong holds is the one the command takes, and the refusal is the one
// a model without the enum would give.
func TestCompletionRunRejectsUnsupportedShell(t *testing.T) {
	var stdout, stderr strings.Builder
	ui := &UI{Stdout: &stdout, Stderr: &stderr}
	if code := (Completion{Shell: "tcsh"}).Run(ui); code != CodeUsage {
		t.Fatalf("Run exit = %d, want %d", code, CodeUsage)
	}
	if !strings.Contains(stderr.String(), "tcsh") {
		t.Fatalf("Run error = %q, want shell name", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("Run wrote unexpected standard output %q", stdout.String())
	}
}

// A shell with a script is written to stdout whole, and nothing is said on
// stderr: the script is what the shell reads.
func TestCompletionRunWritesTheScriptAndSaysNothingElse(t *testing.T) {
	for shell, location := range map[string]string{
		"bash":       "~/.bashrc",
		"zsh":        "~/.zshrc",
		"fish":       "~/.config/fish/completions/itos-container.fish",
		"powershell": "$PROFILE",
	} {
		t.Run(shell, func(t *testing.T) {
			var stdout, stderr strings.Builder
			ui := &UI{Stdout: &stdout, Stderr: &stderr}
			if code := (Completion{Shell: shell}).Run(ui); code != 0 {
				t.Fatalf("Run exit = %d, want 0", code)
			}
			if stderr.Len() != 0 {
				t.Errorf("error output = %q, want none", stderr.String())
			}
			script := stdout.String()
			for _, want := range []string{location, "__complete", "itos-container"} {
				if !strings.Contains(script, want) {
					t.Errorf("the script does not name %q\n%s", want, script)
				}
			}
		})
	}
}

// Every shell the command declares has a script, and no script names a shell
// the command does not: a shell named in the help that printed nothing, and
// one a script exists for that no command line reaches, are both broken
// promises.
func TestEveryShellTheCommandDeclaresHasAScriptAndNoOther(t *testing.T) {
	declared := completionShells
	if len(declared) == 0 {
		t.Fatal("the command's shell argument declares no shells")
	}
	for _, shell := range declared {
		if _, ok := Scripts()[shell]; !ok {
			t.Errorf("the command declares %q and has no script for it", shell)
		}
	}
	for shell := range Scripts() {
		if !slices.Contains(declared, shell) {
			t.Errorf("there is a script for %q, which no command line names", shell)
		}
	}
}

// completionShells is the shell argument of the command in a model of its
// own, so the shells it declares are read from kong and not from a copy of
// the tag.
var completionShells = func() []string {
	app, err := kong.New(&struct {
		Completion Completion `cmd:""`
	}{}, kong.Name("test"))
	if err != nil {
		panic(err)
	}
	return app.Model.Node.Children[0].Positional[0].EnumSlice()
}()
