package features

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/cucumber/godog"
)

type credentialWatch struct {
	fd      int
	control int
	closed  bool
}

func (o *credentialWatch) close() {
	if !o.closed {
		credentialWatchClose(o.fd)
		o.closed = true
	}
}

func (w *world) credentialObserverSteps(sc *godog.ScenarioContext) {
	sc.Step(`^credential source data access is observed with an independent read control$`, func() error {
		fd, err := credentialWatchOpen()
		if err != nil {
			return err
		}
		o := &credentialWatch{fd: fd}
		w.credentials.watch = o
		for _, path := range w.credentials.paths {
			info, err := os.Lstat(path)
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return err
			}
			if info.Mode().IsRegular() {
				if _, err := credentialWatchAdd(fd, path); err != nil {
					return err
				}
			}
		}
		control := filepath.Join(w.policy.host, "slice-29-read-control")
		if err := os.WriteFile(control, []byte("observer control"), 0o600); err != nil {
			return err
		}
		o.control, err = credentialWatchAdd(fd, control)
		if err != nil {
			return err
		}
		_, err = os.ReadFile(control)
		return err
	})
	sc.Step(`^the source observer saw its control read and no credential data open or read$`, func() error {
		o := w.credentials.watch
		data, err := credentialWatchRead(o.fd)
		o.close()
		if err != nil {
			return err
		}
		control := false
		for len(data) >= 16 {
			wd := int(int32(binary.NativeEndian.Uint32(data)))
			size := int(binary.NativeEndian.Uint32(data[12:]))
			if len(data) < 16+size {
				return fmt.Errorf("truncated inotify event")
			}
			if wd != o.control {
				return fmt.Errorf("credential source data was opened or read")
			}
			control = true
			data = data[16+size:]
		}
		if !control {
			return fmt.Errorf("independent read did not reach the observer")
		}
		return nil
	})
	sc.Step(`^no credential marker or content hash appears in inspection output checkout or state$`, func() error {
		var needles []string
		for _, marker := range w.credentials.markers {
			hash := sha256.Sum256([]byte(marker))
			needles = append(needles, marker, hex.EncodeToString(hash[:]))
			// Exclude the two edge groups whose bits depend on neighboring bytes.
			for offset := 0; offset < 3; offset++ {
				text := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("x", offset) + marker))
				needles = append(needles, text[4:len(text)-4])
			}
		}
		if len(needles) == 0 {
			return fmt.Errorf("marker control needs a synthetic source")
		}
		control := strings.Join(needles, "\n")
		controlPath := filepath.Join(w.policy.host, "slice-29-marker-control")
		if err := os.WriteFile(controlPath, []byte(control), 0o600); err != nil {
			return err
		}
		bytes, err := os.ReadFile(controlPath)
		if err != nil {
			return err
		}
		check := func(data string) error {
			for _, n := range needles {
				if strings.Contains(data, n) {
					return fmt.Errorf("synthetic credential material escaped inspection")
				}
			}
			return nil
		}
		for _, n := range needles {
			if !strings.Contains(string(bytes), n) {
				return fmt.Errorf("marker search control did not contain an encoding")
			}
		}
		if err := check(w.stdout + w.stderr); err != nil {
			return err
		}
		for _, root := range []string{w.manifest.checkout, w.credentials.state} {
			err := filepath.WalkDir(root, func(path string, e fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if !e.Type().IsRegular() {
					return nil
				}
				data, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				return check(string(data))
			})
			if err != nil {
				return err
			}
		}
		return nil
	})
}
