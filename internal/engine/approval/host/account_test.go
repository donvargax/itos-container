package host

import (
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"testing"
)

func TestAccountHomeUsesEffectiveUIDWithoutEnvironmentFallback(t *testing.T) {
	for _, kind := range []string{"valid", "lookup failed", "nil account", "different UID", "relative home"} {
		t.Run(kind, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", filepath.Join(home, "environment"))
			t.Setenv("USER", "other")
			t.Setenv("LOGNAME", "other")
			called := ""
			ops := Process{}.operations()
			ops.geteuid = func() int { return 1234 }
			ops.lookup = func(uid string) (*user.User, error) {
				called = uid
				switch kind {
				case "lookup failed":
					return nil, errors.New("unavailable")
				case "nil account":
					return nil, nil
				case "different UID":
					return &user.User{Uid: "2345", HomeDir: home}, nil
				case "relative home":
					return &user.User{Uid: uid, HomeDir: "relative"}, nil
				default:
					return &user.User{Uid: uid, HomeDir: home}, nil
				}
			}
			got, f := (Process{ops: &ops}).AccountHome()
			if called != "1234" {
				t.Fatal("lookup did not use effective UID")
			}
			if kind == "valid" {
				if f != "" || got != home {
					t.Fatal(got, f)
				}
			} else if f != "state.unavailable" || got != "" {
				t.Fatal(got, f)
			}
		})
	}
}
func TestAccountHomeReadOnlyHostLookup(t *testing.T) {
	account, err := user.LookupId(strconv.Itoa(os.Geteuid()))
	got, f := (Process{}).AccountHome()
	if err != nil || !filepath.IsAbs(account.HomeDir) {
		if f != "state.unavailable" {
			t.Fatal(f)
		}
		return
	}
	if f != "" || got != filepath.Clean(account.HomeDir) {
		t.Fatal("effective-account lookup disagrees", f)
	}
}
