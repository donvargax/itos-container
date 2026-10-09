package cli

import (
	"reflect"
	"testing"

	"github.com/alecthomas/kong"
)

// The scenarios (ID-CLI-05 to 07) hold the usage errors by their exit code
// and the flag they name. These hold what no scenario reads: a flag given no
// value, a flag's value never read as a flag, and a switch's environment
// variable, which one scenario sets and none reads a refusal of.

// line is a command line of every kind of flag Flags reads: a switch with
// its --no- pair and a flag that takes one value.
type line struct {
	Folder string `arg:"" optional:""`
	Stack  string `placeholder:"STACK"`
	JSON   bool   `negatable:"" env:"ITOS_CONTAINER_CLI_TEST_JSON"`
}

// parse is args parsed as line, through Flags, and the error kong gives.
func parse(t *testing.T, args ...string) (line, error) {
	t.Helper()
	var l line
	parser, err := kong.New(&l, append(Flags(), kong.Name("itos-container"), kong.Exit(func(int) { t.Fatal("kong exited") }))...)
	if err != nil {
		t.Fatal(err)
	}
	_, err = parser.Parse(args)
	return l, err
}

func TestAFlagGivenNoValueIsRefusedNamingIt(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"--stack"}, "--stack: needs a value, as --stack STACK"},
		{[]string{"--stack", "--json"}, "--stack: needs a value, and --json is a flag, never its value; to give it as the value, write --stack=--json"},
		{[]string{"--stack", "-h"}, "--stack: needs a value, and -h is a flag, never its value; to give it as the value, write --stack=-h"},
	}
	for _, c := range cases {
		if _, err := parse(t, c.args...); err == nil || err.Error() != c.want {
			t.Errorf("%v: got %v, want %q", c.args, err, c.want)
		}
	}
}

func TestAValueGivenAsAFlagsOwnIsTaken(t *testing.T) {
	l, err := parse(t, "made", "--stack=go", "--stack=--json")
	if err != nil {
		t.Fatal(err)
	}
	if l.Folder != "made" || l.Stack != "--json" {
		t.Errorf("got %+v", l)
	}
}

func TestASwitchGivenAValueIsRefusedNamingItAndItsPair(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"--json=yes"}, "--json: a switch takes no value; give --json or --no-json alone"},
		{[]string{"--no-json=false"}, "--json: a switch takes no value; give --json or --no-json alone"},
	}
	for _, c := range cases {
		if _, err := parse(t, c.args...); err == nil || err.Error() != c.want {
			t.Errorf("%v: got %v, want %q", c.args, err, c.want)
		}
	}
}

func TestSwitchesGivenAloneAreTaken(t *testing.T) {
	l, err := parse(t, "--json", "--no-json")
	if err != nil {
		t.Fatal(err)
	}
	if l.JSON {
		t.Errorf("got %+v", l)
	}
}

func TestASwitchTakesItsEnvironmentVariablesValue(t *testing.T) {
	for value, want := range map[string]bool{"1": true, "false": false} {
		t.Setenv("ITOS_CONTAINER_CLI_TEST_JSON", value)
		l, err := parse(t)
		if err != nil {
			t.Fatalf("%s: %v", value, err)
		}
		if l.JSON != want {
			t.Errorf("%s: got %v, want %v", value, l.JSON, want)
		}
	}
	t.Setenv("ITOS_CONTAINER_CLI_TEST_JSON", "true")
	l, err := parse(t, "--no-json")
	if err != nil || l.JSON {
		t.Errorf("--no-json over the variable: got %+v, %v", l, err)
	}
}

// A switch of no --no- pair is named alone, never with a pair that is not
// its own: the message must not offer a flag the command line cannot take.
func TestEitherNamesThePairOnlyOfANegatableSwitch(t *testing.T) {
	for _, c := range []struct {
		name string
		flag *kong.Flag
		want string
	}{
		{name: "plain", flag: flagNamed(t, "quiet", `name:"quiet"`), want: ""},
		{name: "negatable", flag: flagNamed(t, "json", `name:"json" negatable:""`), want: " or --no-json"},
	} {
		if got := either(c.flag); got != c.want {
			t.Errorf("%s: either = %q, want %q", c.name, got, c.want)
		}
	}
}

// flagNamed is the flag named name in a model declaring it with the tag, so
// the test holds either's reading of kong's own tag and not a field's place.
func flagNamed(t *testing.T, name, tag string) *kong.Flag {
	t.Helper()
	typ := reflect.StructOf([]reflect.StructField{{Name: "Switch", Type: reflect.TypeFor[bool](), Tag: reflect.StructTag(tag)}})
	parser, err := kong.New(reflect.New(typ).Interface(), Flags()...)
	if err != nil {
		t.Fatal(err)
	}
	for _, flag := range parser.Model.Flags {
		if flag.Name == name {
			return flag
		}
	}
	t.Fatalf("the model declares no flag named %q", name)
	return nil
}
