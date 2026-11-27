package features

import (
	"encoding/binary"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

type credentialLinuxObserver struct {
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
	return startCredentialRunObserver(true, checkout, control)
}

func newCredentialRunStagingObserver() (credentialRunObserver, error) {
	return startCredentialRunObserver(false, "/dev/shm")
}

func startCredentialRunObserver(recursive bool, roots ...string) (credentialRunObserver, error) {
	fd, err := unix.InotifyInit1(unix.IN_NONBLOCK | unix.IN_CLOEXEC)
	if err != nil {
		return nil, err
	}
	o := &credentialLinuxObserver{fd: fd, recursive: recursive, watches: map[int]string{}, capture: make(chan chan credentialObserverSnapshot), stop: make(chan struct{}), done: make(chan struct{})}
	for _, root := range roots {
		if recursive {
			err = o.watchTree(root, false)
		} else {
			err = o.watchDirectory(root)
		}
		if err != nil {
			return nil, errors.Join(err, unix.Close(fd))
		}
	}
	go o.drain()
	return o, nil
}

func (o *credentialLinuxObserver) watchTree(root string, created bool) error {
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

func (o *credentialLinuxObserver) watchDirectory(path string) error {
	wd, err := unix.InotifyAddWatch(o.fd, path, unix.IN_CREATE|unix.IN_MODIFY|unix.IN_ATTRIB|unix.IN_CLOSE_WRITE|unix.IN_MOVED_FROM|unix.IN_MOVED_TO|unix.IN_DELETE|unix.IN_DELETE_SELF|unix.IN_MOVE_SELF|unix.IN_ONLYDIR|unix.IN_DONT_FOLLOW)
	if err == nil {
		o.watches[wd] = path
	}
	return err
}

func (o *credentialLinuxObserver) read(buffer []byte) error {
	for {
		n, err := unix.Read(o.fd, buffer)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if errors.Is(err, unix.EAGAIN) {
			return nil
		}
		if err != nil {
			return err
		}
		for data := buffer[:n]; len(data) >= unix.SizeofInotifyEvent; {
			word := binary.NativeEndian.Uint32
			wd, mask, size := int(int32(word(data))), word(data[4:]), int(word(data[12:]))
			if size > len(data)-unix.SizeofInotifyEvent {
				return errors.New("short inotify record")
			}
			name := strings.TrimRight(string(data[unix.SizeofInotifyEvent:unix.SizeofInotifyEvent+size]), "\x00")
			data = data[unix.SizeofInotifyEvent+size:]
			if mask == unix.IN_Q_OVERFLOW {
				o.records = append(o.records, credentialRunEvent{kind: credentialOverflow})
				continue
			}
			path := filepath.Join(o.watches[wd], name)
			if mask&unix.IN_IGNORED != 0 {
				delete(o.watches, wd)
				continue
			}
			o.records = append(o.records, credentialRunEvent{path, mask &^ unix.IN_ISDIR})
			if o.recursive && mask&unix.IN_ISDIR != 0 && mask&(unix.IN_CREATE|unix.IN_MOVED_TO) != 0 {
				if err := o.watchTree(path, true); err != nil {
					return err
				}
			}
		}
	}
}

func (o *credentialLinuxObserver) drain() {
	defer close(o.done)
	buffer := make([]byte, 64*1024)
	fds := []unix.PollFd{{Fd: int32(o.fd), Events: unix.POLLIN}}
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
			n, err := unix.Poll(fds, 10)
			if err != nil && !errors.Is(err, unix.EINTR) {
				failure = err
			}
			if n > 0 {
				failure = o.read(buffer)
			}
		}
	}
}

func (o *credentialLinuxObserver) events() ([]credentialRunEvent, error) {
	reply := make(chan credentialObserverSnapshot)
	o.capture <- reply
	result := <-reply
	return result.events, result.err
}

func (o *credentialLinuxObserver) close() error {
	close(o.stop)
	<-o.done
	return unix.Close(o.fd)
}

func credentialRunSameDevice(a, b string) (bool, error) {
	var left, right unix.Stat_t
	err := errors.Join(unix.Stat(a, &left), unix.Stat(b, &right))
	return left.Dev == right.Dev, err
}
