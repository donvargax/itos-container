package porttest

import (
	"bytes"
	"context"
	"github.com/donvargax/itos-container/internal/engine/session/port"
	"strings"
)

type CredentialSourceFixture struct {
	Entries               []port.CheckedCredential
	Failure               port.Outcome
	Revalidations, Closes int
}

func (s *CredentialSourceFixture) Kinds() port.CredentialKinds {
	var kinds port.CredentialKinds
	for _, entry := range s.Entries {
		if entry.Kind == "claude-token" {
			kinds.Claude = true
		}
		if entry.Kind == "github-token" {
			kinds.GitHub = true
		}
	}
	return kinds
}
func (s *CredentialSourceFixture) Revalidate(context.Context) port.Outcome {
	s.Revalidations++
	return s.Failure
}
func (s *CredentialSourceFixture) Checked() []port.CheckedCredential {
	return append([]port.CheckedCredential(nil), s.Entries...)
}
func (s *CredentialSourceFixture) Close() { s.Closes++ }

type CredentialFixture struct {
	UID     string
	Fault   func(string) port.CredentialStager
	Stager  port.CredentialStager
	Sources *CredentialSourceFixture
	Read    func(string) []byte
	Present func(string) bool
}

func (f CredentialFixture) Intent() port.Record {
	r := CredentialIntent(f.Sources.Kinds())
	if f.UID != "" {
		r.UID = f.UID
	}
	return r
}

func CredentialIntent(kinds port.CredentialKinds) port.Record {
	b, _, _ := Fixture()
	r := RunIntent(b)
	r.Schema, r.Stage = 4, "reserved"
	r.Credentials = port.CredentialStaging{Directory: "/dev/shm/itos-container-" + strings.Repeat("a", 32), Stage: "intent", Kinds: kinds}
	return r
}

// CredentialStagingContract uses actual observed bytes as its oracle. A
// reporter can run it against deliberately faulty implementations as a control.
func CredentialStagingContract(t Reporter, create func(map[string][]byte) CredentialFixture) {
	t.Helper()
	for _, kinds := range []port.CredentialKinds{{Claude: true}, {GitHub: true}, {Claude: true, GitHub: true}} {
		for _, size := range []int{1, 65536} {
			data := bytes.Repeat([]byte{'x'}, size)
			data[size-1] = '\n'
			values := map[string][]byte{}
			for _, kind := range kinds.List() {
				values[kind] = data
			}
			f := create(values)
			r := f.Intent()
			publications := 0
			failure := f.Stager.Stage(context.Background(), r, f.Sources, func(s port.CredentialStaging) port.Outcome { publications++; r.Credentials = s; return "" })
			if failure != "" {
				t.Errorf("credential copy positive control refused: %s", failure)
				continue
			}
			if publications < 1 || r.Credentials.Stage != "owned" || r.Credentials.Device == "" || r.Credentials.Inode == "" || f.Sources.Revalidations != 1 {
				t.Errorf("copy lacks revalidation and published directory identity")
			}
			for _, m := range r.Credentials.Mounts() {
				if !bytes.Equal(f.Read(m.Source), data) {
					t.Errorf("credential copy changed bytes for kind %s", m.Kind)
				}
				if m.Target != "/run/itos-container/credentials/"+m.Kind {
					t.Errorf("credential target changed")
				}
			}
			if presence, fail := f.Stager.Observe(context.Background(), r); fail != "" || presence != "present" {
				t.Errorf("staging presence is not independently observable: %s %s", presence, fail)
			}
			if fail := f.Stager.Remove(context.Background(), r); fail != "" {
				t.Errorf("owned staging cleanup failed: %s", fail)
			}
			if presence, fail := f.Stager.Observe(context.Background(), r); fail != "" || presence != "absent" || f.Present(r.Credentials.Directory) {
				t.Errorf("cleanup did not confirm path absence")
			}
			if fail := f.Stager.Remove(context.Background(), r); fail != "" {
				t.Errorf("repeated cleanup failed: %s", fail)
			}
		}
	}
	for _, fault := range []string{"non-tmpfs", "occupied", "read", "short read", "write", "short write"} {
		fixture := create(map[string][]byte{"claude-token": []byte("synthetic\n"), "github-token": []byte("another\n")})
		stager := fixture.Fault(fault)
		record := fixture.Intent()
		failure := stager.Stage(context.Background(), record, fixture.Sources, func(staging port.CredentialStaging) port.Outcome { record.Credentials = staging; return "" })
		if failure == "" {
			t.Errorf("staging fault accepted: %s", fault)
		}
		if record.Credentials.Stage == "owned" {
			if f := stager.Remove(context.Background(), record); f != "" {
				t.Errorf("partial staging lost cleanup: %s", fault)
			}
		}
	}
	for _, kinds := range []port.CredentialKinds{{Claude: true}, {GitHub: true}, {Claude: true, GitHub: true}} {
		values := map[string][]byte{}
		for _, kind := range kinds.List() {
			values[kind] = []byte("synthetic\n")
		}
		fixture := create(values)
		r := fixture.Intent()
		publications := 0
		fail := fixture.Stager.Stage(context.Background(), r, fixture.Sources, func(next port.CredentialStaging) port.Outcome {
			publications++
			if next.Stage != "owned" || next.Device == "" || next.Inode == "" || next.Files != [2]string{} || !fixture.Present(next.Directory) {
				t.Errorf("directory publication lacks its created identity before file creation")
			}
			return port.IntentWrite
		})
		if fail != port.IntentWrite || publications != 1 {
			t.Errorf("directory publication failure lost its primary outcome")
		}
		if presence, fail := fixture.Stager.Observe(context.Background(), r); fail != "" || presence != "absent" || fixture.Present(r.Credentials.Directory) {
			t.Errorf("unpublished directory remained outside durable cleanup authority")
		}
		for range 2 {
			if fail := fixture.Stager.Remove(context.Background(), r); fail != "" {
				t.Errorf("intent-only recovery could not confirm unpublished directory absence")
			}
		}
	}
	f := create(map[string][]byte{"claude-token": []byte("synthetic\n")})
	f.Sources.Failure = port.CredentialRefused
	r := f.Intent()
	if fail := f.Stager.Stage(context.Background(), r, f.Sources, func(port.CredentialStaging) port.Outcome {
		t.Errorf("revalidation refusal published staging")
		return ""
	}); fail != port.CredentialRefused || f.Present(r.Credentials.Directory) {
		t.Errorf("refused source caused staging effects")
	}
}

func MemoryCredentialFixture(values map[string][]byte) CredentialFixture {
	s := &CredentialStager{Files: map[string][]byte{}, Directories: map[string]port.CredentialStaging{}, SourceData: map[string][]byte{}}
	sources := &CredentialSourceFixture{}
	for i, kind := range []string{"claude-token", "github-token"} {
		if data, ok := values[kind]; ok {
			path := "/proc/self/fd/" + []string{"10", "11"}[i]
			s.SourceData[path] = append([]byte(nil), data...)
			sources.Entries = append(sources.Entries, port.CheckedCredential{Kind: kind, ReadPath: path})
		}
	}
	return CredentialFixture{Fault: func(fault string) port.CredentialStager {
		if fault == "occupied" {
			r := CredentialIntent(sources.Kinds())
			s.Directories[r.Credentials.Directory] = r.Credentials
		} else {
			s.Fault = fault
		}
		return s
	}, Stager: s, Sources: sources, Read: func(path string) []byte { return s.Files[path] }, Present: func(path string) bool { _, ok := s.Directories[path]; return ok }}
}
