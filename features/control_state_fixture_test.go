package features

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
)

// Each project has its own selected state and child HOME. Only the synthetic
// effective account home is common. No Given starts a product command.
type controlFixture struct {
	parent, home              string
	p                         *world
	workers                   *overlapFixture
	before                    map[string]string
	results                   []doctorResult
	err                       error
	condition                 string
	reviewBefore, reviewAfter map[string]string
	traces                    []*recoveryProcess
	surviving                 map[string]sessionRecord
	bootstrapBefore           map[string]string
}

func (w *world) controlSetup(approval, location, engines string) error {
	base := filepath.Join(w.dir, "slice-21")
	f := &controlFixture{parent: base, home: filepath.Join(base, "account-home")}
	w.control = f
	f.workers = &overlapFixture{worlds: map[string]*world{}, engines: map[string]*overlapEngine{}, firstName: "W"}
	w.overlap = f.workers // reuse identity-checked subprocess/API fixture cleanup
	if err := os.MkdirAll(f.home, 0700); err != nil {
		return err
	}
	bin, err := w.sessionProbeBinary()
	if err != nil {
		return err
	}
	names := []string{"W", "P"}
	if location == "first approval" {
		names = []string{"P"}
	}
	for _, name := range names {
		p := &world{root: w.root, bin: bin, dir: filepath.Join(base, "fixtures", name)}
		f.workers.worlds[name] = p
		if err := os.MkdirAll(p.dir, 0700); err != nil {
			return err
		}
		if err := p.doctorCheckout(); err != nil {
			return err
		}
		p.approval = &approvalWorld{uid: strconv.Itoa(os.Geteuid()), gid: strconv.Itoa(os.Getegid())}
		p.doctor.engine = newDoctorAPI(p.doctor.image)
		if err := p.doctorSentinels(); err != nil {
			return err
		}
		root := filepath.Join(base, "projects", name, "itos-container")
		if err := os.MkdirAll(filepath.Dir(root), 0700); err != nil {
			return err
		}
		if err := os.Rename(p.doctor.checkout, root); err != nil {
			return err
		}
		p.doctor.checkout, p.doctor.cwd, p.doctor.env["ITOS_ROOT"] = root, root, root
		p.session, p.recovery, p.recoveryLocal = &sessionWorld{}, &recoveryNative{mode: "overlap"}, &recoveryLocal{}
		a := f.workers.engines["W"]
		if a == nil || engines == "distinct daemons" {
			var err error
			a, err = newOverlapEngineAt(p, "slice-21-daemon-"+name, filepath.Join(base, name+".sock"))
			if err != nil {
				return err
			}
		}
		f.workers.engines[name] = a
		p.doctor.env["DOCKER_HOST"] = a.endpoint
		p.doctor.env["SLICE_21_ACCOUNT_HOME"] = f.home
		p.doctor.engine.info["ID"] = a.metadata.info["ID"]
		p.doctor.engineID = fmt.Sprint(a.metadata.info["ID"])
		if name == "W" {
			if err := p.approvalSeed("1", false); err != nil {
				return err
			}
			if err := p.sessionSeedSnapshot(); err != nil {
				return err
			}
			continue
		}
		f.p = p
		if location == "first approval" {
			p.doctor.env["HOME"] = f.home
			delete(p.doctor.env, "XDG_STATE_HOME")
			return p.approvalOracle()
		}
		worker := f.workers.worlds["W"].doctor.checkout
		switch location {
		case "equal to W root":
			p.doctor.env["XDG_STATE_HOME"] = filepath.Dir(worker)
		case "below W root":
			p.doctor.env["XDG_STATE_HOME"] = filepath.Join(worker, "selected-state")
		default:
			return fmt.Errorf("unknown control-state location %q", location)
		}
		if err := p.approvalOracle(); err != nil {
			return err
		}
		if approval != "absent" {
			if approval != "matching" && approval != "changed" {
				return fmt.Errorf("unknown control-state approval %q", approval)
			}
			if err := p.approvalSeed("4", approval == "changed"); err != nil {
				return err
			}
		}
	}
	return nil
}

func (f *controlFixture) startWorker() {
	f.workers.first = f.workers.start("W", "", "")
	f.workers.observe(f.workers.first, false)
	f.workers.checkpoint(f.workers.first)
}

func (f *controlFixture) inspect(command, format string) {
	var err error
	f.before, err = f.inventory()
	f.err = errors.Join(f.err, err)
	args := []string{command}
	if command == "approve" {
		args = append(args, "--fingerprint", f.p.approval.fingerprint)
	}
	if format == "JSON" {
		args = append(args, "--json")
	}
	f.err = errors.Join(f.err, f.p.runDoctor(f.p.env(), args...))
	f.results = append(f.results, doctorResult{exit: f.p.exit, stdout: f.p.stdout, stderr: f.p.stderr})
}

func (f *controlFixture) exposed() error {
	if err := errors.Join(f.err, f.workers.live(f.workers.first)); err != nil {
		return err
	}
	if err := f.p.approvalProblem("approval.state.exposed", 1, ""); err != nil {
		return err
	}
	return f.p.approvalCandidate()
}

// Compare every entry, including ownership, links, file identity and mtime.
// The API fixture does not write worker heartbeat files. Its process journals
// and stream captures live in the fixture directory, outside these inventories.
func (f *controlFixture) inventory() (map[string]string, error) {
	result := map[string]string{}
	roots := []string{f.home, filepath.Join(f.parent, "projects")}
	for _, p := range f.workers.worlds {
		roots = append(roots, p.doctor.host)
	}
	for _, root := range roots {
		tree, err := doctorTree(root)
		if err != nil {
			return nil, err
		}
		for path, value := range tree {
			info, err := os.Lstat(path)
			if err != nil {
				return nil, err
			}
			stat := reflect.Indirect(reflect.ValueOf(info.Sys()))
			value += fmt.Sprintf(":%v:%v:%v", stat.FieldByName("Dev"), stat.FieldByName("Ino"), stat.FieldByName("Nlink"))
			if !info.IsDir() {
				value += ":" + info.ModTime().String()
			}
			result[path] = value
		}
	}
	return result, nil
}

func (f *controlFixture) unchanged() error {
	now, err := f.inventory()
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(now, f.before) {
		for path, value := range now {
			if f.before[path] != value {
				return fmt.Errorf("control-state command changed %s", path)
			}
		}
		return errors.New("control-state command deleted an entry")
	}
	return nil
}

func (f *controlFixture) inspectionOnly() error {
	workerPID := -1
	if f.workers.first != nil {
		workerPID = f.workers.first.pid()
	}
	seen := map[*overlapEngine]bool{}
	for _, a := range f.workers.engines {
		if seen[a] {
			continue
		}
		seen[a] = true
		for _, q := range a.journal() {
			if q.pid != workerPID && q.method != "GET" && q.method != "HEAD" {
				return fmt.Errorf("inspection mutated an engine: %s %s", q.method, q.path)
			}
		}
	}
	for _, p := range f.workers.worlds {
		entries, err := os.ReadDir(p.doctor.recorder)
		if err != nil || len(entries) != 0 {
			return errors.New("checkout helper executed or recorder unavailable")
		}
	}
	return nil
}

func (f *controlFixture) safeDoctor() {
	w := f.workers.worlds["W"]
	f.p.doctor.env["XDG_STATE_HOME"] = w.doctor.checkout + "-more"
	f.err = errors.Join(f.err, f.p.approvalOracle())
	f.inspect("doctor", "JSON")
}

func (f *controlFixture) missing() error {
	if err := errors.Join(f.err, f.p.exitsWith(0), f.p.approvalStatus("missing"), f.unchanged()); err != nil {
		return err
	}
	for path := range f.before {
		if strings.HasPrefix(path, filepath.Join(f.p.doctor.env["XDG_STATE_HOME"], "itos-container")+string(filepath.Separator)) {
			return errors.New("safe control was not a missing selected tree")
		}
	}
	return f.workers.live(f.workers.first)
}
