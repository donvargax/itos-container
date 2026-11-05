package features

import (
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

type sessionReadWatch struct {
	file  *os.File
	paths map[int32]string
}

func (w *world) sessionWatchInputs() (*sessionReadWatch, error) {
	fd, err := sessionInotifyInit()
	if err != nil {
		return nil, err
	}
	watch := &sessionReadWatch{file: os.NewFile(uintptr(fd), "session-input-watch"), paths: map[int32]string{}}
	for _, path := range []string{filepath.Join(w.doctor.checkout, "itos-container.yaml"), filepath.Join(w.doctor.checkout, ".devcontainer/devcontainer.json"), w.approval.record} {
		id, e := sessionInotifyAdd(fd, path)
		if e != nil {
			_ = watch.file.Close()
			return nil, e
		}
		watch.paths[int32(id)] = path
	}
	return watch, nil
}
func (watch *sessionReadWatch) finish() ([]string, error) {
	defer func() { _ = watch.file.Close() }()
	var result []string
	var buffer [4096]byte
	for {
		n, err := sessionInotifyRead(int(watch.file.Fd()), buffer[:])
		if errors.Is(err, syscall.EAGAIN) {
			return result, nil
		}
		if err != nil {
			return result, err
		}
		if n == 0 {
			return result, nil
		}
		for offset := 0; offset+16 <= n; {
			id := int32(binary.NativeEndian.Uint32(buffer[offset:]))
			size := binary.NativeEndian.Uint32(buffer[offset+12:])
			result = append(result, watch.paths[id])
			offset += 16 + int(size)
		}
	}
}
