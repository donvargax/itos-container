package features

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/creack/pty"
)

// This helper can consent only to a full generation ID independently observed
// in this CI fixture. It never operates on the caller's account catalog.
func (w *world) homeNativeConsent(id, action string) homeNativeResult {
	r := homeNativeResult{exit: -1}
	if err := doctorRealGuard(); err != nil {
		r.err = err
		return r
	}
	if !offlineHex(id) || w.home.native.volumes["itos-container-home-"+id] == nil || (action != "--reset" && action != "--delete") ||
		!strings.HasPrefix(w.doctor.env["SLICE_21_ACCOUNT_HOME"], w.dir+string(filepath.Separator)) {
		r.err = errors.New("home consent fixture refused an untracked generation or non-isolated account")
		return r
	}
	trace, err := w.recoveryTraceBinary()
	if err != nil {
		r.err = err
		return r
	}
	master, slave, err := pty.Open()
	if err != nil {
		r.err = err
		return r
	}
	defer func() { _ = master.Close() }()
	defer func() { _ = slave.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	r.journal = filepath.Join(w.dir, "slice-34-consent-"+strconv.FormatInt(time.Now().UnixNano(), 10)+".jsonl")
	cmd := exec.CommandContext(ctx, trace, r.journal, r.journal+".pid", w.doctor.host, w.bin, "stop", "--home", id, action, "--json")
	cmd.Dir, cmd.Env = w.dir, w.env()
	var output, terminal offlineBuffer
	cmd.Stdin, cmd.Stdout, cmd.Stderr = slave, &output, slave
	cmd.WaitDelay = time.Second
	if err := cmd.Start(); err != nil {
		r.err = err
		return r
	}
	_ = slave.Close()
	drained := make(chan struct{})
	go func() { _, _ = io.Copy(&terminal, master); close(drained) }()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	answered := false
waiting:
	for {
		select {
		case err = <-done:
			break waiting
		case <-tick.C:
			text := string(terminal.bytes())
			if !answered && strings.Contains(text, "y/N") {
				r.review = text
				_, writeErr := master.Write([]byte("y\n"))
				r.err = errors.Join(r.err, writeErr)
				answered = true
			}
		}
	}
	<-drained
	r.exit = cmd.ProcessState.ExitCode()
	r.out, r.errout = output.bytes(), terminal.bytes()
	var exited *exec.ExitError
	if err != nil && !errors.As(err, &exited) {
		r.err = errors.Join(r.err, err)
	}
	if ctx.Err() != nil {
		r.err = errors.Join(r.err, errors.New("terminal home management exceeded its fixture deadline"))
	}
	return r
}

// Check actual successful fsync observations for every changed control file,
// without requiring a particular home-catalog filename or on-disk JSON layout.
func (w *world) homeNativeDurable(result homeNativeResult, before map[string]runEntry) error {
	after, err := runTree(w.doctor.env["SLICE_21_ACCOUNT_HOME"])
	if err != nil {
		return err
	}
	events, err := recoveryReadSyncs(result.journal)
	if err != nil {
		return err
	}
	changed := 0
	for path, entry := range after {
		if !entry.Mode.IsRegular() || reflect.DeepEqual(entry, before[path]) {
			continue
		}
		// Empty lock files contain no completion record. Their presence cannot
		// stand in for a durable generation/binding publication.
		if entry.Data == "" {
			continue
		}
		changed++
		final, err := sessionReadRecord(path)
		if err != nil {
			return err
		}
		if err := homeNativeSynced(final, path, events); err != nil {
			return err
		}
	}
	if changed == 0 {
		return errors.New("home retirement published no durable control-state change")
	}
	return nil
}

func homeNativeSynced(final sessionRecord, path string, events []recoverySync) error {
	if final.Mode != 0600 || final.Bytes == "" {
		return errors.New("home completion did not retain protected control metadata")
	}
	fileSynced := false
	for _, event := range events {
		if !event.Directory && event.Inode == final.Inode && event.Data == final.Bytes {
			fileSynced = true
		}
		if fileSynced && event.Directory && event.Path == filepath.Dir(path) && event.Files[filepath.Base(path)] == final.Inode {
			return nil
		}
	}
	return fmt.Errorf("home completion %s lacks successful file and publication-directory fsync evidence", filepath.Base(path))
}
