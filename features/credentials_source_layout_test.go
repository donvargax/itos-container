package features

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
)

func (w *world) credentialLayout(name, layout string) error {
	c := w.credentials
	path := filepath.Join(c.sources, name)
	c.paths[name] = path
	switch layout {
	case "private regular file":
		return w.credentialMarkerFile(path)
	case "absent leaf":
		return nil
	case "absent ancestor":
		c.paths[name] = filepath.Join(c.sources, "absent", name)
		return nil
	case "directory":
		return os.Mkdir(path, 0o700)
	case "FIFO with no writer":
		return manifestFIFO(path)
	case "Unix socket with no listener":
		listener, err := net.Listen("unix", path)
		if err != nil {
			return err
		}
		listener.(*net.UnixListener).SetUnlinkOnClose(false)
		return listener.Close()
	case "leaf symlink to private source", "dangling leaf symlink":
		target := path + "-target"
		if layout == "leaf symlink to private source" {
			if err := w.credentialMarkerFile(target); err != nil {
				return err
			}
		}
		return os.Symlink(target, path)
	case "parent symlink to private directory", "dangling parent symlink":
		target := path + "-target"
		if layout == "parent symlink to private directory" {
			if err := w.credentialMarkerFile(filepath.Join(target, "token")); err != nil {
				return err
			}
		}
		c.paths[name] = filepath.Join(path, "token")
		return os.Symlink(target, path)
	case "regular file used as a parent":
		c.paths[name] = filepath.Join(path, "token")
		return w.credentialMarkerFile(path)
	case "checkout root":
		c.paths[name] = w.manifest.checkout
		return nil
	case "protected state root":
		c.paths[name] = c.state
		return nil
	case "private file at checkout root":
		path = filepath.Join(w.manifest.checkout, name)
	case "private file below checkout .git":
		path = filepath.Join(w.manifest.checkout, ".git", name)
	case "private file below checkout hidden folder":
		path = filepath.Join(w.manifest.checkout, ".hidden", name)
	case "private file below protected state":
		path = filepath.Join(c.state, "private", name)
	case "private source in checkout-name-suffix sibling directory":
		path = filepath.Join(w.manifest.checkout+"-sibling", name)
	case "private source in state-name-suffix sibling directory":
		path = filepath.Join(c.state+"-sibling", name)
	case "world-readable regular file":
		if err := w.credentialMarkerFile(path); err != nil {
			return err
		}
		return os.Chmod(path, 0o604)
	case "private regular file of zero bytes":
		return w.credentialFile(name, 0, "0600")
	case "private regular file of 65537 bytes":
		return w.credentialFile(name, 65537, "0600")
	case "private source below group-writable parent", "private source below other-writable parent", "private source below user-owned world-writable sticky parent":
		parent := path + "-parent"
		path = filepath.Join(parent, "token")
		c.paths[name] = path
		if err := w.credentialMarkerFile(path); err != nil {
			return err
		}
		mode := os.FileMode(0o720)
		if strings.Contains(layout, "other-writable") {
			mode = 0o702
		}
		if strings.Contains(layout, "sticky") {
			mode = 0o777 | os.ModeSticky
		}
		return os.Chmod(parent, mode)
	case "private source with a hard link in checkout", "private source with a hard link in protected state", "private source with a hard link outside checkout":
		if err := w.credentialMarkerFile(path); err != nil {
			return err
		}
		root := c.sources
		if layout == "private source with a hard link in checkout" {
			root = w.manifest.checkout
		}
		if layout == "private source with a hard link in protected state" {
			root = c.state
		}
		return os.Link(path, filepath.Join(root, name+"-link"))
	default:
		return fmt.Errorf("unknown credential layout %q", layout)
	}
	c.paths[name] = path
	return w.credentialMarkerFile(path)
}
