package features

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cucumber/godog"
)

// These engine steps execute the product binary, never an approval command or
// a product package. They retain early exits as observations for the Then step.
func (w *world) offlineRuntimeSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a CI offline checkout with the cached image and an argument-reporting probe$`, func() error { return w.offlineFixture("argv") })
	sc.Step(`^the offline argument fixture contains these exact JSON arguments$`, func(doc *godog.DocString) error {
		if err := json.Unmarshal([]byte(doc.Content), &w.offline.arguments); err != nil {
			return err
		}
		if len(w.offline.arguments) == 0 {
			return fmt.Errorf("empty argument fixture")
		}
		return nil
	})
	sc.Step(`^the product runs the offline argument probe with that argv$`, func() error {
		return w.offlineExecute(append([]string{"/workspace/slice-10-probe", "argv"}, w.offline.arguments...), false)
	})
	sc.Step(`^the worker report contains exactly those argument bytes in that order with no inserted or missing argument$`, w.offlineArguments)
	sc.Step(`^no offline shell marker exists in the checkout or host fixture$`, w.offlineNoShellMarkers)
	sc.Step(`^the product returns zero only after that worker is independently confirmed absent$`, w.offlineSuccess)

	sc.Step(`^a CI offline checkout with the cached image and a framed stdin stdout stderr probe$`, func() error { return w.offlineFixture("streams") })
	sc.Step(`^the product runs the offline stream probe and the harness answers its live handshake then closes stdin$`, func() error {
		return w.offlineExecute([]string{"/workspace/slice-10-probe", "streams"}, true)
	})
	sc.Step(`^both readiness streams arrive before the input reply and before workload exit$`, w.offlineHandshake)
	sc.Step(`^the worker receives the exact stdin frame and EOF$`, func() error {
		return w.offlineStreamBytes(true)
	})
	sc.Step(`^stdout and workload stderr retain their exact separate frames including the trailing bytes$`, func() error {
		return w.offlineStreamBytes(false)
	})
	sc.Step(`^controller diagnostics occur only on stderr and the product returns zero after confirmed cleanup$`, func() error {
		if err := w.offlineStreamBytes(false); err != nil {
			return err
		}
		return w.offlineSuccess()
	})

	sc.Step(`^a CI offline checkout with the cached image and approved workspace subdirectory and literal APP environment$`, func() error { return w.offlineFixture("shell") })
	sc.Step(`^the offline controller environment and host helper contain synthetic markers absent from approved settings$`, w.offlineHostMarkers)
	sc.Step(`^the product runs an explicitly supplied shell which prints its workspace and APP then writes a checkout file$`, func() error {
		return w.offlineExecute([]string{"sh", "-c", `pwd; printf '%s\n' "$APP"; printf '%s\n' "$HOME" "$TMPDIR"; printf '%s' worker > edit.txt; exec /workspace/slice-10-probe environment`}, false)
	})
	sc.Step(`^shell interpretation and the file write occur only inside the worker at the approved workspace subdirectory$`, w.offlineShellWorkspace)
	sc.Step(`^APP retains its exact approved bytes without expansion or an environment marker file$`, w.offlineLiteralEnvironment)
	sc.Step(`^the worker has HOME and TMPDIR in its tmpfs and none of the host-only environment markers$`, w.offlineEnvironment)
	sc.Step(`^the host helper never executes and the checkout edit survives confirmed owned cleanup$`, func() error {
		if err := w.offlineShellWorkspace(); err != nil {
			return err
		}
		if err := w.offlineNoShellMarkers(); err != nil {
			return err
		}
		return w.offlineSuccess()
	})
}

func (w *world) offlineHostMarkers() error {
	for _, key := range []string{"HOST_ONLY", "GITHUB_TOKEN", "AWS_SECRET_ACCESS_KEY", "ITOS_ROOT_MARKER"} {
		w.doctor.env[key] = "SLICE_10_SYNTHETIC_" + key
	}
	bin := filepath.Join(w.doctor.host, "slice-10-host-bin")
	if err := os.Mkdir(bin, 0700); err != nil {
		return err
	}
	// This same-basename tripwire is inert unless the product wrongly resolves
	// the supplied shell on the host. The harness never executes it.
	marker := filepath.Join(w.doctor.host, "host-helper-marker")
	script := "#!/bin/sh\nprintf host > '" + strings.ReplaceAll(marker, "'", "'\\''") + "'\nexit 97\n"
	if err := os.WriteFile(filepath.Join(bin, "sh"), []byte(script), 0700); err != nil {
		return err
	}
	w.doctor.env["PATH"] = bin + string(os.PathListSeparator) + os.Getenv("PATH")
	return w.offlineCheckpoint()
}
