package features

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/cucumber/godog"
)

func (w *world) credentialExportReviewSteps(sc *godog.ScenarioContext) {
	sc.Step(`^the credential fixture selects state through "([^"]*)" with a path that is not valid UTF-8$`, w.credentialStateEncoding)
	sc.Step(`^the credential source is a second "([^"]*)" mount of "([^"]*)" from the export mounted at "([^"]*)"$`, w.credentialExportAlias)
}

// The byte 0xff starts no UTF-8 sequence. The selected tree exists, private
// and owned by the user, so its encoding is the only thing left to refuse.
func (w *world) credentialStateEncoding(selection string) error {
	bad := filepath.Join(w.manifest.host, "selected-\xff")
	state := filepath.Join(bad, "itos-container")
	switch selection {
	case "XDG_STATE_HOME":
		w.manifest.env["XDG_STATE_HOME"] = bad
	case "HOME fallback":
		delete(w.manifest.env, "XDG_STATE_HOME")
		w.manifest.env["HOME"] = bad
		state = filepath.Join(bad, ".local", "state", "itos-container")
	default:
		return fmt.Errorf("unknown state selection %q", selection)
	}
	w.credentials.state = state
	return os.MkdirAll(state, 0o700)
}

// One export is mounted at point and a second time, rooted at subdirectory,
// over the source's parent. Both rows carry the export's device and the root
// "/" the kernel prints for every NFS mount, so the rows alone do not say that
// the source is the subdirectory's own file. The table is raw kernel text and
// the nodes raw identities: neither holds a verdict.
func (w *world) credentialExportAlias(filesystem, subdirectory, point string) error {
	if filesystem != "nfs" && filesystem != "nfs4" {
		return fmt.Errorf("unknown export fixture filesystem %q", filesystem)
	}
	below := func(p, root string) bool { return p == root || strings.HasPrefix(p, strings.TrimSuffix(root, "/")+"/") }
	if !below(subdirectory, point) || subdirectory == point {
		return fmt.Errorf("%q is not a subdirectory of the export at %q", subdirectory, point)
	}
	o := w.credentials.observation
	source := w.credentials.paths["token"]
	outside := path.Dir(source)
	parent := ""
	for i, part := range strings.Split(strings.TrimPrefix(subdirectory, "/"), "/") {
		parent += "/" + part
		if _, exists := o.Nodes[parent]; !exists {
			n := o.Nodes["/state"]
			n.Path, n.Inode = parent, uint64(700+i)
			o.Nodes[parent] = n
		}
	}
	for p, n := range o.Nodes {
		if below(p, point) {
			n.Device, n.MountID = 50, 5
			o.Nodes[p] = n
		}
	}
	// The second mount shows the subdirectory itself and its one-link file.
	dir := o.Nodes[subdirectory]
	dir.Path, dir.MountID = outside, 6
	o.Nodes[outside] = dir
	token := o.Nodes[source]
	token.Device, token.MountID = 50, 6
	o.Nodes[source] = token
	rows := fmt.Sprintf("5 1 0:50 / %s rw - %s server:/export rw\n6 1 0:50 / %s rw - %s server:/export/subdirectory rw\n", point, filesystem, outside, filesystem)
	if below(subdirectory, "/checkout") {
		// A host submount covers the checkout's own entry: the inventory meets
		// the covering directory, never the backing one the worker bind exposes.
		cover := o.Nodes[subdirectory]
		cover.Device, cover.Inode, cover.MountID = 2, cover.Inode+100, 7
		o.Nodes[subdirectory] = cover
		rows += "7 5 0:2 / " + subdirectory + " rw - tmpfs tmpfs rw\n"
	} else {
		// Protected state keeps the same file under its own name. Nothing may
		// visit it there: state records are never enumerated.
		hidden := token
		hidden.Path, hidden.MountID = subdirectory+"/token", 5
		o.Nodes[hidden.Path] = hidden
	}
	for i := range o.Mounts {
		o.Mounts[i] += rows
	}
	return nil
}
