package features

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

var featureBinaryBuild sync.Mutex

// Parallel scenarios share only compiled, trusted helpers outside their
// checkouts. Wait for a complete build before reuse; never overwrite a helper
// another scenario may already be executing. Failed builds are not cached.
func cachedFeatureBinary(path string, build func(string) error) error {
	featureBinaryBuild.Lock()
	defer featureBinaryBuild.Unlock()
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	dir, err := os.MkdirTemp(filepath.Dir(path), "slice-21-binary-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	output := filepath.Join(dir, filepath.Base(path))
	if err := build(output); err != nil {
		return err
	}
	return os.Rename(output, path)
}

func TestFeatureBinaryPublishesOnceAfterBuild(t *testing.T) {
	path := filepath.Join(t.TempDir(), "helper")
	entered, release := make(chan struct{}), make(chan struct{})
	built := 0
	build := func(output string) error {
		built++
		if err := os.WriteFile(output, []byte("partial"), 0700); err != nil {
			return err
		}
		close(entered)
		<-release
		return os.WriteFile(output, []byte("complete"), 0700)
	}
	results := make(chan error, 2)
	go func() { results <- cachedFeatureBinary(path, build) }()
	<-entered
	go func() { results <- cachedFeatureBinary(path, build) }()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("incomplete helper became visible to another scenario")
	}
	close(release)
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "complete" || built != 1 {
		t.Fatalf("helper = %q, builds = %d, error = %v", data, built, err)
	}
}

func TestFeatureBinaryRetriesFailedBuild(t *testing.T) {
	path := filepath.Join(t.TempDir(), "helper")
	failure := errors.New("compiler failed after partial output")
	err := cachedFeatureBinary(path, func(output string) error {
		if err := os.WriteFile(output, []byte("partial"), 0700); err != nil {
			return err
		}
		return failure
	})
	if !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("failed helper was cached")
	}
	if err := cachedFeatureBinary(path, func(output string) error {
		return os.WriteFile(output, []byte("complete"), 0700)
	}); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) != 1 || entries[0].Name() != "helper" {
		t.Fatalf("build left temporary entries: %v, %v", entries, err)
	}
}
