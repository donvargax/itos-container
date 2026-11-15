package disk

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"github.com/donvargax/itos-container/internal/engine/session/port"
	f "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type credentialStatInfo struct {
	os.FileInfo
	raw any
}

func (i credentialStatInfo) Sys() any { return i.raw }

func TestCredentialMetadataWidths(t *testing.T) {
	info, err := os.Stat(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []any{
		struct{ Dev, Ino, Nlink, Uid, Mode uint64 }{1, 2, 1, 1 << 32, 0100400},
		struct{ Dev, Ino, Nlink, Uid, Mode uint64 }{1, 2, 1, 1000, 1 << 32},
	} {
		if _, ok := credentialNode(credentialStatInfo{info, raw}); ok {
			t.Fatal("out-of-range ownership or mode metadata was narrowed")
		}
	}
}

func stageCredentialFixture(t *testing.T, fixture f.CredentialFixture) (port.Record, port.Outcome) {
	t.Helper()
	r := fixture.Intent()
	fail := fixture.Stager.Stage(t.Context(), r, fixture.Sources, func(s port.CredentialStaging) port.Outcome { r.Credentials = s; return "" })
	return r, fail
}
func TestCredentialStagingRefusals(t *testing.T) {
	for _, fault := range []string{"non-tmpfs", "occupied", "directory symlink", "short read", "read error", "short write", "write error", "metadata", "publish", "unsupported"} {
		t.Run(fault, func(t *testing.T) {
			fixture := diskCredentialFixture(t, map[string][]byte{"claude-token": []byte("synthetic\n")})
			s := fixture.Stager.(Credentials)
			r := fixture.Intent()
			publish := func(next port.CredentialStaging) port.Outcome { r.Credentials = next; return "" }
			switch fault {
			case "non-tmpfs":
				s.tmpfs = func(string) bool { return false }
			case "occupied":
				if err := os.Mkdir(filepath.Join(s.root, filepath.Base(r.Credentials.Directory)), 0700); err != nil {
					t.Fatal(err)
				}
			case "directory symlink":
				if err := os.Symlink(t.TempDir(), filepath.Join(s.root, filepath.Base(r.Credentials.Directory))); err != nil {
					t.Fatal(err)
				}
			case "short read":
				s.read = func(*os.File, []byte) (int, error) { return 1, nil }
			case "read error":
				s.read = func(*os.File, []byte) (int, error) { return 0, errors.New("synthetic read failure") }
			case "short write":
				s.write = func(*os.File, []byte) (int, error) { return 1, nil }
			case "write error":
				s.write = func(*os.File, []byte) (int, error) { return 0, errors.New("synthetic write failure") }
			case "metadata":
				fixture.Sources.Entries[0].Node.Size++
			case "publish":
				publish = func(port.CredentialStaging) port.Outcome { return port.IntentWrite }
			case "unsupported":
				s.OS = "darwin"
			}
			fail := s.Stage(t.Context(), r, fixture.Sources, publish)
			if fail == "" {
				t.Fatal("unsafe credential preparation succeeded")
			}
			if r.Credentials.Stage == "owned" {
				if cleanup := s.Remove(t.Context(), r); cleanup != "" {
					t.Fatal("partial staging lost safe cleanup", cleanup)
				}
			}
		})
	}
}

func TestCredentialFilePublicationFailure(t *testing.T) {
	for _, kinds := range []port.CredentialKinds{{Claude: true}, {GitHub: true}, {Claude: true, GitHub: true}} {
		for _, failedKind := range kinds.List() {
			t.Run(strings.Join(kinds.List(), "+")+"/"+failedKind, func(t *testing.T) {
				values := map[string][]byte{}
				for _, kind := range kinds.List() {
					values[kind] = []byte("synthetic\n")
				}
				fixture := diskCredentialFixture(t, values)
				s := fixture.Stager.(Credentials)
				r := fixture.Intent()
				index := port.CredentialIndex(failedKind)
				path := filepath.Join(s.root, filepath.Base(r.Credentials.Directory), failedKind)
				reached := false
				fail := s.Stage(t.Context(), r, fixture.Sources, func(next port.CredentialStaging) port.Outcome {
					if next.Files[index] != "" {
						reached = true
						info, err := os.Stat(path)
						if err != nil || info.Size() != 0 {
							t.Fatal("file identity was not published before copying")
						}
						return port.IntentWrite
					}
					r.Credentials = next
					return ""
				})
				if !reached || fail != port.IntentWrite {
					t.Fatal("file publication failure lost its primary outcome", fail)
				}
				if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
					t.Error("unpublished file remained outside durable cleanup authority")
				}
				if kinds.Claude && failedKind == "github-token" && !bytes.Equal(fixture.Read(r.Credentials.Directory+"/claude-token"), values["claude-token"]) {
					t.Fatal("earlier published copy was lost before owned cleanup")
				}
				for range 2 {
					if fail := s.Remove(t.Context(), r); fail != "" {
						t.Fatal("durable recovery cannot remove partial staging", fail)
					}
				}
				if presence, fail := s.Observe(t.Context(), r); fail != "" || presence != "absent" {
					t.Fatal("partial staging cleanup did not confirm absence")
				}
			})
		}
	}
}

func TestCredentialUnpublishedFileSubstitution(t *testing.T) {
	for _, substitution := range []string{"file", "symlink", "missing"} {
		t.Run(substitution, func(t *testing.T) {
			fixture := diskCredentialFixture(t, map[string][]byte{"claude-token": []byte("synthetic\n")})
			s := fixture.Stager.(Credentials)
			r := fixture.Intent()
			path := filepath.Join(s.root, filepath.Base(r.Credentials.Directory), "claude-token")
			retained := filepath.Join(t.TempDir(), "retained")
			reached := false
			fail := s.Stage(t.Context(), r, fixture.Sources, func(next port.CredentialStaging) port.Outcome {
				if next.Files[0] == "" {
					r.Credentials = next
					return ""
				}
				reached = true
				if err := os.Rename(path, retained); err != nil {
					t.Fatal(err)
				}
				switch substitution {
				case "file":
					if err := os.WriteFile(path, []byte("replacement"), 0400); err != nil {
						t.Fatal(err)
					}
				case "symlink":
					if err := os.Symlink(retained, path); err != nil {
						t.Fatal(err)
					}
				}
				return port.IntentWrite
			})
			if !reached || fail != port.IntentWrite {
				t.Fatal("substitution changed the publication failure", fail)
			}
			if _, err := os.Stat(retained); err != nil {
				t.Fatal("cleanup removed the retained descriptor's other name")
			}
			if substitution == "missing" {
				if fail := s.Remove(t.Context(), r); fail != "" {
					t.Fatal("already absent unpublished file blocked cleanup", fail)
				}
			} else {
				if _, err := os.Lstat(path); err != nil {
					t.Fatal("publication cleanup removed a substituted entry")
				}
				if fail := s.Remove(t.Context(), r); fail != port.CredentialRefused {
					t.Fatal("durable cleanup accepted a substituted unpublished entry", fail)
				}
			}
		})
	}
}
func TestCredentialCleanupRefusesSubstitution(t *testing.T) {
	for _, fault := range []string{"extra entry", "directory mode", "file mode", "file symlink", "file inode", "owner", "directory inode"} {
		t.Run(fault, func(t *testing.T) {
			fixture := diskCredentialFixture(t, map[string][]byte{"claude-token": []byte("synthetic\n")})
			r, fail := stageCredentialFixture(t, fixture)
			if fail != "" {
				t.Fatal(fail)
			}
			s := fixture.Stager.(Credentials)
			dir := filepath.Join(s.root, filepath.Base(r.Credentials.Directory))
			file := filepath.Join(dir, "claude-token")
			switch fault {
			case "extra entry":
				if err := os.WriteFile(filepath.Join(dir, "unexpected"), []byte("retain"), 0400); err != nil {
					t.Fatal(err)
				}
			case "directory mode":
				if err := os.Chmod(dir, 0755); err != nil {
					t.Fatal(err)
				}
			case "file mode":
				if err := os.Chmod(file, 0600); err != nil {
					t.Fatal(err)
				}
			case "file symlink":
				if err := os.Remove(file); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink("/missing", file); err != nil {
					t.Fatal(err)
				}
			case "file inode":
				if err := os.Rename(file, file+".original"); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(file, []byte("replacement"), 0400); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(file + ".original"); err != nil {
					t.Fatal(err)
				}
			case "owner":
				r.UID = "999999"
			case "directory inode":
				r.Credentials.Inode = "999999999"
			}
			if fail := s.Remove(t.Context(), r); fail == "" {
				t.Fatal("substituted staging was removed")
			}
			if _, err := os.Lstat(dir); err != nil {
				t.Fatal("unsafe cleanup lost original evidence")
			}
		})
	}
}
func TestCredentialCleanupAttemptsEveryFileAndRetries(t *testing.T) {
	fixture := diskCredentialFixture(t, map[string][]byte{"claude-token": []byte("a"), "github-token": []byte("b")})
	r, fail := stageCredentialFixture(t, fixture)
	if fail != "" {
		t.Fatal(fail)
	}
	s := fixture.Stager.(Credentials)
	attempts := 0
	s.unlink = func(root *os.Root, name string) error {
		attempts++
		if name == "claude-token" {
			return errors.New("synthetic unlink failure")
		}
		return root.Remove(name)
	}
	if fail = s.Remove(t.Context(), r); fail == "" || attempts != 2 {
		t.Fatal("cleanup skipped a file or ignored a failed unlink")
	}
	if presence, _ := s.Observe(t.Context(), r); presence != "present" {
		t.Fatal("partial cleanup lost directory authority")
	}
	s.unlink = nil
	if fail = s.Remove(t.Context(), r); fail != "" {
		t.Fatal("cleanup retry failed", fail)
	}
}
func TestCredentialSourcesAndIndependentCommands(t *testing.T) {
	value := []byte("synthetic-final-newline\n")
	fixture := diskCredentialFixture(t, map[string][]byte{"claude-token": value})
	first, fail := stageCredentialFixture(t, fixture)
	if fail != "" {
		t.Fatal(fail)
	}
	second := fixture.Intent()
	second.Credentials.Directory = "/dev/shm/itos-container-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	fail = fixture.Stager.Stage(t.Context(), second, fixture.Sources, func(s port.CredentialStaging) port.Outcome { second.Credentials = s; return "" })
	if fail != "" {
		t.Fatal(fail)
	}
	if fail = fixture.Stager.Remove(t.Context(), first); fail != "" {
		t.Fatal(fail)
	}
	if !bytes.Equal(fixture.Read(second.Credentials.Mounts()[0].Source), value) {
		t.Fatal("cleanup changed another command's copy")
	}
	original, err := os.ReadFile(fixture.Sources.Checked()[0].ReadPath)
	if err != nil || !bytes.Equal(original, value) {
		t.Fatal("staging changed the original source")
	}
	if fail = fixture.Stager.Remove(t.Context(), second); fail != "" {
		t.Fatal(fail)
	}
}
func TestCredentialNoGrantAllocatesNothing(t *testing.T) {
	s := &f.CredentialStager{}
	if fail := s.Stage(context.Background(), port.Record{}, nil, func(port.CredentialStaging) port.Outcome { t.Fatal("no-grant publication"); return "" }); fail != "" || len(s.Events) != 0 {
		t.Fatal("no-grant allocated staging")
	}
	disk := Credentials{OS: "linux", root: t.TempDir(), tmpfs: func(string) bool { t.Fatal("no-grant tmpfs observation"); return false }}
	if fail := disk.Stage(t.Context(), port.Record{}, nil, nil); fail != "" {
		t.Fatal(fail)
	}
}

func TestCredentialStagingOutputAbsence(t *testing.T) {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		t.Fatal("marker setup failed")
	}
	marker := []byte("itos-synthetic-credential-" + hex.EncodeToString(random[:]))
	fixture := diskCredentialFixture(t, map[string][]byte{"claude-token": marker})
	r, fail := stageCredentialFixture(t, fixture)
	if fail != "" {
		t.Fatal(fail)
	}
	source, err := os.ReadFile(fixture.Sources.Checked()[0].ReadPath)
	if err != nil || !f.CredentialMarkerPresent(source, marker) {
		t.Fatal("source detector positive control failed")
	}
	if !f.CredentialMarkerPresent(fixture.Read(r.Credentials.Mounts()[0].Source), marker) {
		t.Fatal("staged copy detector positive control failed")
	}
	if fail = fixture.Stager.Remove(t.Context(), r); fail != "" {
		t.Fatal(fail)
	}
	root := fixture.Stager.(Credentials).root
	if err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() {
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			if f.CredentialMarkerPresent(data, marker) {
				t.Error("credential marker remained in staging fixture")
			}
		}
		return nil
	}); err != nil {
		t.Fatal("staging output search failed")
	}
}

func TestCredentialRecoveryInterruptedPublication(t *testing.T) {
	for _, disk := range []bool{false, true} {
		t.Run(map[bool]string{false: "memory", true: "disk"}[disk], func(t *testing.T) {
			b, memory, _ := f.Fixture()
			b.Credentials = port.CredentialKinds{Claude: true}
			memory.Codec.(*f.Codec).Value = b
			before := f.RunIntent(b)
			before.Stage = "reserved"
			var store port.Store
			var fault func(bool)
			if disk {
				native, dir := egressStore(b, before)
				store = native
				fault = func(enabled bool) {
					if enabled {
						dir.WriteFailure = "state.write"
					} else {
						dir.WriteFailure = ""
					}
				}
			} else {
				memory.Records = map[string][]byte{port.Reference(before.Project, before.NameToken()): memory.Codec.Encode(before)}
				store = memory
				fault = func(enabled bool) {
					if enabled {
						memory.Fault = "reserved-record write"
					} else {
						memory.Fault = ""
					}
				}
			}
			ref := port.Reference(before.Project, before.NameToken())
			i, fail := store.OpenCleanup(t.Context(), ref)
			if fail != "" {
				t.Fatal(fail)
			}
			next := before
			next.Schema = 4
			next.Credentials = f.CredentialIntent(b.Credentials).Credentials
			fault(true)
			fail = i.Save(next)
			i.Close()
			if fail == "" {
				t.Fatal("interrupted staging intent publication reported durability")
			}
			fault(false)
			i, fail = store.OpenCleanup(t.Context(), ref)
			if fail != "" {
				t.Fatal(fail)
			}
			defer i.Close()
			if i.Record() != before || i.Validate() != "" {
				t.Fatal("interrupted publication changed original recovery authority")
			}
			if fail = i.Save(next); fail != "" {
				t.Fatal("fresh publication could not advance original authority", fail)
			}
		})
	}
}
