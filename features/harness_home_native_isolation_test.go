package features

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"

	"github.com/cucumber/godog"
)

func (w *world) homeNativeIsolationSteps(sc *godog.ScenarioContext) {
	sc.Step(`^two CI home checkouts with equal basenames and the same approved image and host UID GID$`, w.homeNativePairFixture)
	sc.Step(`^their product workers write distinct private home markers and each checkout is run again$`, func() error {
		for i, project := range w.homeNativeProjects() {
			if run := project.homeNativeCompleted("write", homeNativeMarker(i), homeNativeMarker(1-i)); run.err != nil {
				return run.err
			}
		}
		for i, project := range w.homeNativeProjects() {
			if run := project.homeNativeCompleted("read", homeNativeMarker(i), homeNativeMarker(1-i)); run.err != nil {
				return run.err
			}
		}
		return nil
	})
	sc.Step(`^each later worker reads only its own retained home marker from a different owned volume$`, w.homeNativeSeparated)
	sc.Step(`^neither worker can read the other home or the synthetic host-home sentinel and a same-home read control succeeds$`, func() error {
		for _, project := range w.homeNativeProjects() {
			for _, run := range project.home.native.runs {
				if run.report.OtherReadable || run.report.HostReadable || !run.report.OwnReadable {
					return errors.New("home separation failed its negative read or same-home positive control")
				}
			}
			if err := project.homeNativeMounts(); err != nil {
				return err
			}
		}
		return nil
	})
	sc.Step(`^cleanup preserves both checkout edits and both homes while removing both sessions$`, w.homeNativePairPreserved)
}

func homeNativeMarker(i int) string {
	if i == 0 {
		return "alpha"
	}
	return "beta"
}

func (w *world) homeNativePairFixture() error {
	if err := w.homeNativeFixture(); err != nil {
		return err
	}
	child := &world{root: w.root, bin: w.bin, coverdir: w.coverdir, dir: filepath.Join(w.dir, "slice-34-other")}
	w.home.native.second = child
	if err := os.Mkdir(child.dir, 0700); err != nil {
		return err
	}
	if err := child.homeNativeFixture(); err != nil {
		return err
	}
	// Same account catalog, same daemon and same numeric owner: only the full
	// checkout identity may separate these equal-basename homes.
	child.doctor.env["SLICE_21_ACCOUNT_HOME"] = w.doctor.env["SLICE_21_ACCOUNT_HOME"]
	if err := child.seedControlLocations(); err != nil {
		return err
	}
	if filepath.Base(w.doctor.checkout) != filepath.Base(child.doctor.checkout) || w.approval.project == child.approval.project ||
		w.doctor.imageID != child.doctor.imageID || w.approval.uid != child.approval.uid || w.approval.gid != child.approval.gid {
		return errors.New("native pair did not establish two equal-basename checkouts with the same image and owner")
	}
	return nil
}

func (w *world) homeNativeProjects() []*world {
	projects := []*world{w}
	if w.home.native.second != nil {
		projects = append(projects, w.home.native.second)
	}
	return projects
}

func homeNativeProbeValid(run homeNativeRun, uid, gid string, code int) error {
	if run.err != nil {
		return run.err
	}
	r := run.report
	if run.exit != code || r.Home != "/home/itos" || r.Tmp != "/tmp" || !r.Writable || r.Mode != "drwx------" ||
		strconv.Itoa(r.UID) != uid || strconv.Itoa(r.GID) != gid || r.RootUID != r.UID || r.RootGID != r.GID || len(r.Errors) != 0 {
		return fmt.Errorf("native run did not provide the approved private home: exit %d, report %s, diagnostic %s", run.exit, approvalJSON(r), run.errout)
	}
	return nil
}

func homeNativeRetained(runs []homeNativeRun, marker, uid, gid string) error {
	if len(runs) != 2 {
		return errors.New("expected a write and a later independently launched read")
	}
	for i, run := range runs {
		if err := homeNativeProbeValid(run, uid, gid, 0); err != nil {
			return err
		}
		if (i == 0 && (run.report.HomeBefore || len(run.report.Initial) != 0)) ||
			(i == 1 && (!run.report.HomeBefore || run.report.Marker != marker)) {
			return fmt.Errorf("run %d did not retain only its own marker %q", i+1, marker)
		}
	}
	if !homeNativeSameVolume(runs[0].home, runs[1].home) {
		return errors.New("later run used a different home generation")
	}
	return nil
}

func (w *world) homeNativeSeparated() error {
	projects := w.homeNativeProjects()
	if len(projects) != 2 {
		return errors.New("missing equal-basename isolation control")
	}
	for i, project := range projects {
		if err := homeNativeRetained(project.home.native.runs, homeNativeMarker(i), project.approval.uid, project.approval.gid); err != nil {
			return err
		}
	}
	first, second := projects[0].home.native.runs[0].home, projects[1].home.native.runs[0].home
	if first["Name"] == second["Name"] || object(first["Labels"])["io.itos-container.scope"] == object(second["Labels"])["io.itos-container.scope"] {
		return errors.New("equal-basename checkouts shared home storage or scope")
	}
	return nil
}

func (w *world) homeNativeCheckoutPreserved(marker string) error {
	f := w.home.native
	after, err := runTree(w.doctor.checkout)
	if err != nil {
		return err
	}
	edit := filepath.Join(w.doctor.checkout, "slice-34-worker-edit")
	if entry, ok := after[edit]; !ok || entry.Data != marker || entry.Mode.Perm() != 0600 || fmt.Sprint(entry.Owner) != w.approval.uid {
		return errors.New("product cleanup lost the owned worker's checkout edit")
	}
	delete(after, edit)
	// Adding the one specified edit changes its parent directory's mtime only.
	root := after[w.doctor.checkout]
	root.Modified = f.checkoutBefore[w.doctor.checkout].Modified
	after[w.doctor.checkout] = root
	if !reflect.DeepEqual(after, f.checkoutBefore) {
		return errors.New("home run changed other checkout data or ownership")
	}
	host, err := runTree(f.sentinels)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(host, f.hostBefore) {
		return errors.New("home run changed host-home sentinels")
	}
	return nil
}

func (w *world) homeNativePairPreserved() error {
	homes := map[string]map[string]any{}
	for _, project := range w.homeNativeProjects() {
		for name, volume := range project.home.native.volumes {
			homes[name] = volume
		}
	}
	for i, project := range w.homeNativeProjects() {
		if err := project.homeNativeCheckoutPreserved(homeNativeMarker(i)); err != nil {
			return err
		}
		if err := project.homeNativeResourcesWith(homes); err != nil {
			return err
		}
	}
	return nil
}
