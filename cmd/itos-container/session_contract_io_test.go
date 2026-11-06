package main

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
)

// Adapter fault callbacks execute the real operation except for the explicitly
// named fault. They record observations, never decide a session outcome.
type sessionContractIO struct {
	fault, stage, bytes, journal string
	events                       []sessionContractEvent
}

func (s *sessionContractIO) observe(operation string, err error) error {
	event := sessionContractEvent{Operation: operation, Stage: s.stage, Bytes: s.bytes, Failed: err != nil}
	s.events = append(s.events, event)
	file, openErr := os.OpenFile(s.journal, os.O_WRONLY|os.O_APPEND, 0600)
	if openErr != nil {
		return errors.Join(err, openErr)
	}
	writeErr := json.NewEncoder(file).Encode(event)
	return errors.Join(err, writeErr, file.Close())
}

func (s *sessionContractIO) create(root *os.Root, name string) (*os.File, error) {
	if s.fails("private file creation") {
		return nil, s.observe("private file creation", errors.New("session fixture create fault"))
	}
	file, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	return file, s.observe("private file creation", err)
}

func (s *sessionContractIO) write(file *os.File, data []byte) (int, error) {
	var record struct {
		Stage string `json:"stage"`
	}
	if err := json.Unmarshal(data, &record); err != nil {
		return 0, err
	}
	s.stage, s.bytes = record.Stage, string(data)
	if s.fails("short write") {
		n, err := file.Write(data[:len(data)-1])
		return n, s.observe("write", errors.Join(err, errors.New("session fixture short write")))
	}
	n, err := file.Write(data)
	return n, s.observe("write", err)
}

func (s *sessionContractIO) fileSync(file *os.File) error {
	if s.stage == "" {
		data := make([]byte, 65537)
		n, err := file.ReadAt(data, 0)
		if err != nil && !errors.Is(err, io.EOF) {
			return s.observe("file fsync", err)
		}
		var record struct {
			Stage string `json:"stage"`
		}
		if err = json.Unmarshal(data[:n], &record); err != nil {
			return s.observe("file fsync", err)
		}
		s.stage, s.bytes = record.Stage, string(data[:n])
	}

	if s.fails("file fsync") {
		return s.observe("file fsync", errors.New("session fixture fsync fault"))
	}
	return s.observe("file fsync", file.Sync())
}

func (s *sessionContractIO) rename(root *os.Root, before, after string) error {
	if s.fails("atomic publication") {
		return s.observe("atomic publication", errors.New("session fixture publication fault"))
	}
	return s.observe("atomic publication", root.Rename(before, after))
}

func (s *sessionContractIO) parentSync(file *os.File) error {
	if s.fails("parent fsync") {
		return s.observe("parent fsync", errors.New("session fixture parent sync fault"))
	}
	return s.observe("parent fsync", file.Sync())
}

func (s *sessionContractIO) directorySync(file *os.File) error {
	if s.fails("new ancestor fsync") {
		return s.observe("new ancestor fsync", errors.New("session fixture ancestor sync fault"))
	}
	return s.observe("new ancestor fsync", file.Sync())
}

func (s *sessionContractIO) fails(operation string) bool {
	prefix := s.stage
	if prefix == "" {
		prefix = "intent"
	}
	if prefix == "owned" || prefix == "absent" {
		prefix += "-record"
	}
	return s.fault == prefix+" "+operation || (s.stage == "owned" && operation == "file fsync" && strings.HasPrefix(s.fault, "owned-record file fsync then "))
}

func (s *sessionContractIO) exclusive(root *os.Root, before, after string) error {
	if s.fails("atomic publication") {
		return s.observe("atomic publication", errors.New("session fixture publication fault"))
	}
	return s.observe("atomic publication", root.Link(before, after))
}
