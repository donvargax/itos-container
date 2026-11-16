package disk

import (
	"context"
	policy "github.com/donvargax/itos-container/internal/engine/policy/port"
	readiness "github.com/donvargax/itos-container/internal/engine/readiness/domain"
	rp "github.com/donvargax/itos-container/internal/engine/readiness/port"
	"github.com/donvargax/itos-container/internal/engine/session/port"
	"strings"
)

// CheckedSources retains the source descriptors of slice-29's complete
// observation. Its FS is supplied by cmd, and it never reads source data.
// One instance belongs to one command and is serialized by the operation lock.
type CheckedSources struct {
	FS      rp.CredentialFS
	Input   rp.CredentialInput
	Grants  []policy.Credential
	held    *sourceCapture
	checked []port.CheckedCredential
}
type sourceCapture struct {
	rp.CredentialFS
	paths    map[rp.CredentialHandle]string
	selected map[string]rp.CredentialHandle
	wanted   map[string]bool
}

func (c *sourceCapture) Open(parent rp.CredentialHandle, name string) (rp.CredentialHandle, rp.CredentialNode, rp.CredentialRead) {
	h, n, status := c.CredentialFS.Open(parent, name)
	if status == rp.CredentialReadOK {
		path := name
		if parent != 0 {
			path = strings.TrimSuffix(c.paths[parent], "/") + "/" + name
		}
		c.paths[h] = path
		// Readiness rewalks each source. Retain its first descriptor so later
		// opens are closed normally instead of replacing and leaking the pin.
		if c.wanted[path] && c.selected[path] == 0 {
			c.selected[path] = h
		}
	}
	return h, n, status
}
func (c *sourceCapture) Close(h rp.CredentialHandle) {
	if c.selected[c.paths[h]] == h {
		return
	}
	c.CredentialFS.Close(h)
	delete(c.paths, h)
}
func (c *sourceCapture) release() {
	for _, h := range c.selected {
		c.CredentialFS.Close(h)
	}
	c.selected = nil
}
func (s *CheckedSources) Kinds() port.CredentialKinds {
	var kinds port.CredentialKinds
	for _, g := range s.Grants {
		if g.Kind == "claude-token" {
			kinds.Claude = true
		}
		if g.Kind == "github-token" {
			kinds.GitHub = true
		}
	}
	return kinds
}
func (s *CheckedSources) Revalidate(ctx context.Context) port.Outcome {
	if ctx.Err() != nil || s.FS == nil || len(s.Grants) == 0 || len(s.Grants) != len(s.Kinds().List()) {
		return port.CredentialRefused
	}
	capture := &sourceCapture{CredentialFS: s.FS, paths: map[rp.CredentialHandle]string{}, selected: map[string]rp.CredentialHandle{}, wanted: map[string]bool{}}
	for _, g := range s.Grants {
		capture.wanted[g.Source] = true
	}
	success := false
	defer func() {
		if !success {
			capture.release()
		}
	}()
	result := (readiness.Credentials{FS: capture, Input: s.Input}).Inspect(s.Grants)
	if len(result.Issues) != 0 || ctx.Err() != nil {
		return port.CredentialRefused
	}
	paths, ok := s.FS.(rp.CredentialReadPaths)
	if !ok {
		return port.CredentialUnavailable
	}
	checked := []port.CheckedCredential{}
	for _, g := range s.Grants {
		h := capture.selected[g.Source]
		node, ok := s.FS.Stat(h)
		path, accessible := paths.ReadPath(h)
		if !ok || !accessible || h == 0 {
			return port.CredentialRefused
		}
		checked = append(checked, port.CheckedCredential{Kind: g.Kind, ReadPath: path, Node: node})
	}
	if s.held != nil {
		if len(checked) != len(s.checked) {
			return port.CredentialRefused
		}
		for index, old := range s.checked {
			h := s.held.selected[s.Grants[index].Source]
			node, ok := s.FS.Stat(h)
			if !ok || node != old.Node || checked[index].Node != old.Node || checked[index].Kind != old.Kind {
				return port.CredentialRefused
			}
		}
		// Keep the originally checked descriptors, not the newly walked paths.
		capture.release()
		success = true
		return ""
	}
	s.held, s.checked = capture, checked
	success = true
	return ""
}
func (s *CheckedSources) Checked() []port.CheckedCredential {
	return append([]port.CheckedCredential(nil), s.checked...)
}
func (s *CheckedSources) Close() {
	if s.held != nil {
		s.held.release()
	}
	s.held = nil
	s.checked = nil
}
