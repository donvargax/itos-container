package features

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/cucumber/godog"
)

// A device number is not stable across boots, so a binding written before a
// renumbering must still be trusted by path and inode. The fixture rewrites only
// the recorded device of every ancestor; no inode, owner or mode changes.
func (w *world) controlDeviceSteps(sc *godog.ScenarioContext) {
	var renumbered map[string][]byte
	sc.Step(`^a control-state fixture whose "([^"]*)" record device numbers the live filesystem no longer uses$`, func(which string) error {
		var err error
		renumbered, err = w.controlRenumbered(which)
		return err
	})
	sc.Step(`^fresh P doctor and exact-fingerprint approve inspect the renumbered catalog as JSON$`, func() {
		f := w.control
		f.results = nil
		f.inspect("doctor", "JSON")
		f.err = errors.Join(f.err, f.unchanged())
		f.inspect("approve", "JSON")
	})
	sc.Step(`^both commands report matching approval with exit 0 and no prompt$`, func() error {
		f := w.control
		if f.err != nil {
			return f.err
		}
		if len(f.results) != 2 {
			return errors.New("both renumbered inspections were not captured")
		}
		for _, r := range f.results {
			f.p.exit, f.p.stdout, f.p.stderr = r.exit, r.stdout, r.stderr
			if err := f.p.approvalSuccess(); err != nil {
				return err
			}
		}
		return nil
	})
	sc.Step(`^the renumbered bindings keep their bytes and the prior approval is unchanged without engine mutation$`, func() error {
		f := w.control
		for path, want := range renumbered {
			got, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(got, want) {
				return fmt.Errorf("renumbered binding %s was rewritten or lost", path)
			}
		}
		return errors.Join(f.p.approvalRecordUnchanged(), f.inspectionOnly())
	})
}

func (w *world) controlRenumbered(which string) (map[string][]byte, error) {
	if err := w.controlSetup("absent", "below W root", "distinct daemons"); err != nil {
		return nil, err
	}
	f := w.control
	f.p.doctor.env["XDG_STATE_HOME"] = filepath.Join(f.p.doctor.host, "state")
	if err := f.p.approvalSeed("4", false); err != nil {
		return nil, err
	}
	self, err := f.seedBinding(f.fixedRoot())
	if err != nil {
		return nil, err
	}
	location, err := f.seedBinding(filepath.Join(f.workers.worlds["W"].doctor.env["XDG_STATE_HOME"], "itos-container"))
	if err != nil {
		return nil, err
	}
	targets, ok := map[string][]string{
		"fixed tree self-binding":     {self},
		"enrolled location binding":   {location},
		"fixed and enrolled bindings": {self, location},
	}[which]
	if !ok {
		return nil, fmt.Errorf("unknown renumbered binding %q", which)
	}
	written := map[string][]byte{}
	for _, path := range targets {
		data, err := controlRenumber(path)
		if err != nil {
			return nil, err
		}
		written[path] = data
	}
	return written, nil
}

// Rewrite in place so the binding keeps its inode, owner and mode.
func controlRenumber(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var record map[string]any
	if err = json.Unmarshal(data, &record); err != nil {
		return nil, err
	}
	ancestors, _ := record["ancestors"].([]any)
	if len(ancestors) == 0 {
		return nil, fmt.Errorf("binding %s records no ancestors", path)
	}
	for _, ancestor := range ancestors {
		entry, _ := ancestor.(map[string]any)
		device, err := strconv.ParseUint(fmt.Sprint(entry["device"]), 10, 64)
		if err != nil {
			return nil, err
		}
		entry["device"] = strconv.FormatUint(device+7, 10)
	}
	data = approvalJSON(record)
	return data, os.WriteFile(path, data, 0600)
}
