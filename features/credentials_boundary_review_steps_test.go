package features

import (
	"fmt"
	"strings"

	"github.com/cucumber/godog"
)

func (w *world) credentialBoundaryReviewSteps(sc *godog.ScenarioContext) {
	sc.Step(`^the credential checkout aliases "([^"]*)" state through "([^"]*)"$`, w.credentialCheckoutStateAlias)
	sc.Step(`^the credential observation recorded no checkout traversal$`, func() error {
		return w.credentialNoCalls("checkout traversal")
	})
	sc.Step(`^credential descriptors have "([^"]*)" beside an unrelated "([^"]*)" mount$`, func(fault, filesystem string) error {
		o := w.credentials.observation
		for i := range o.Mounts {
			o.Mounts[i] += "2 1 0:64 / /unrelated/merged rw - " + filesystem + " backing rw\n"
		}
		switch fault {
		case "missing mount ID":
			o.MountIDUnavailable = map[string]bool{}
			for path := range o.Nodes {
				o.MountIDUnavailable[path] = true
			}
		case "unavailable checkout descriptor":
			o.DescriptorErrors["/checkout"] = true
		default:
			return fmt.Errorf("unknown descriptor fault %q", fault)
		}
		return nil
	})
}

func (w *world) credentialCheckoutStateAlias(which, layout string) error {
	o := w.credentials.observation
	state := "/state"
	if which == "account" {
		state = "/account/.local/state/itos-container"
	} else if which != "selected" {
		return fmt.Errorf("unknown state boundary %q", which)
	}
	parent := ""
	for i, part := range strings.Split(state[1:], "/") {
		parent += "/" + part
		if _, exists := o.Nodes[parent]; !exists {
			n := o.Nodes["/state"]
			n.Path, n.Inode = parent, uint64(800+i)
			o.Nodes[parent] = n
		}
	}
	point, root := "/checkout", state
	switch layout {
	case "root bind":
		n := o.Nodes[state]
		n.Path = point
		o.Nodes[point] = n
	case "descendant bind":
		root += "/private"
	case "ancestor bind":
		root = "/"
	case "descendant mount":
		point += "/records"
	case "inverse bind":
		point, root = state, "/checkout/private"
	default:
		return fmt.Errorf("unknown boundary alias %q", layout)
	}
	for i := range o.Mounts {
		o.Mounts[i] += "2 1 0:1 " + root + " " + point + " rw - tmpfs tmpfs rw\n"
	}
	for path, n := range o.Nodes {
		if path == point || strings.HasPrefix(path, point+"/") {
			n.MountID = 2
			o.Nodes[path] = n
		}
	}
	// Any attempted inventory fails, instead of silently treating protected
	// records as ordinary checkout entries under the alias spelling.
	o.ReadErrors["/checkout"] = true
	return nil
}
