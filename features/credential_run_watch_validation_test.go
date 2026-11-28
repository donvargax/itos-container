package features

import (
	"encoding/binary"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
)

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
