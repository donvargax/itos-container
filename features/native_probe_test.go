package features

import (
	"os"
	"path/filepath"
	"sync"
)

var nativeProbeBuild sync.Mutex

// Cache only compiled trusted bytes beside the suite's product binary, outside
// every writable checkout. Publish after a successful build, then copy into a
// fresh inode: a worker may modify its own probe without poisoning later cases.
func (w *world) installNativeProbe(name string, build func(string) error) error {
	cache := w.bin + "-" + name
	if !filepath.IsAbs(w.bin) {
		cache = filepath.Join(w.dir, name+"-cache")
	}
	data, err := nativeProbeBytes(cache, build)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(w.doctor.checkout, name), data, 0700)
}

func nativeProbeBytes(cache string, build func(string) error) ([]byte, error) {
	nativeProbeBuild.Lock()
	defer nativeProbeBuild.Unlock()
	data, err := os.ReadFile(cache)
	if !os.IsNotExist(err) {
		return data, err
	}
	dir, err := os.MkdirTemp(filepath.Dir(cache), "slice-12-probe-build-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	path := filepath.Join(dir, "probe")
	if err := build(path); err != nil {
		return nil, err
	}
	data, err = os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if err := os.Rename(path, cache); err != nil {
		return nil, err
	}
	return data, nil
}
