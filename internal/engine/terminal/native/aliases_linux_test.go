package native

import (
	"golang.org/x/term"
	"os"
	"reflect"
	"testing"
)

func controllingAliases(t *testing.T) {
	alias, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = alias.Close() }()
	before, err := term.GetState(int(os.Stdin.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = term.Restore(int(os.Stdin.Fd()), before) }()
	for _, inputAlias := range []bool{false, true} {
		in, out := os.Stdin, alias
		if inputAlias {
			in, out = out, in
		}
		lease, err := NewPair(in, out, "linux").Acquire(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		result := lease.Close()
		after, err := term.GetState(int(os.Stdin.Fd()))
		if err != nil || result.RestoreErr != nil || !reflect.DeepEqual(before, after) {
			t.Errorf("alias input=%t failed exact restoration: %v %+v", inputAlias, err, result)
		}
	}
}
