package features

import (
	"encoding/binary"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"testing/fstest"
)

func TestCredentialRunWatchIntroducedDirectory(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "new")
	child := filepath.Join(directory, "entry")
	const watchID = 7
	name := "new\x00"
	record := make([]byte, credentialWatchHeaderSize+len(name))
	binary.NativeEndian.PutUint32(record, watchID)
	binary.NativeEndian.PutUint32(record[4:], credentialMoveTo|credentialWatchDirectory)
	binary.NativeEndian.PutUint32(record[12:], uint32(len(name)))
	copy(record[credentialWatchHeaderSize:], name)

	var watched []string
	observer := credentialEventObserver{
		recursive: true,
		watches:   map[int]string{watchID: root},
		watchIO: credentialWatchIO{
			read: func(_ int, buffer []byte) (int, error) {
				if record == nil {
					return 0, syscall.EAGAIN
				}
				n := copy(buffer, record)
				record = nil
				return n, nil
			},
			addWatch: func(_ int, path string, _ uint32) (int, error) {
				watched = append(watched, path)
				return watchID + 1, nil
			},
			walkDir: func(path string, visit fs.WalkDirFunc) error {
				if path != directory {
					t.Fatalf("walked %q, want %q", path, directory)
				}
				tree := fstest.MapFS{"entry": {}}
				return fs.WalkDir(tree, ".", func(name string, entry fs.DirEntry, err error) error {
					return visit(filepath.Join(path, name), entry, err)
				})
			},
		},
	}
	if err := observer.read(make([]byte, len(record))); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(watched, []string{directory}) || observer.watches[watchID+1] != directory {
		t.Fatalf("watched %v, registry %v, want new watch for %q", watched, observer.watches, directory)
	}
	if want := []credentialRunEvent{{directory, credentialMoveTo}, {child, credentialCreate}}; !slices.Equal(observer.records, want) {
		t.Fatalf("observed %v, want %v", observer.records, want)
	}
}

func TestCredentialRunWatchReads(t *testing.T) {
	const watchID = 7
	root := t.TempDir()
	name := "entry\x00\x00\x00"
	record := make([]byte, credentialWatchHeaderSize+len(name))
	binary.NativeEndian.PutUint32(record, watchID)
	binary.NativeEndian.PutUint32(record[4:], credentialCreate)
	binary.NativeEndian.PutUint32(record[12:], uint32(len(name)))
	copy(record[credentialWatchHeaderSize:], name)

	tests := []struct {
		name    string
		data    []byte
		want    []credentialRunEvent
		wantErr string
	}{
		{name: "valid record", data: record, want: []credentialRunEvent{{filepath.Join(root, "entry"), credentialCreate}}},
		{name: "short header", data: record[:4], wantErr: "short inotify header"},
		{name: "short record", data: record[:credentialWatchHeaderSize], wantErr: "short inotify record"},
		{name: "empty read", wantErr: "inotify read returned no data"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reads := 0
			observer := credentialEventObserver{
				watches: map[int]string{watchID: root},
				watchIO: credentialWatchIO{read: func(_ int, buffer []byte) (int, error) {
					reads++
					switch reads {
					case 1:
						return 0, syscall.EINTR
					case 2:
						return copy(buffer, tt.data), nil
					default:
						return 0, syscall.EAGAIN
					}
				}},
			}
			err := observer.read(make([]byte, len(record)))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("read error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil || !slices.Equal(observer.records, tt.want) {
				t.Fatalf("read records = %v, error = %v, want %v", observer.records, err, tt.want)
			}
		})
	}
}
