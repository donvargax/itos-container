package features

import (
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"testing/synctest"
)

// The initial implementation mirrors the existing per-helper cache. The
// concurrency regression below exposes publication of an incomplete build.
func featureBinary(path string, build func(string) error) (string, error) {
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}
	return path, build(path)
}

func TestFeatureBinaryPublishesOnceAfterBuild(t *testing.T) {
	path := filepath.Join(t.TempDir(), "probe")
	synctest.Test(t, func(t *testing.T) {
		var builds atomic.Int32
		started, release, done := make(chan struct{}), make(chan struct{}), make(chan string, 2)
		build := func(target string) error {
			if builds.Add(1) != 1 {
				return errors.New("duplicate build")
			}
			if err := os.WriteFile(target, []byte("partial"), 0700); err != nil {
				return err
			}
			close(started)
			<-release
			return os.WriteFile(target, []byte("complete"), 0700)
		}
		read := func() {
			got, err := featureBinary(path, build)
			if err != nil {
				done <- err.Error()
				return
			}
			data, err := os.ReadFile(got)
			if err != nil {
				done <- err.Error()
				return
			}
			done <- string(data)
		}
		go read()
		<-started
		go read()
		synctest.Wait()
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Error("cache published before build completed")
		}
		close(release)
		for range 2 {
			if got := <-done; got != "complete" {
				t.Errorf("reader observed %q", got)
			}
		}
		got, err := featureBinary(path, func(string) error { return errors.New("cache rebuilt") })
		if err != nil || got != path || builds.Load() != 1 {
			t.Error("successful cache not reused", got, err, builds.Load())
		}
	})
}

func TestFeatureBinaryFailedBuildIsNotCached(t *testing.T) {
	path := filepath.Join(t.TempDir(), "probe")
	failure := errors.New("failed compiler")
	_, err := featureBinary(path, func(target string) error {
		if err := os.WriteFile(target, []byte("partial"), 0700); err != nil {
			return err
		}
		return failure
	})
	if !errors.Is(err, failure) {
		t.Fatal("lost build failure", err)
	}
	_, err = featureBinary(path, func(target string) error {
		return os.WriteFile(target, []byte("complete"), 0700)
	})
	data, readErr := os.ReadFile(path)
	if err != nil || readErr != nil || string(data) != "complete" {
		t.Error("failed build poisoned cache", err, readErr, string(data))
	}
}
