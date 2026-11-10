package features

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/cucumber/godog"
)

type credentialGrant struct {
	Kind   string `json:"kind"`
	Source string `json:"source"`
	Target string `json:"target,omitempty"`
}

type credentialWorld struct {
	state, sources string
	paths          map[string]string
	grants         []credentialGrant
	snapshot       map[string]string
	cleanup        []func()
	markers        []string
	watch          *credentialWatch
	observation    *credentialObservation
}

func (w *world) credentialSteps(sc *godog.ScenarioContext) {
	w.credentialObserverSteps(sc)
	w.credentialObservationSteps(sc)
	w.credentialReviewSteps(sc)
	sc.Step(`^a private credential inspection fixture$`, w.credentialFixture)
	sc.Step(`^credential source "([^"]*)" has layout "([^"]*)"$`, w.credentialLayout)
	sc.Step(`^credential source "([^"]*)" is a regular file of (\d+) bytes with mode "([^"]*)"$`, w.credentialFile)
	sc.Step(`^the credential fixture requests these sources:$`, func(table *godog.Table) error {
		w.credentials.grants = nil
		for _, row := range table.Rows[1:] {
			path, ok := w.credentials.paths[row.Cells[1].Value]
			if !ok {
				return fmt.Errorf("unknown fixture source %q", row.Cells[1].Value)
			}
			w.credentials.grants = append(w.credentials.grants, credentialGrant{Kind: row.Cells[0].Value, Source: path})
		}
		return w.credentialPolicy(3)
	})
	sc.Step(`^the credential fixture keeps "([^"]*)" grants$`, func(which string) error {
		if which == "none" {
			w.credentials.grants = nil
		} else if which != "requested" {
			return fmt.Errorf("unknown grants %q", which)
		}
		return w.credentialPolicy(3)
	})
	sc.Step(`^the credential fixture manifest is absent$`, func() error { return os.Remove(w.manifestPath()) })
	sc.Step(`^the credential fixture policy version is (\d+) with no credentials section$`, func(version int) error {
		w.credentials.grants = nil
		return w.credentialPolicy(version)
	})
	sc.Step(`^the credential fixture protected state root is absent$`, func() error { return os.Remove(w.credentials.state) })
	sc.Step(`^the credential fixture "([^"]*)" has an unreadable descendant$`, func(boundary string) error {
		root := w.manifest.checkout
		if boundary == "protected state" {
			root = w.credentials.state
		} else if boundary != "checkout" {
			return fmt.Errorf("unknown boundary %q", boundary)
		}
		path := filepath.Join(root, "unreadable")
		if err := os.Mkdir(path, 0o700); err != nil {
			return err
		}
		w.credentials.cleanup = append(w.credentials.cleanup, func() { _ = os.Chmod(path, 0o700) })
		return os.Chmod(path, 0)
	})
	sc.Step(`^credential inspection effects are snapshotted$`, func() error {
		var err error
		w.credentials.snapshot, err = w.credentialSnapshot()
		return err
	})
	sc.Step(`^credential inspection has no side effects or helper execution$`, func() error {
		after, err := w.credentialSnapshot()
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(after, w.credentials.snapshot) {
			return fmt.Errorf("credential inspection changed fixture metadata or bytes")
		}
		entries, err := os.ReadDir(w.manifest.recorder)
		if err != nil {
			return err
		}
		if len(entries) != 0 {
			return fmt.Errorf("credential inspection executed helpers: %v", entries)
		}
		return nil
	})
	sc.Step(`^its credential JSON reports exactly these grants and no problems:$`, func(table *godog.Table) error {
		var grants []credentialGrant
		for _, row := range table.Rows[1:] {
			grants = append(grants, credentialGrant{row.Cells[0].Value, w.credentials.paths[row.Cells[1].Value], row.Cells[2].Value})
		}
		return w.credentialJSON(grants, &godog.Table{})
	})
	sc.Step(`^its credential JSON reports exactly these source problems and retains the requested grants:$`, func(table *godog.Table) error { return w.credentialJSON(w.requestedCredentials(), table) })
	sc.Step(`^the credential fixture command reports exactly "([^"]*)" at "([^"]*)" with exit (\d+)$`, func(rule, path string, code int) error {
		if err := w.exitsWith(code); err != nil {
			return err
		}
		if strings.TrimSpace(w.stdout) == "" {
			line := strings.TrimSuffix(w.stderr, "\n")
			prefix := "itos-container: " + rule + " " + strconv.Quote("itos-container.yaml"+path) + ": "
			message, ok := strings.CutPrefix(line, prefix)
			text, fix, separated := strings.Cut(message, "; ")
			if !ok || !separated || text == "" || fix == "" || strings.Contains(line, "\n") {
				return fmt.Errorf("incorrect plain credential gate diagnostic\n%s", w.report())
			}
			return nil
		}
		var table godog.Table
		data := fmt.Sprintf(`{"rows":[{}, {"cells":[{"value":%q},{"value":%q},{"value":%q}]}]}`, rule, path, strconv.Itoa(code))
		if err := json.Unmarshal([]byte(data), &table); err != nil {
			return err
		}
		return w.policyProblems(&table, false)
	})
	sc.Step(`^its plain credential output lists exactly the requested grants$`, func() error {
		grants := w.requestedCredentials()
		var lines []string
		for _, line := range strings.Split(w.stdout, "\n") {
			if strings.Contains(line, "claude-token") || strings.Contains(line, "github-token") {
				lines = append(lines, line)
			}
		}
		if len(lines) != len(grants) {
			return fmt.Errorf("got %d credential lines, want %d", len(lines), len(grants))
		}
		for i, g := range grants {
			escaped := strconv.Quote(g.Source)
			// The specification requires one sorted line per kind, with the
			// exact literal path and fixed target, without fixing its labels.
			for _, value := range []string{g.Kind, escaped[1 : len(escaped)-1], g.Target} {
				pattern := `(^|[[:space:].:=])` + regexp.QuoteMeta(value) + `($|[[:space:]:=])`
				if !regexp.MustCompile(pattern).MatchString(lines[i]) {
					return fmt.Errorf("credential line %q lacks exact field %q", lines[i], value)
				}
			}
		}
		return nil
	})
}

func (w *world) credentialFixture() error {
	root, err := os.MkdirTemp("/tmp", "slice-29-credentials-")
	if err != nil {
		return err
	}
	if err := os.Remove(w.dir); err != nil {
		_ = os.RemoveAll(root)
		return err
	}
	w.dir = root

	if err := w.manifestCheckout(); err != nil {
		return err
	}
	c := &credentialWorld{paths: map[string]string{}, sources: filepath.Join(w.manifest.host, "sources"), state: filepath.Join(w.manifest.env["XDG_STATE_HOME"], "itos-container")}
	w.credentials = c
	for _, path := range []string{c.sources, c.state, filepath.Join(w.manifest.checkout, ".git")} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			return err
		}
	}
	w.manifest.env["ITOS_ROOT"] = w.manifest.checkout
	w.manifest.env["DOCKER_HOST"] = w.policy.env["DOCKER_HOST"]
	return w.manifestSentinels()
}

func (w *world) closeCredentials() {
	if w.credentials == nil {
		return
	}
	if w.credentials.watch != nil {
		w.credentials.watch.close()
	}
	for _, cleanup := range w.credentials.cleanup {
		cleanup()
	}
}

func (w *world) credentialPolicy(version int) error {
	p := map[string]any{"version": version, "manifest": w.manifest.selected, "network": map[string]string{"mode": "offline"}}
	if len(w.credentials.grants) != 0 {
		p["credentials"] = w.credentials.grants
	}
	data, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(w.manifest.checkout, "itos-container.yaml"), data, 0o600)
}

func (w *world) credentialFile(name string, size int, mode string) error {
	bits, err := strconv.ParseUint(mode, 8, 32)
	if err != nil {
		return err
	}
	path := filepath.Join(w.credentials.sources, name)
	w.credentials.paths[name] = path
	if err := os.WriteFile(path, []byte(strings.Repeat("x", size)), 0o600); err != nil {
		return err
	}
	return os.Chmod(path, fs.FileMode(bits))
}

func (w *world) requestedCredentials() []credentialGrant {
	grants := slices.Clone(w.credentials.grants)
	for i := range grants {
		grants[i].Target = "/run/itos-container/credentials/" + grants[i].Kind
	}
	slices.SortFunc(grants, func(a, b credentialGrant) int { return strings.Compare(a.Kind, b.Kind) })
	return grants
}

func (w *world) credentialJSON(grants []credentialGrant, table *godog.Table) error {
	object, err := w.json()
	if err != nil {
		return err
	}
	if object["schema"] != float64(1) || object["scope"] != "credentials" || object["ok"] != (len(table.Rows) == 0) {
		return fmt.Errorf("incorrect credential envelope\n%s", w.report())
	}
	if len(object) != 5 {
		return fmt.Errorf("unexpected credential envelope fields\n%s", w.report())
	}
	want := make([]map[string]any, 0, len(grants))
	for _, g := range grants {
		want = append(want, map[string]any{"kind": g.Kind, "source": g.Source, "target": g.Target})
	}
	data, err := json.Marshal(want)
	if err != nil {
		return err
	}
	var expected any
	if err := json.Unmarshal(data, &expected); err != nil {
		return err
	}
	if !reflect.DeepEqual(object["credentials"], expected) {
		return fmt.Errorf("incorrect credential grants\n%s", w.report())
	}
	if len(table.Rows) == 0 {
		problems, ok := object["problems"].([]any)
		if !ok || len(problems) != 0 {
			return fmt.Errorf("unexpected credential problems\n%s", w.report())
		}
		return nil
	}
	return w.policyProblems(table, false)
}

func (w *world) credentialSnapshot() (map[string]string, error) {
	result := map[string]string{}
	err := filepath.WalkDir(w.dir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		result[path] = fmt.Sprintf("%v|%d", info.Mode(), info.Size())
		stat := reflect.Indirect(reflect.ValueOf(info.Sys()))
		if stat.IsValid() && stat.Kind() == reflect.Struct {
			for _, name := range []string{"Uid", "Gid", "Dev", "Ino", "Nlink"} {
				field := stat.FieldByName(name)
				if field.IsValid() && field.CanUint() {
					result[path] += fmt.Sprintf("|%s=%d", name, field.Uint())
				}
			}
		}
		if info.IsDir() && info.Mode().Perm()&0o500 != 0o500 {
			return filepath.SkipDir
		}
		if info.Mode().IsRegular() && info.Mode().Perm()&0o400 != 0 {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			result[path] += "|" + string(data)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			result[path] += "|" + target
		}
		return nil
	})
	return result, err
}

func (w *world) credentialMarkerFile(path string) error {
	marker := "itos-synthetic-" + rand.Text()
	w.credentials.markers = append(w.credentials.markers, marker)
	return writeManifestFixture(path, []byte(marker), 0o600)
}
