package disk

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/donvargax/itos-container/internal/engine/approval/port"
)

func TestAdmissionScanRetriesAtomicProgress(t *testing.T) {
	for _, window := range []string{"before open", "after open", "after read"} {
		for _, replacements := range []int{1, 2, 3} {
			t.Run(fmt.Sprintf("%s/%d", window, replacements), func(t *testing.T) {
				d, writer := admissionProgressFixture(t)
				ops := *d.store.ops
				attempts, published := 0, 0
				previous := []byte("original")
				replace := func() {
					if published >= replacements {
						return
					}
					published++
					next := []byte(fmt.Sprintf("progress %d", published))
					if f := writer.Publish("record.json", previous, next); f != "" {
						t.Fatal("progress publication", f)
					}
					previous = next
				}
				open := ops.openRecord
				ops.openRecord = func(root *os.Root, name string, flags int, mode os.FileMode) (*os.File, error) {
					if name == "record.json" {
						attempts++
						if window == "before open" {
							replace()
						}
					}
					file, err := open(root, name, flags, mode)
					if err == nil && name == "record.json" && window == "after open" {
						replace()
					}
					return file, err
				}
				read := ops.readAll
				ops.readAll = func(r io.Reader) ([]byte, error) {
					data, err := read(r)
					if window == "after read" {
						replace()
					}
					return data, err
				}
				d.store.ops = &ops
				records, f := d.Sessions()
				if replacements == 3 {
					want := port.Failure("state.path")
					if window == "after open" {
						want = "state.unsafe"
					}
					if f != want || len(records) != 0 || attempts != 3 {
						t.Fatalf("persistent replacement must refuse after three reads: %v %s, attempts %d", records, f, attempts)
					}
					return
				}
				if f != "" || len(records) != 1 || string(records[0].Data) != string(previous) || attempts != replacements+1 {
					t.Fatalf("progress replacement refused or returned stale data: %v %s, attempts %d", records, f, attempts)
				}
				if records[0].Index != d.candidate.Project || records[0].ID != strings.Repeat("b", 64) {
					t.Fatal("progress replacement changed registration identity", records)
				}
			})
		}
	}
}

func TestAdmissionScanRetryRetainsSafety(t *testing.T) {
	for _, fault := range []string{"symlink", "hardlink", "mode", "directory", "canceled", "read error"} {
		t.Run(fault, func(t *testing.T) {
			d, writer := admissionProgressFixture(t)
			ctx, cancel := context.WithCancel(d.ctx)
			defer cancel()
			d.ctx = ctx
			ops := *d.store.ops
			read := ops.readAll
			reads := 0
			ops.readAll = func(r io.Reader) ([]byte, error) {
				data, err := read(r)
				reads++
				if reads != 1 {
					return data, err
				}
				if fault == "read error" {
					return nil, os.ErrPermission
				}
				if f := writer.Publish("record.json", []byte("original"), []byte("progress")); f != "" {
					t.Fatal(f)
				}
				path := filepath.Join(writer.(*sessionDirectory).pin.path, "record.json")
				switch fault {
				case "symlink":
					if err := os.Rename(path, path+"-saved"); err != nil {
						t.Fatal(err)
					}
					err = os.Symlink(path+"-saved", path)
				case "hardlink":
					err = os.Link(path, path+"-linked")
				case "mode":
					err = os.Chmod(path, 0644)
				case "directory":
					err = os.Rename(filepath.Dir(path), filepath.Dir(path)+"-moved")
				case "canceled":
					cancel()
				}
				if err != nil {
					t.Fatal(err)
				}
				return data, nil
			}
			d.store.ops = &ops
			if records, f := d.Sessions(); f == "" || len(records) != 0 || reads != 1 {
				t.Fatalf("unsafe retry accepted or reread unsafe state: %v %s, reads %d", records, f, reads)
			}
		})
	}
}

func admissionProgressFixture(t *testing.T) (*admissionDirectory, port.SessionDirectory) {
	t.Helper()
	s, c, _ := diskFixture(t)
	r := Registry{Store: s}
	id := strings.Repeat("b", 64)
	lease, f := r.Register(t.Context(), c, id, []byte("original"))
	if f != "" {
		t.Fatal(f)
	}
	t.Cleanup(lease.Close)
	writer, f := r.Open(t.Context(), c, id, false)
	if f != "" {
		t.Fatal(f)
	}
	// Keep the real operation lock held throughout the scan and every retry.
	t.Cleanup(writer.Close)
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	t.Cleanup(cancel)
	d, f := r.Admission(ctx, c)
	if f != "" {
		t.Fatal(f)
	}
	t.Cleanup(d.Close)
	return d.(*admissionDirectory), writer
}
