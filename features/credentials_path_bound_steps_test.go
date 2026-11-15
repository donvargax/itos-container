package features

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/cucumber/godog"
)

// These fixtures are real files on the host's kernel. A path longer than
// PATH_MAX cannot be named in one call, so every step below works relative to
// an open directory and never passes the long path to the kernel itself.
func (w *world) credentialPathBoundSteps(sc *godog.ScenarioContext) {
	sc.Step(`^the credential fixture "([^"]*)" lies at a real path of exactly (\d+) bytes$`, w.credentialPathOfLength)
}

func (w *world) credentialPathOfLength(kind string, size int) error {
	c := w.credentials
	switch kind {
	case "source":
		// The request named "token" moves to the long path; the fixture's
		// checkout and its policy stay where they are.
		parent, path, err := credentialLongParent(c.sources, size, "token")
		if err != nil {
			return err
		}
		defer func() { _ = parent.Close() }()
		marker := "itos-synthetic-" + rand.Text()
		c.markers = append(c.markers, marker)
		if err := parent.WriteFile("token", []byte(marker), 0o600); err != nil {
			return err
		}
		for i := range c.grants {
			if c.grants[i].Source == c.paths["token"] {
				c.grants[i].Source = path
			}
		}
		c.paths["token"] = path
		return w.credentialPolicy(3)
	case "checkout":
		// Only ITOS_ROOT names the long checkout. It holds the fixture's own
		// request, which the mode reads through its pin of the effective root.
		parent, path, err := credentialLongParent(w.dir, size, "checkout")
		if err != nil {
			return err
		}
		defer func() { _ = parent.Close() }()
		data, err := w.credentialPolicyData(3)
		if err != nil {
			return err
		}
		if err := parent.MkdirAll("checkout/.git", 0o700); err != nil {
			return err
		}
		if err := parent.WriteFile("checkout/itos-container.yaml", data, 0o600); err != nil {
			return err
		}
		w.manifest.env["ITOS_ROOT"] = path
		return nil
	case "selected state":
		// XDG_STATE_HOME selects the parent; the state root exists, private,
		// so nothing but its length can refuse it.
		parent, path, err := credentialLongParent(w.dir, size, "itos-container")
		if err != nil {
			return err
		}
		defer func() { _ = parent.Close() }()
		if err := parent.Mkdir("itos-container", 0o700); err != nil {
			return err
		}
		c.state = path
		w.manifest.env["XDG_STATE_HOME"] = filepath.Dir(path)
		return nil
	default:
		return fmt.Errorf("unknown long credential path %q", kind)
	}
}

// credentialLongParent creates private nested directories below base, in a
// folder named for the length, so that <returned parent>/<leaf> is a path of
// exactly size bytes. It returns the open parent and that path; the caller
// creates the leaf. Every directory name stays within NAME_MAX.
func credentialLongParent(base string, size int, leaf string) (*os.Root, string, error) {
	path := filepath.Join(base, "path-of-"+strconv.Itoa(size)+"-bytes-for-"+leaf)
	if err := os.Mkdir(path, 0o700); err != nil {
		return nil, "", err
	}
	parent, err := os.OpenRoot(path)
	if err != nil {
		return nil, "", err
	}
	// Each nested directory spends one separator and its name.
	for fill := size - len(path) - 1 - len(leaf); fill != 0; {
		name := min(fill-1, 200)
		if fill-1-name == 1 {
			name-- // never leave a single byte, which no separator and name can spend
		}
		if name < 1 {
			_ = parent.Close()
			return nil, "", fmt.Errorf("cannot build a path of %d bytes below %q", size, base)
		}
		component := strings.Repeat("d", name)
		if err := parent.Mkdir(component, 0o700); err != nil {
			_ = parent.Close()
			return nil, "", err
		}
		next, err := parent.OpenRoot(component)
		_ = parent.Close()
		if err != nil {
			return nil, "", err
		}
		parent, path, fill = next, path+"/"+component, fill-name-1
	}
	path += "/" + leaf
	if len(path) != size {
		_ = parent.Close()
		return nil, "", fmt.Errorf("built a path of %d bytes, want %d", len(path), size)
	}
	return parent, path, nil
}
