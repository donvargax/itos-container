package features

import (
	"fmt"
	"github.com/cucumber/godog"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
)

func (w *world) runFixture() *runWorld {
	if w.run == nil {
		w.run = &runWorld{}
	}
	return w.run
}
func (w *world) runPreflightSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a local owned foreground Engine API fixture$`, w.localOfflineFixture)
	sc.Step(`^run completes the inspected owned foreground worker with exit zero$`, w.localOfflineSuccess)
	sc.Step(`^the local foreground journal contains only inspections and owned worker operations$`, w.localOfflineOwned)
	sc.Step(`^only protected snapshot intent and public session records were added without chown or checkout changes$`, func() error { return w.runSameTree(true) })
	sc.Step(`^the first run completed owned execution and the second made no startup effects$`, w.localOfflineFirstRun)
	sc.Step(`^run preflight has local defect "([^"]*)"$`, w.runLocalDefect)
	sc.Step(`^run preflight has local approval condition "([^"]*)"$`, w.runApprovalCondition)
	sc.Step(`^run preflight has engine observation "([^"]*)"$`, w.runEngineObservation)
	sc.Step(`^the run preflight files are snapshotted$`, w.runCheckpoint)
	sc.Step(`^run reports preflight problem "([^"]*)" with exit (\d+) and subject "([^"]*)"$`, w.runProblem)
	sc.Step(`^run preflight has changed no checkout or host state(?: apart from the fixture replacement)?$`, func() error { return w.runSameTree(false) })
	sc.Step(`^the run Engine API journal contains inspection requests only$`, func() error {
		return w.runJournal(w.doctor.engine.requests(), "inspection only", w.runFixture().imageObservation)
	})
	sc.Step(`^run preflight pauses the Engine API before its image response$`, func() { w.runFixture(); w.doctor.engine.paused = true })
	sc.Step(`^run starts and the fixture replaces "([^"]*)" before releasing the image response$`, w.runRace)
	sc.Step(`^the run manifest users are "([^"]*)"$`, func(users string) error {
		w.runFixture()
		if users == "omitted" {
			return w.doctorUsers("omitted", "omitted")
		}
		if users != "effective host UID" {
			return fmt.Errorf("unknown users %q", users)
		}
		id := strconv.Itoa(os.Geteuid())
		return w.doctorUsers(id, id)
	})
	sc.Step(`^the run input spelling is "([^"]*)"$`, func(spelling string) error {
		edits := map[string]string{"unchanged": "", "reordered keys with comments and whitespace": "reorder YAML and JSON keys and add comments and whitespace", "explicit default workspace and empty environment": "spell default workspace and empty environment explicitly", "omitted manifest users": "omit both manifest users that equal the effective host UID", "equivalent JSON string escaping": "change JSON string escaping without changing decoded values"}
		edit, ok := edits[spelling]
		if !ok {
			return fmt.Errorf("unknown spelling %q", spelling)
		}
		if edit == "" {
			return nil
		}
		return w.approvalEdit(edit)
	})
	sc.Step(`^the run child has umask "([^"]*)"$`, func(mask string) error {
		if mask != "0000" && mask != "0777" {
			return fmt.Errorf("invalid fixture mask")
		}
		w.runFixture().mask = mask
		return nil
	})
	sc.Step(`^the protected run snapshot is exactly the independently calculated approval at generation "([^"]*)"$`, w.runSnapshot)
	sc.Step(`^the snapshot resolves both manifest users to the effective UID and binds the effective GID$`, func() error { return w.runSnapshot(w.approval.generation) })
	sc.Step(`^only the protected run snapshot subtree was added without chown or checkout changes$`, func() error { return w.runSameTree(true) })
	sc.Step(`^run is invoked, the fixture changes "([^"]*)", and run is invoked again$`, w.runTwice)
	sc.Step(`^the first run published the exact protected snapshot for generation "([^"]*)"$`, func(g string) error { return w.runObservedSnapshot(0, g) })
	sc.Step(`^the second run reports "([^"]*)" with exit (\d+) and journal "([^"]*)"$`, w.runSecondProblem)
	sc.Step(`^the first snapshot and active approval were not rewritten by either run$`, w.runUnchangedRecords)
	sc.Step(`^the second run published no snapshot and made no startup handoff$`, func() error { return w.runSameTree(false) })
	sc.Step(`^neither run attempted engine mutation, image acquisition or helper execution$`, w.runNoEffects)
	sc.Step(`^the run snapshot path has "([^"]*)"$`, w.runSnapshotDefect)
	sc.Step(`^two runs for the same approval execute "([^"]*)" and a third run follows$`, w.runThree)
	sc.Step(`^all three runs observe the same exact protected snapshot for generation "([^"]*)"$`, func(g string) error {
		for i := 0; i < 3; i++ {
			if err := w.runObservedSnapshot(i, g); err != nil {
				return err
			}
		}
		return nil
	})
	sc.Step(`^all three complete owned foreground execution with exit zero and empty stdout$`, w.runThreeCompletions)
	sc.Step(`^each run's Engine API journal contains fresh inspection and only its owned worker operations$`, w.runThreeJournals)
	sc.Step(`^the final snapshot is never partial and its inode and mtime remain unchanged after publication$`, w.runSnapshotStable)
	sc.Step(`^the policy inspection files differ only by protected snapshot intent and public session records$`, func() error {
		got, err := w.policyFiles()
		if err != nil {
			return err
		}
		tree, err := runTree(w.dir)
		if err != nil {
			return err
		}
		registry, err := w.localRegistryPaths(tree)
		if err != nil {
			return err
		}
		prefix := filepath.Dir(filepath.Dir(w.runSnapshotPath()))
		for path := range got {
			if path == prefix || strings.HasPrefix(path, prefix+string(filepath.Separator)) || w.localIntentPath(path) || registry[path] {
				delete(got, path)
			}
		}
		if !reflect.DeepEqual(got, w.policy.snapshot) {
			return fmt.Errorf("unrelated policy fixture changed")
		}
		return nil
	})
	w.runContractSteps(sc)
}
func (w *world) runLocalDefect(defect string) error {
	w.runFixture()
	d := w.doctor
	path := filepath.Join(d.checkout, ".devcontainer/devcontainer.json")
	switch defect {
	case "policy is missing":
		return os.Remove(filepath.Join(d.checkout, "itos-container.yaml"))
	case "policy requests network mode online":
		return os.WriteFile(filepath.Join(d.checkout, "itos-container.yaml"), []byte("version: 1\nnetwork: {mode: online}\n"), 0600)
	case "selected manifest is missing":
		return os.Remove(path)
	case "manifest requests initializeCommand":
		d.manifest["initializeCommand"] = "./forbidden"
	case "manifest has a duplicate image key":
		return os.WriteFile(path, append([]byte(`{"image":"`+doctorImage+`",`), approvalJSON(d.manifest)[1:]...), 0600)
	case "containerUser differs from effective UID":
		d.manifest["containerUser"] = strconv.Itoa(os.Geteuid() + 1)
	case "remoteUser differs from effective UID":
		d.manifest["remoteUser"] = strconv.Itoa(os.Geteuid() + 1)
	case "embedded .git is a symlink outside checkout":
		target := filepath.Join(w.dir, "outside-git")
		if err := os.Rename(filepath.Join(d.checkout, ".git"), target); err != nil {
			return err
		}
		return os.Symlink(target, filepath.Join(d.checkout, ".git"))
	default:
		return fmt.Errorf("unknown local defect %q", defect)
	}
	return w.approvalManifest()
}
func (w *world) runApprovalCondition(condition string) error {
	w.runFixture()
	switch condition {
	case "no host approval":
		return nil
	case "approval only inside the checkout":
		return w.approvalState("matching only in the checkout")
	case "truncated active approval":
		return w.approvalDefect("truncated JSON")
	case "historical A with active B":
		if err := w.approvalSeed("1", false); err != nil {
			return err
		}
		data := append([]byte{}, w.approval.recordBefore...)
		if err := w.approvalSeed("2", true); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(filepath.Dir(w.approval.record), "historical.json"), data, 0600)
	case "active approval has mode 0644":
		return w.approvalUnsafe("current.json mode is 0644")
	case "manifest environment differs from A", "manifest image differs from A":
		if err := w.approvalSeed("1", false); err != nil {
			return err
		}
		if condition == "manifest environment differs from A" {
			return w.approvalEdit("change the literal application environment")
		}
		return w.runChangeImage()
	default:
		return fmt.Errorf("unknown approval condition %q", condition)
	}
}
func (w *world) runChangeImage() error {
	w.doctor.manifest["image"] = "worker.test/agent@sha256:" + strings.Repeat("c", 64)
	return w.approvalManifest()
}
func (w *world) runEngineObservation(observation string) error {
	r := w.runFixture()
	a := w.doctor.engine
	switch observation {
	case "cached image reports another config digest":
		a.image["Id"] = "sha256:" + strings.Repeat("c", 64)
		r.imageObservation = true
	case "missing daemon ID":
		a.info["ID"] = ""
	case "unsupported engine product":
		a.version["Platform"] = map[string]string{"Name": "Podman Engine"}
	case "engine reports linux arm64":
		a.version["Arch"] = "arm64"
		a.info["Architecture"] = "aarch64"
	case "requested image returns not found":
		a.imageStatus = 404
		r.imageObservation = true
	case "image attests a different digest":
		a.image["RepoDigests"] = []string{"worker.test/agent@sha256:" + strings.Repeat("c", 64)}
		r.imageObservation = true
	case "image reports linux arm64":
		a.image["Architecture"] = "arm64"
		r.imageObservation = true
	case "image response has no Config":
		delete(a.image, "Config")
		r.imageObservation = true
	case "metadata requests containerUser":
		a.config()["Labels"] = map[string]string{"devcontainer.metadata": `[{"containerUser":"1000"}]`}
		r.imageObservation = true
	default:
		return fmt.Errorf("unknown observation %q", observation)
	}
	return nil
}
