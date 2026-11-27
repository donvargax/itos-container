package features

import (
	"encoding/binary"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// The record layout and flags are part of the Linux inotify wire format.
const (
	credentialWatchHeaderSize = 16
	credentialWatchIgnored    = 0x8000
	credentialWatchDirectory  = 0x40000000
)

type credentialWatchIO struct {
	open     func() (int, error)
	addWatch func(int, string) (int, error)
	read     func(int, []byte) (int, error)
	poll     func(int) (int, error)
	close    func(int) error
}

type credentialEventObserver struct {
	watchIO   credentialWatchIO
	recursive bool
	fd        int
	watches   map[int]string
	records   []credentialRunEvent
	capture   chan chan credentialObserverSnapshot
	stop      chan struct{}
	done      chan struct{}
}

type credentialObserverSnapshot struct {
	events []credentialRunEvent
	err    error
}

func newCredentialRunObserver(checkout, control string) (credentialRunObserver, error) {
	return startCredentialRunObserver(credentialWatchSystem, true, checkout, control)
}

func newCredentialRunStagingObserver() (credentialRunObserver, error) {
	return startCredentialRunObserver(credentialWatchSystem, false, "/dev/shm")
}

func startCredentialRunObserver(watchIO credentialWatchIO, recursive bool, roots ...string) (credentialRunObserver, error) {
	fd, err := watchIO.open()
	if err != nil {
		return nil, err
	}
	o := &credentialEventObserver{
		watchIO:   watchIO,
		fd:        fd,
		recursive: recursive,
		watches:   make(map[int]string),
		capture:   make(chan chan credentialObserverSnapshot),
		stop:      make(chan struct{}),
		done:      make(chan struct{}),
	}
	for _, root := range roots {
		if recursive {
			err = o.watchTree(root, false)
		} else {
			err = o.watchDirectory(root)
		}
		if err != nil {
			return nil, errors.Join(err, watchIO.close(fd))
		}
	}
	go o.drain()
	return o, nil
}

func (o *credentialEventObserver) watchTree(root string, created bool) error {
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if created && os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if created && path != root {
			o.records = append(o.records, credentialRunEvent{path, credentialCreate})
		}
		if !entry.IsDir() {
			return nil
		}
		return o.watchDirectory(path)
	})
}

func (o *credentialEventObserver) watchDirectory(path string) error {
	wd, err := o.watchIO.addWatch(o.fd, path)
	if err == nil {
		o.watches[wd] = path
	}
	return err
}

func (o *credentialEventObserver) read(buffer []byte) error {
	for {
		n, err := o.watchIO.read(o.fd, buffer)
		if errors.Is(err, syscall.EINTR) {
			continue
		}
		if errors.Is(err, syscall.EAGAIN) {
			return nil
		}
		if err != nil {
			return err
		}
		for data := buffer[:n]; len(data) != 0; {
			if len(data) < credentialWatchHeaderSize {
				return errors.New("short inotify header")
			}
			word := binary.NativeEndian.Uint32
			wd, mask, size := int(int32(word(data))), word(data[4:]), word(data[12:])
			if uint64(size) > uint64(len(data)-credentialWatchHeaderSize) {
				return errors.New("short inotify record")
			}
			end := credentialWatchHeaderSize + int(size)
			name := strings.TrimRight(string(data[credentialWatchHeaderSize:end]), "\x00")
			data = data[end:]
			if mask == credentialOverflow {
				o.records = append(o.records, credentialRunEvent{kind: credentialOverflow})
				continue
			}
			path := filepath.Join(o.watches[wd], name)
			if mask&credentialWatchIgnored != 0 {
				delete(o.watches, wd)
				continue
			}
			o.records = append(o.records, credentialRunEvent{path, mask &^ credentialWatchDirectory})
			if o.recursive && mask&credentialWatchDirectory != 0 && mask&(credentialCreate|credentialMoveTo) != 0 {
				if err := o.watchTree(path, true); err != nil {
					return err
				}
			}
		}
	}
}

func (o *credentialEventObserver) drain() {
	defer close(o.done)
	buffer := make([]byte, 64*1024)
	var failure error
	for {
		select {
		case <-o.stop:
			return
		case reply := <-o.capture:
			if failure == nil {
				failure = o.read(buffer)
			}
			reply <- credentialObserverSnapshot{events: append([]credentialRunEvent(nil), o.records...), err: failure}
		default:
			if failure != nil {
				select {
				case <-o.stop:
					return
				case reply := <-o.capture:
					reply <- credentialObserverSnapshot{err: failure}
				}
				continue
			}
			n, err := o.watchIO.poll(o.fd)
			if err != nil && !errors.Is(err, syscall.EINTR) {
				failure = err
			}
			if n > 0 {
				failure = o.read(buffer)
			}
		}
	}
}

func (o *credentialEventObserver) events() ([]credentialRunEvent, error) {
	reply := make(chan credentialObserverSnapshot)
	o.capture <- reply
	result := <-reply
	return result.events, result.err
}

func (o *credentialEventObserver) close() error {
	close(o.stop)
	<-o.done
	return o.watchIO.close(o.fd)
}

func credentialRunSameDevice(a, b string) (bool, error) {
	left, leftErr := credentialRunDevice(a)
	right, rightErr := credentialRunDevice(b)
	return left == right, errors.Join(leftErr, rightErr)
}
