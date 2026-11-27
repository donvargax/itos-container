package porttest

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/readiness/port"
)

type rawCredentialReporter = Reporter
type rawCredentialFixture = CredentialFiles
type rawCredentialBatch = CredentialBatch

func newRawCredentialFixture() *rawCredentialFixture { return CredentialFixture() }

func TestCredentialRawContract(t *testing.T) {
	runFakeCredentialRawContract(t, rawCredentialFake)
}

func rawCredentialFake(raw *CredentialFiles) credentialRawFS { return credentialRawFake{raw} }

func runFakeCredentialRawContract(t Reporter, files func(*CredentialFiles) credentialRawFS) {
	credentialRawContract(t, func(raw *CredentialFiles) (credentialRawFS, func() int) {
		return files(raw), func() int { return len(raw.handles) }
	})
}

// ReadPath names a live fake handle without exposing an openable descriptor path.
type credentialRawFake struct{ *CredentialFiles }

func (f credentialRawFake) ReadPath(h port.CredentialHandle) (string, bool) {
	if _, ok := f.handles[h]; !ok {
		return "", false
	}
	return "fake-fd/" + strconv.FormatUint(uint64(h), 10), true
}

func TestCredentialRawContractRejectsFaultyFakes(t *testing.T) {
	good := &credentialRawFailures{}
	runFakeCredentialRawContract(good, rawCredentialFake)
	if len(good.messages) != 0 {
		t.Fatalf("plain fake reported failures: %v", good.messages)
	}

	cases := []struct {
		name  string
		want  string
		files func(*CredentialFiles) credentialRawFS
	}{
		{"refused open returns a handle", "open refused: Open(", func(raw *CredentialFiles) credentialRawFS {
			return refusedOpenFault{rawCredentialFake(raw), func(_ port.CredentialHandle) (port.CredentialHandle, port.CredentialNode) {
				return 1 << 41, port.CredentialNode{}
			}}
		}},
		{"refused open returns metadata", "open refused: Open(", func(raw *CredentialFiles) credentialRawFS {
			return refusedOpenFault{rawCredentialFake(raw), func(parent port.CredentialHandle) (port.CredentialHandle, port.CredentialNode) {
				node, _ := raw.Stat(parent)
				return 0, node
			}}
		}},
		{"refused open keeps a pin", "open refused: retained 3 pins, want 2", func(raw *CredentialFiles) credentialRawFS {
			return refusedOpenFault{rawCredentialFake(raw), func(_ port.CredentialHandle) (port.CredentialHandle, port.CredentialNode) {
				raw.Open(0, "/")
				return 0, port.CredentialNode{}
			}}
		}},
		{"close keeps the pin", "walk: retained 3 pins, want 0", func(raw *CredentialFiles) credentialRawFS { return closePinFault{rawCredentialFake(raw)} }},
		{"closed handle still stats", "closed: Stat(", func(raw *CredentialFiles) credentialRawFS {
			return &closedStatFault{credentialRawFS: rawCredentialFake(raw), closed: map[port.CredentialHandle]port.CredentialNode{}}
		}},
		{"refusal carries a value", "identity refused: Descriptor(", func(raw *CredentialFiles) credentialRawFS { return descriptorValueFault{rawCredentialFake(raw)} }},
		{"partial batch survives failure", "after a partial batch: Entries(", func(raw *CredentialFiles) credentialRawFS {
			return &partialBatchFault{credentialRawFS: rawCredentialFake(raw), previous: map[port.CredentialHandle][]string{}}
		}},
		{"read path survives close", "closed: ReadPath(", func(raw *CredentialFiles) credentialRawFS {
			return &closedReadPathFault{credentialRawFS: rawCredentialFake(raw), closed: map[port.CredentialHandle]string{}}
		}},
		{"read paths alias", "walk: ReadPath(", func(raw *CredentialFiles) credentialRawFS { return aliasedReadPathFault{rawCredentialFake(raw)} }},
		{"read path exposes an original pathname", "walk: ReadPath returned original pathname", func(raw *CredentialFiles) credentialRawFS { return originalReadPathFault{rawCredentialFake(raw)} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			failures := &credentialRawFailures{}
			runFakeCredentialRawContract(failures, tc.files)
			for _, message := range failures.messages {
				if strings.HasPrefix(message, tc.want) {
					return
				}
			}
			t.Errorf("contract did not report %q for faulty fake, got %v", tc.want, failures.messages)
		})
	}
}

type credentialRawFailures struct{ messages []string }

func (*credentialRawFailures) Helper()               {}
func (*credentialRawFailures) Fatalf(string, ...any) { panic("raw contract must use Errorf") }
func (r *credentialRawFailures) Errorf(format string, args ...any) {
	r.messages = append(r.messages, fmt.Sprintf(format, args...))
}

type refusedOpenFault struct {
	credentialRawFS
	refusal func(port.CredentialHandle) (port.CredentialHandle, port.CredentialNode)
}

func (f refusedOpenFault) Open(parent port.CredentialHandle, name string) (port.CredentialHandle, port.CredentialNode, port.CredentialRead) {
	h, node, status := f.credentialRawFS.Open(parent, name)
	if status != port.CredentialReadOK {
		h, node = f.refusal(parent)
	}
	return h, node, status
}

type closePinFault struct{ credentialRawFS }

func (closePinFault) Close(port.CredentialHandle) {}

type closedStatFault struct {
	credentialRawFS
	closed map[port.CredentialHandle]port.CredentialNode
}

func (f *closedStatFault) Close(h port.CredentialHandle) {
	if node, ok := f.credentialRawFS.Stat(h); ok {
		f.closed[h] = node
	}
	f.credentialRawFS.Close(h)
}

func (f *closedStatFault) Stat(h port.CredentialHandle) (port.CredentialNode, bool) {
	if node, ok := f.closed[h]; ok {
		return node, true
	}
	return f.credentialRawFS.Stat(h)
}

type descriptorValueFault struct{ credentialRawFS }

func (f descriptorValueFault) Descriptor(h port.CredentialHandle) (port.CredentialDescriptor, bool) {
	descriptor, ok := f.credentialRawFS.Descriptor(h)
	if !ok {
		return port.CredentialDescriptor{Path: "/junk", MountID: 7}, false
	}
	return descriptor, true
}

type partialBatchFault struct {
	credentialRawFS
	previous map[port.CredentialHandle][]string
}

func (f *partialBatchFault) Entries(h port.CredentialHandle) ([]string, bool, bool) {
	names, done, ok := f.credentialRawFS.Entries(h)
	if !ok {
		return f.previous[h], false, false
	}
	f.previous[h] = names
	return names, done, true
}

type closedReadPathFault struct {
	credentialRawFS
	closed map[port.CredentialHandle]string
}

func (f *closedReadPathFault) Close(h port.CredentialHandle) {
	if path, ok := f.credentialRawFS.ReadPath(h); ok {
		f.closed[h] = path
	}
	f.credentialRawFS.Close(h)
}

func (f *closedReadPathFault) ReadPath(h port.CredentialHandle) (string, bool) {
	if path, ok := f.closed[h]; ok {
		return path, true
	}
	return f.credentialRawFS.ReadPath(h)
}

type aliasedReadPathFault struct{ credentialRawFS }

func (f aliasedReadPathFault) ReadPath(h port.CredentialHandle) (string, bool) {
	path, ok := f.credentialRawFS.ReadPath(h)
	if ok {
		return "fake-fd/shared", true
	}
	return path, false
}

type originalReadPathFault struct{ credentialRawFS }

func (f originalReadPathFault) ReadPath(h port.CredentialHandle) (string, bool) {
	descriptor, ok := f.Descriptor(h)
	return descriptor.Path, ok
}
