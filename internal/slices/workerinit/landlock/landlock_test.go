package landlock

import (
	"errors"
	"slices"
	"syscall"
	"testing"

	"github.com/donvargax/itos-container/internal/slices/workerinit/port"
	"github.com/donvargax/itos-container/internal/slices/workerinit/port/porttest"
)

var (
	errCreate  = errors.New("create failed")
	errNoNew   = errors.New("no_new_privs failed")
	errRestr   = errors.New("restrict failed")
	errClose   = errors.New("close failed")
	errVersion = errors.New("version failed")
)

// recording is a set of raw calls that records what it was given, and fails
// the calls named in fail with the sentinel of that call.
type recording struct {
	log  []string
	fail map[string]error

	attr         rulesetAttr
	createFlags  uint
	option       int
	args         []uintptr
	restrictFD   int
	restrictFlag uint
	closeFD      int
	abi          int
}

const fakeFD = 7

func (r *recording) calls() calls {
	return calls{
		handledFS:  0xA5000,
		noNewPrivs: 77,
		version: func() (int, error) {
			r.log = append(r.log, "version")
			return r.abi, r.fail["version"]
		},
		create: func(attr rulesetAttr, flags uint) (int, error) {
			r.log = append(r.log, "create")
			r.attr, r.createFlags = attr, flags
			return fakeFD, r.fail["create"]
		},
		prctl: func(option int, a2, a3, a4, a5 uintptr) error {
			r.log = append(r.log, "prctl")
			r.option, r.args = option, []uintptr{a2, a3, a4, a5}
			return r.fail["prctl"]
		},
		restrict: func(fd int, flags uint) error {
			r.log = append(r.log, "restrict")
			r.restrictFD, r.restrictFlag = fd, flags
			return r.fail["restrict"]
		},
		close: func(fd int) error {
			r.log = append(r.log, "close")
			r.closeFD = fd
			return r.fail["close"]
		},
	}
}

func TestLandlockContract(t *testing.T) {
	t.Run("fake", func(t *testing.T) {
		for _, c := range []struct {
			name string
			l    porttest.Landlock
			want porttest.LandlockWant
		}{
			{"ABI 10", porttest.Landlock{Version: 10}, porttest.LandlockWant{ABI: 10}},
			{"ABI 9", porttest.Landlock{Version: 9}, porttest.LandlockWant{ABI: 9}},
			{"ABI 8", porttest.Landlock{Version: 8}, porttest.LandlockWant{ABI: 8}},
			{"unavailable", porttest.Landlock{VersionErr: port.ErrUnavailable}, porttest.LandlockWant{Unavailable: true}},
		} {
			t.Run(c.name, func(t *testing.T) {
				c.l.Rec = &porttest.Recorder{}
				porttest.LandlockContract(t, func() (port.Landlock, porttest.LandlockWant) { return &c.l, c.want })
			})
		}
	})

	t.Run("raw call sequence", func(t *testing.T) {
		rows := []struct {
			name string
			fail map[string]error
			log  []string
			want error
		}{
			{"success", nil, []string{"create", "prctl", "restrict", "close"}, nil},
			{"create fails", map[string]error{"create": errCreate}, []string{"create"}, errCreate},
			{"no_new_privs fails", map[string]error{"prctl": errNoNew}, []string{"create", "prctl", "close"}, errNoNew},
			{"restrict fails", map[string]error{"restrict": errRestr}, []string{"create", "prctl", "restrict", "close"}, errRestr},
			{"no_new_privs and cleanup fail", map[string]error{"prctl": errNoNew, "close": errClose}, []string{"create", "prctl", "close"}, errNoNew},
			{"restrict and cleanup fail", map[string]error{"restrict": errRestr, "close": errClose}, []string{"create", "prctl", "restrict", "close"}, errRestr},
			{"close after restrict fails", map[string]error{"close": errClose}, []string{"create", "prctl", "restrict", "close"}, errClose},
		}
		for _, row := range rows {
			t.Run(row.name, func(t *testing.T) {
				r := &recording{fail: row.fail}
				err := (&Adapter{calls: r.calls()}).Restrict()
				if !slices.Equal(r.log, row.log) {
					t.Errorf("calls = %v; want %v", r.log, row.log)
				}
				if row.want == nil && err != nil {
					t.Errorf("Restrict() = %v; want nil", err)
				}
				if row.want != nil && !errors.Is(err, row.want) {
					t.Errorf("Restrict() = %v; want %v", err, row.want)
				}
				if errors.Is(err, errClose) != (row.want == errClose) {
					t.Errorf("Restrict() = %v; the close error must be returned only when nothing failed before it", err)
				}
				if r.attr != (rulesetAttr{HandledAccessFS: 0xA5000}) || r.createFlags != 0 {
					t.Errorf("create got %+v, flags %d; want only handled_access_fs 0xA5000 and flags 0", r.attr, r.createFlags)
				}
				if slices.Contains(r.log, "prctl") && (r.option != 77 || !slices.Equal(r.args, []uintptr{1, 0, 0, 0})) {
					t.Errorf("prctl got %d %v; want 77 [1 0 0 0]", r.option, r.args)
				}
				if slices.Contains(r.log, "restrict") && (r.restrictFD != fakeFD || r.restrictFlag != 0) {
					t.Errorf("restrict got fd %d flags %d; want %d, 0", r.restrictFD, r.restrictFlag, fakeFD)
				}
				if slices.Contains(r.log, "close") && r.closeFD != fakeFD {
					t.Errorf("close got fd %d; want %d", r.closeFD, fakeFD)
				}
			})
		}
	})

	t.Run("ABI classification", func(t *testing.T) {
		for _, c := range []struct {
			name        string
			err         error
			unavailable bool
		}{
			{"ENOSYS", syscall.ENOSYS, true},
			{"EOPNOTSUPP", syscall.EOPNOTSUPP, true},
			{"other errno", syscall.EINVAL, false},
			{"other error", errVersion, false},
		} {
			r := &recording{abi: 10, fail: map[string]error{"version": c.err}}
			abi, err := (&Adapter{calls: r.calls()}).ABI()
			if !errors.Is(err, c.err) || errors.Is(err, port.ErrUnavailable) != c.unavailable || abi != 0 {
				t.Errorf("%s: ABI() = %d, %v; want 0 and %v, unavailable %v", c.name, abi, err, c.err, c.unavailable)
			}
			r = &recording{fail: map[string]error{"create": c.err}}
			if err := (&Adapter{calls: r.calls()}).Restrict(); !errors.Is(err, c.err) || errors.Is(err, port.ErrUnavailable) != c.unavailable {
				t.Errorf("%s: Restrict() = %v; want %v, unavailable %v", c.name, err, c.err, c.unavailable)
			}
		}
		r := &recording{abi: 10}
		if abi, err := (&Adapter{calls: r.calls()}).ABI(); abi != 10 || err != nil {
			t.Errorf("ABI() = %d, %v; want 10, nil", abi, err)
		}
	})

	t.Run("real", testRealAdapter)
}
