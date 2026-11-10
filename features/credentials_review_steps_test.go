package features

import (
	"fmt"
	"strings"

	"github.com/cucumber/godog"
)

func (w *world) credentialReviewSteps(sc *godog.ScenarioContext) {
	sc.Step(`^credential observation exposes the source through "([^"]*)" at "([^"]*)"$`, w.credentialStackingMount)
	sc.Step(`^the credential source has (\d+) additional ancestor directories$`, func(depth int) error {
		o := w.credentials.observation
		source := w.credentials.paths["token"]
		n := o.Nodes[source]
		delete(o.Nodes, source)
		parent, _, _ := strings.Cut(source, "/token")
		for i := 0; i < depth; i++ {
			parent += "/d"
			dir := n
			dir.Path, dir.Inode, dir.Mode = parent, uint64(1000+i), 0040700
			o.Nodes[parent] = dir
		}
		n.Path = parent + "/token"
		o.Nodes[n.Path] = n
		w.credentials.paths["token"] = n.Path
		w.credentials.grants[0].Source = n.Path
		return w.credentialObservationPolicy(3)
	})
	sc.Step(`^the credential filesystem permits at most (\d+) open handles$`, func(limit int) {
		w.credentials.observation.HandleLimit = limit
	})
	sc.Step(`^credential mount tables include an unrelated mount with an empty source$`, func() {
		o := w.credentials.observation
		for i := range o.Mounts {
			o.Mounts[i] += "473 1 0:59 / /scratch rw,relatime - tmpfs  rw,uid=1000\n"
		}
	})
	sc.Step(`^credential mount tables include root "([^"]*)" at "([^"]*)"$`, func(root, point string) {
		o := w.credentials.observation
		for i := range o.Mounts {
			o.Mounts[i] += fmt.Sprintf("%d 1 0:4 %s %s rw - nsfs nsfs rw\n", 100+strings.Count(o.Mounts[i], "\n"), root, point)
		}
	})
	sc.Step(`^the credential checkout is inside "([^"]*)" state$`, func(which string) error {
		o := w.credentials.observation
		paths := []string{"/state", "/state/work", "/state/work/checkout"}
		if which == "account" {
			paths = []string{"/account", "/account/.local", "/account/.local/state", "/account/.local/state/itos-container", "/account/.local/state/itos-container/work", "/account/.local/state/itos-container/work/checkout"}
		} else if which != "selected" {
			return fmt.Errorf("unknown state boundary %q", which)
		}
		for i, path := range paths {
			n := o.Nodes["/checkout"]
			n.Path, n.Inode = path, uint64(900+i)
			o.Nodes[path] = n
		}
		o.Root = paths[len(paths)-1]
		return nil
	})
	sc.Step(`^the credential checkout environment uses "([^"]*)" spelling$`, func(spelling string) error {
		root := w.manifest.checkout
		switch spelling {
		case "trailing slash":
			root += "/"
		case "repeated slash":
			root = strings.ReplaceAll(root, "/", "//")
		case "dot component":
			root += "/."
		case "parent component":
			root += "/unused/.."
		default:
			return fmt.Errorf("unknown checkout spelling %q", spelling)
		}
		w.manifest.env["ITOS_ROOT"] = root
		return nil
	})
}
