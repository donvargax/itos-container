package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	approvaldisk "github.com/donvargax/itos-container/internal/engine/approval/disk"
)

// replacementEvidence is what the paused replacement left behind for the fixture
// once the real Rename had returned: the name it renamed, what the target then
// was, and whether the fixture kept it waiting past the finite-call cap. It is
// read from the file system, not claimed by the product.
type replacementEvidence struct {
	Source, Target       string
	TimedOut             bool
	Error                string
	Device, Inode, Links string
	Bytes                string
}

// publicReplacementIO adds a barrier only for a subprocess request that asks
// for one. The rename target distinguishes the intent and session records from
// admission claims, which live in the admission collection.
func publicReplacementIO(fault approvaldisk.SnapshotIO) approvaldisk.SnapshotIO {
	path := os.Getenv("SLICE_11_REQUEST")
	if path == "" {
		return fault
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fault
	}
	var request publicProbeRequest
	if json.Unmarshal(data, &request) != nil {
		return fault
	}
	return pauseReplacement(fault, request)
}

// pauseReplacement wraps the Rename of one storage's fault IO for the two
// replacement boundaries. It pauses on the first call that renames a Publish
// temporary onto the record that boundary names, once that temporary is durable:
// it writes Ready with the source name, then waits for Release. The real Rename
// stays the fault IO's own. After it returns, it writes Evidence with the wait's
// outcome and the target's device, inode, link count and bytes, so the fixture
// can tell what the replacement installed.
//
// The wait is bounded by the finite-call cap of a command, as the product's own
// calls are, so a fixture that never releases cannot hold the run for longer. The
// real Rename still runs after that deadline; the timeout is evidence, not a
// reason to strand W until the shard's hang guard.
func pauseReplacement(fault approvaldisk.SnapshotIO, request publicProbeRequest) approvaldisk.SnapshotIO {
	boundary := request.Boundary
	if boundary != "intent replacement" && boundary != "public record replacement" {
		return fault
	}
	real, paused := fault.Rename, false
	fault.Rename = func(root *os.Root, from, to string) error {
		if paused || !publishTemporary(from) || !replacementTarget(boundary, root.Name(), to) {
			return real(root, from, to)
		}
		paused = true
		_ = os.WriteFile(request.Ready, []byte(from), 0600)
		timedOut := false
		deadline := time.NewTimer(10 * time.Second)
		defer deadline.Stop()
		tick := time.NewTicker(5 * time.Millisecond)
		defer tick.Stop()
	wait:
		for {
			if data, err := os.ReadFile(request.Release); err == nil && string(data) == "continue" {
				break
			}
			select {
			case <-deadline.C:
				timedOut = true
				break wait
			case <-tick.C:
			}
		}
		renameErr := real(root, from, to)
		evidence := replacementEvidence{Source: from, Target: to, TimedOut: timedOut}
		if renameErr != nil {
			evidence.Error = renameErr.Error()
		}
		if info, err := root.Lstat(to); err == nil {
			evidence.Device, evidence.Inode, evidence.Links = replacementStat(info)
			if file, err := root.Open(to); err == nil {
				data, readErr := io.ReadAll(file)
				_ = file.Close()
				if readErr == nil {
					evidence.Bytes = hex.EncodeToString(data)
				}
			}
		}
		if data, err := json.Marshal(evidence); err == nil {
			_ = os.WriteFile(request.Evidence, data, 0600)
		}
		return renameErr
	}
	return fault
}

// publishTemporary is the name directoryStore.Publish gives its temporary.
func publishTemporary(name string) bool {
	return len(name) == 44 && strings.HasPrefix(name, ".intent-") && strings.HasSuffix(name, ".tmp") && strings.Trim(name[8:40], "0123456789abcdef") == ""
}

// replacementTarget names the record a boundary waits for: an intent record for
// the intent row, and the session's public record for the other.
func replacementTarget(boundary, directory, name string) bool {
	if boundary == "public record replacement" {
		return name == "record.json"
	}
	return filepath.Base(filepath.Dir(directory)) == "intents" && len(name) == 69 && strings.HasSuffix(name, ".json") && strings.Trim(name[:64], "0123456789abcdef") == ""
}

func replacementStat(info os.FileInfo) (string, string, string) {
	stat := reflect.Indirect(reflect.ValueOf(info.Sys()))
	return fmt.Sprint(stat.FieldByName("Dev")), fmt.Sprint(stat.FieldByName("Ino")), fmt.Sprint(stat.FieldByName("Nlink"))
}
