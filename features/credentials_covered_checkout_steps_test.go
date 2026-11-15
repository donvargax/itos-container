package features

import (
	"fmt"

	"github.com/cucumber/godog"
)

func (w *world) credentialCoveredCheckoutSteps(sc *godog.ScenarioContext) {
	sc.Step(`^the credential source aliases a covered checkout "([^"]*)" in the "([^"]*)" observation$`, w.credentialCoveredCheckout)
}

func (w *world) credentialCoveredCheckout(kind, observation string) error {
	o := w.credentials.observation
	source := w.credentials.paths["token"]
	covered, point, coverRoot := "/checkout/private", "/checkout-sources", "/"
	n := o.Nodes[covered]
	switch kind {
	case "directory":
		alias := n
		alias.Path, alias.MountID = point, 2
		o.Nodes[point] = alias
	case "file":
		covered, point, coverRoot = covered+"/token", source, "/replacement-token"
		n = o.Nodes[source]
	default:
		return fmt.Errorf("unknown covered checkout kind %q", kind)
	}
	// The host inventory sees only the unrelated covering mount. The worker's
	// non-recursive checkout bind exposes the backing directory or file instead.
	n.Path, n.Device, n.Inode, n.MountID = covered, 2, 90, 3
	o.Nodes[covered] = n
	n = o.Nodes[source]
	n.MountID = 2
	o.Nodes[source] = n
	if observation != "both" && observation != "first" && observation != "final" {
		return fmt.Errorf("unknown covered checkout observation %q", observation)
	}
	for i := range o.Mounts {
		root := covered
		if observation == "first" && i == 1 || observation == "final" && i == 0 {
			root = "/independent"
		}
		o.Mounts[i] += fmt.Sprintf("2 1 0:1 %s %s rw - tmpfs tmpfs rw\n3 1 0:2 %s %s rw - tmpfs tmpfs rw\n", root, point, coverRoot, covered)
	}
	return nil
}
