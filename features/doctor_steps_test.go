package features

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

const doctorImage = "worker.test/agent@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const doctorImageID = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
const doctorCanary = "slice-3-synthetic-private-canary"

type doctorResult struct {
	stdout, stderr   string
	exit             int
	elapsed          time.Duration
	checkout         map[string]any
	endpoint, daemon string
}

type doctorWorld struct {
	checkout, cwd, host, recorder string
	image, imageID, engineID, api string
	env                           map[string]string
	manifest                      map[string]any
	engine                        *doctorAPI
	cleanup                       []func()
	snapshot                      map[string]string
	replacement                   string
	replacementSnapshot           map[string]string
	results                       []doctorResult
	real                          *doctorReal
}

func (w *world) doctorSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a doctor checkout$`, w.doctorCheckout)
	sc.Step(`^the doctor manifest pins the image name "([^"]*)"$`, w.doctorImageName)
	sc.Step(`^the inspected image attests the digest under "([^"]*)"$`, func(name string) {
		w.doctor.engine.image["RepoDigests"] = []string{name + "@sha256:" + strings.Repeat("a", 64)}
	})
	sc.Step(`^a doctor checkout on an unsupported host$`, func() error {
		if runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
			return fmt.Errorf("unsupported-host case needs a native macOS/Windows process")
		}
		return w.doctorCheckout()
	})
	sc.Step(`^doctor checkouts "([^"]*)" and "([^"]*)"$`, w.doctorCheckouts)
	sc.Step(`^a doctor checkout changed to the layout "([^"]*)"$`, w.doctorLayout)
	sc.Step(`^a doctor checkout with "([^"]*)" replaced by a symlink to an outside fixture$`, w.doctorSymlink)
	sc.Step(`^ITOS_ROOT names that checkout while the child cwd is a different empty directory$`, func() error {
		w.doctor.env["ITOS_ROOT"] = w.doctor.checkout
		w.doctor.cwd = filepath.Join(w.dir, "empty-cwd")
		return os.Mkdir(w.doctor.cwd, 0700)
	})
	sc.Step(`^ITOS_CONFIG names a regular core configuration file with unrelated keys$`, func() error {
		path := filepath.Join(w.doctor.host, "core.yaml")
		w.doctor.env["ITOS_CONFIG"] = path
		return writeManifestFixture(path, []byte("language: go\nproject: fixture\n"), 0600)
	})
	sc.Step(`^ITOS_ROOT is "([^"]*)"$`, w.doctorRoot)
	sc.Step(`^a local Engine API fixture$`, func() error { return w.doctorLocalAPI(false) })
	sc.Step(`^a local Engine API fixture paused before its image response$`, func() error { return w.doctorLocalAPI(true) })
	sc.Step(`^a local Engine API fixture with "([^"]*)"$`, w.doctorObservation)
	sc.Step(`^a local Engine API fixture that "([^"]*)"$`, w.doctorFailure)
	sc.Step(`^a local Engine API fixture returning image-not-found for the requested digest$`, func() error {
		if err := w.doctorLocalAPI(false); err != nil {
			return err
		}
		w.doctor.engine.imageStatus = 404
		return nil
	})
	sc.Step(`^a local Engine API fixture with image RepoDigests "([^"]*)"$`, w.doctorDigests)
	sc.Step(`^a local Engine API fixture with image platform "([^"]*)"$`, w.doctorPlatform)
	sc.Step(`^a local Engine API fixture with image response "([^"]*)"$`, w.doctorImageResponse)
	sc.Step(`^a local Engine API fixture with the wrong image digest and platform linux/arm64$`, func() error {
		if err := w.doctorDigests("same repository with a different digest"); err != nil {
			return err
		}
		w.doctor.engine.image["Architecture"] = "arm64"
		return nil
	})
	sc.Step(`^a local Engine API fixture with synthetic canary text in "([^"]*)"$`, w.doctorCanaries)
	sc.Step(`^the doctor manifest requests containerUser "([^"]*)" and remoteUser "([^"]*)"$`, w.doctorUsers)
	sc.Step(`^the inspected image Config.User is "([^"]*)"$`, func(user string) { w.doctor.engine.config()["User"] = user })
	sc.Step(`^the inspected image devcontainer.metadata label is:$`, func(doc *godog.DocString) {
		w.doctor.engine.config()["Labels"] = map[string]string{"devcontainer.metadata": doc.Content}
	})
	sc.Step(`^executable marker fixtures for Git hooks, fsmonitor, credential helpers, builds and PATH tools$`, w.doctorSentinels)
	sc.Step(`^existing host approval and session records containing synthetic fixture data$`, w.doctorHostState)
	sc.Step(`^marker fixtures at Docker context and credential-helper locations$`, w.doctorCredentialSentinels)
	sc.Step(`^a connection-recording endpoint fixture selected as "([^"]*)"$`, w.doctorEndpoint)
	sc.Step(`^a connection-recording endpoint fixture$`, func() error { return w.doctorEndpoint("tcp://127.0.0.1:2375") })
	sc.Step(`^DOCKER_HOST names a remote HTTPS endpoint with synthetic canaries in userinfo and query$`, w.doctorRemoteCanary)
	sc.Step(`^the selected local socket is "([^"]*)"$`, w.doctorSocket)
	sc.Step(`^doctor inspects "([^"]*)", "([^"]*)" and "([^"]*)" again as JSON$`, w.doctorThreeRuns)
	sc.Step(`^doctor inspects as JSON before and after the fixture daemon ID changes to "([^"]*)"$`, w.doctorTwoRuns)
	sc.Step(`^doctor runs as JSON and the fixture replaces (checkout root|\.git) before releasing the response$`, func(entry string) error { return w.doctorRace(entry, false) })
	sc.Step(`^doctor runs as JSON and the fixture changes (daemon ID|socket device/inode) before releasing the response$`, func(entry string) error { return w.doctorRace(entry, true) })
	sc.Step(`^its JSON inspection identifies the checkout, selected engine and requested cached image$`, w.doctorIdentifies)
	sc.Step(`^its JSON inspection is ready without asserting approval or runtime isolation$`, w.doctorReady)
	sc.Step(`^its JSON inspection is not ready$`, w.doctorNotReady)
	sc.Step(`^its JSON output has a doctor problem "([^"]*)" with exit (\d+)$`, w.doctorProblem)
	sc.Step(`^its JSON inspection outcome is "([^"]*)" with exit (\d+)$`, w.doctorOutcome)
	sc.Step(`^its JSON output reports an inspected image metadata problem at "([^"]*)" with exit (\d+)$`, w.doctorMetadataProblem)
	sc.Step(`^its JSON effective manifest matches the checkout request without metadata merging$`, w.doctorEffective)
	sc.Step(`^its JSON output lists doctor.image.digest and doctor.image.platform in that order with exit 1 each$`, w.doctorIndependentProblems)
	sc.Step(`^its standard output names the checkout, engine identity and requested image digest$`, func() error {
		for _, value := range []string{w.doctor.checkout, w.doctor.engineID, w.doctor.image} {
			if !strings.Contains(w.stdout, value) {
				return fmt.Errorf("plain inspection lacks %q\n%s", value, w.report())
			}
		}
		return nil
	})
	sc.Step(`^the problem identifies the requested image reference$`, func() error { return w.doctorProblemSubject("image", w.doctor.image) })
	sc.Step(`^the problem identifies the field "([^"]*)"$`, func(field string) error { return w.doctorProblemSubject("field", field) })
	sc.Step(`^its fix asks the host operator to prepare the exact image outside doctor$`, w.doctorPrepareFix)
	sc.Step(`^the three JSON checkout identities match their filesystem identities$`, w.doctorThreeIdentities)
	sc.Step(`^the first and second checkout identities differ$`, func() error { return w.doctorCompareCheckouts(0, 1, false) })
	sc.Step(`^the first and third checkout identities are equal$`, func() error { return w.doctorCompareCheckouts(0, 2, true) })
	sc.Step(`^all three inspections exit with code 0$`, func() error { return w.doctorAllExits(3) })
	sc.Step(`^both inspections exit with code 0$`, func() error { return w.doctorAllExits(2) })
	sc.Step(`^both JSON inspections report their observed endpoint and daemon ID$`, w.doctorTwoIdentities)
	sc.Step(`^the engine endpoints are equal and the daemon IDs differ$`, w.doctorCompareEngines)
	sc.Step(`^the Engine API journal contains only ping, version, info and the requested image inspection$`, w.doctorJournal)
	sc.Step(`^the Engine API journal is empty$`, w.doctorEmptyJournal)
	sc.Step(`^no cached image inspection or engine mutation was attempted$`, func() error { return w.doctorAllowedJournal(false) })
	sc.Step(`^no engine mutation or image acquisition was attempted$`, func() error { return w.doctorAllowedJournal(true) })
	sc.Step(`^(?:the endpoint fixture received no connection|no endpoint connection was attempted|no redirect destination received a connection)$`, w.doctorNoConnection)
	sc.Step(`^no executable marker was created$`, w.doctorNoMarker)
	sc.Step(`^the checkout and host control state are unchanged$`, w.doctorUnchanged)
	sc.Step(`^the outside fixture is unchanged$`, w.doctorUnchanged)
	sc.Step(`^the replacement tree is unchanged$`, w.doctorReplacementUnchanged)
	sc.Step(`^neither output stream nor any changed file contains the synthetic canary$`, w.doctorNoCanary)
	sc.Step(`^the command finishes within (\d+) seconds$`, func(seconds int) error {
		if len(w.doctor.results) != 1 || w.timedOut || w.doctor.results[0].elapsed > time.Duration(seconds)*time.Second {
			return fmt.Errorf("doctor exceeded %d seconds", seconds)
		}
		return nil
	})
	w.doctorRuntimeSteps(sc)
}
