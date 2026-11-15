package porttest

import (
	"context"
	"github.com/donvargax/itos-container/internal/engine/session/port"
	"slices"
	"strconv"
	"strings"
)

// CredentialStager models pathnames independently of acquired runtime references.
// Sources and files are independent byte storage. No fake performs host I/O.
type CredentialStager struct {
	SourceData  map[string][]byte
	Files       map[string][]byte
	Directories map[string]port.CredentialStaging
	Fault       string
	Events      []string
	Before      func(string)
}

func (s *CredentialStager) event(name string) {
	s.Events = append(s.Events, name)
	if s.Before != nil {
		s.Before(name)
	}
}
func (s *CredentialStager) Stage(ctx context.Context, r port.Record, sources port.CredentialSources, publish func(port.CredentialStaging) port.Outcome) port.Outcome {
	if r.Credentials == (port.CredentialStaging{}) && sources == nil {
		return ""
	}
	if !r.Credentials.Valid() || r.Credentials.Stage != "intent" || sources == nil || sources.Kinds() != r.Credentials.Kinds {
		return port.CredentialRefused
	}
	s.event("revalidate")
	if f := sources.Revalidate(ctx); f != "" {
		return f
	}
	if s.Fault == "non-tmpfs" {
		return port.CredentialUnavailable
	}
	if ctx.Err() != nil {
		return port.CredentialUnavailable
	}
	s.event("mkdir")
	if s.Directories == nil {
		s.Directories = map[string]port.CredentialStaging{}
	}
	if _, exists := s.Directories[r.Credentials.Directory]; exists || s.Fault == "mkdir" {
		return port.CredentialRefused
	}
	staging := r.Credentials
	staging.Stage = "owned"
	staging.Device = "1"
	staging.Inode = strconv.Itoa(100 + len(s.Directories))
	s.Directories[staging.Directory] = staging
	if f := publish(staging); f != "" {
		return f
	}
	if s.Files == nil {
		s.Files = map[string][]byte{}
	}
	for _, source := range sources.Checked() {
		s.event("read")
		if ctx.Err() != nil {
			return port.CredentialUnavailable
		}
		data, ok := s.SourceData[source.ReadPath]
		if !ok || len(data) < 1 || len(data) > 65536 || (s.Fault == "read" || s.Fault == "short read") {
			return port.CredentialRefused
		}
		path := staging.Directory + "/" + source.Kind
		if _, exists := s.Files[path]; exists {
			return port.CredentialRefused
		}
		index := port.CredentialIndex(source.Kind)
		if index < 0 {
			return port.CredentialRefused
		}
		s.Files[path] = []byte{}
		staging.Files[index] = "1:" + strconv.Itoa(200+index)
		s.Directories[staging.Directory] = staging
		if f := publish(staging); f != "" {
			return f
		}
		s.event("copy")
		if ctx.Err() != nil {
			return port.CredentialUnavailable
		}
		copy := slices.Clone(data)
		if s.Fault == "trim newline" && len(copy) > 0 && copy[len(copy)-1] == '\n' {
			copy = copy[:len(copy)-1]
		}
		if s.Fault == "write" || s.Fault == "short write" {
			s.Files[path] = copy[:1]
			return port.CredentialUnavailable
		}
		s.Files[path] = copy
	}
	return ""
}
func (s *CredentialStager) Observe(ctx context.Context, r port.Record) (string, port.Outcome) {
	s.event("observe")
	if ctx.Err() != nil || s.Fault == "observe" {
		return "unknown", port.CredentialUnavailable
	}
	got, exists := s.Directories[r.Credentials.Directory]
	if !exists {
		return "absent", ""
	}
	if got != r.Credentials || !r.Credentials.Valid() || r.Credentials.Stage == "absent" || s.Fault == "owner" || s.Fault == "mode" || s.Fault == "symlink" {
		return "unknown", port.CredentialRefused
	}
	for name := range s.Files {
		if strings.HasPrefix(name, r.Credentials.Directory+"/") {
			kind := strings.TrimPrefix(name, r.Credentials.Directory+"/")
			index := port.CredentialIndex(kind)
			if index < 0 || r.Credentials.Files[index] == "" {
				return "unknown", port.CredentialRefused
			}
		}
	}
	return "present", ""
}
func (s *CredentialStager) Remove(ctx context.Context, r port.Record) port.Outcome {
	presence, f := s.Observe(ctx, r)
	if f != "" || presence == "absent" {
		return f
	}
	failure := port.Outcome("")
	for _, m := range r.Credentials.Mounts() {
		s.event("unlink")
		if ctx.Err() != nil {
			return port.CredentialUnavailable
		}
		if s.Fault == "unlink" {
			failure = port.Incomplete
			continue
		}
		delete(s.Files, m.Source)
	}
	if failure != "" {
		return failure
	}
	delete(s.Directories, r.Credentials.Directory)
	return ""
}
