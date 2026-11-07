package main

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	approvaldisk "github.com/donvargax/itos-container/internal/engine/approval/disk"
	ap "github.com/donvargax/itos-container/internal/engine/approval/port"
)

// Publish once after admission opens the original record. Both record versions
// are valid; only the interleaving is controlled by this test-only assembly.
func publicProgressReplacement(t *testing.T, storage approvaldisk.Registry, request publicProbeRequest) func(*os.File) (os.FileInfo, error) {
	t.Helper()
	replaced := false
	return func(file *os.File) (os.FileInfo, error) {
		info, err := file.Stat()
		dir := filepath.Dir(file.Name())
		if err != nil || replaced || filepath.Base(file.Name()) != "record.json" || filepath.Base(dir) != request.ID {
			return info, err
		}
		replaced = true
		cwd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
		defer cancel()
		c := ap.Candidate{Root: cwd, UID: strconv.Itoa(os.Geteuid()), Project: filepath.Base(filepath.Dir(dir))}
		session, failure := storage.Open(ctx, c, request.ID, false)
		if failure != "" {
			t.Fatal("progress writer open", failure)
		}
		defer session.Close()
		data, failure := session.Read("record.json")
		if failure != "" {
			t.Fatal("progress writer read", failure)
		}
		if failure = session.Publish("record.json", data, data); failure != "" {
			t.Fatal("progress writer publication", failure)
		}
		if err := os.WriteFile(request.Ready, []byte("replaced"), 0600); err != nil {
			return nil, err
		}
		return info, nil
	}
}
