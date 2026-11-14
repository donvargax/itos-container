package features

import (
	"fmt"
	"strings"
)

// Model a backing file and its merged/pass-through view with the same inode
// and different devices. The table describes the mount, not an alias verdict.
func (w *world) credentialStackingMount(filesystem, point string) error {
	if filesystem != "overlay" && filesystem != "fuse" && filesystem != "fuseblk" && !strings.HasPrefix(filesystem, "fuse.") && !strings.HasPrefix(filesystem, "fuseblk.") {
		return fmt.Errorf("unknown stacking fixture filesystem %q", filesystem)
	}
	o := w.credentials.observation
	source := o.Nodes[w.credentials.paths["token"]]
	parent := ""
	for i, part := range strings.Split(strings.TrimPrefix(point, "/"), "/") {
		parent += "/" + part
		if _, exists := o.Nodes[parent]; !exists {
			n := o.Nodes["/state"]
			n.Path, n.Inode = parent, uint64(100+i)
			o.Nodes[parent] = n
		}
	}
	source.Path = point + "/token"
	o.Nodes[source.Path] = source
	for path, n := range o.Nodes {
		if path == point || strings.HasPrefix(path, point+"/") {
			n.Device, n.MountID = 64, 2
			o.Nodes[path] = n
		}
	}
	for i := range o.Mounts {
		o.Mounts[i] += "2 1 0:64 / " + point + " rw - " + filesystem + " backing rw\n"
	}
	return nil
}
