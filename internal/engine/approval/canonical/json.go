// Package canonical implements the specified encoding/json SHA-256 wire format.
package canonical

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"strconv"
	"strings"

	"github.com/donvargax/itos-container/internal/engine/approval/port"
	policydomain "github.com/donvargax/itos-container/internal/engine/policy/domain"
	readiness "github.com/donvargax/itos-container/internal/engine/readiness/port"
)

// JSON is stateless; encoding never reads checkout files or host environment.
type JSON struct{}

type host struct {
	UID string `json:"uid"`
	GID string `json:"gid"`
}
type checkout struct {
	Root      string `json:"root"`
	Device    string `json:"device"`
	Inode     string `json:"inode"`
	GitDevice string `json:"git_device"`
	GitInode  string `json:"git_inode"`
}
type engine struct {
	Endpoint     string `json:"endpoint"`
	ID           string `json:"id"`
	OS           string `json:"os"`
	Architecture string `json:"architecture"`
}
type network struct {
	Mode  string   `json:"mode"`
	Allow []string `json:"allow,omitempty"`
}
type proxy struct {
	Image string `json:"image"`
}

// ProxyImage binds the inert proxy root filesystem selected in T-38.
const ProxyImage = "docker.io/library/busybox@sha256:5624bb6777cf469b1fb50ae3f981b15d2715ee2fffaf20b9998cfb7843726835"

type policy struct {
	Version  int     `json:"version"`
	Manifest string  `json:"manifest"`
	Network  network `json:"network"`
}
type manifest struct {
	Name          *string           `json:"name"`
	Image         string            `json:"image"`
	Workspace     string            `json:"workspaceFolder"`
	Env           map[string]string `json:"containerEnv"`
	ContainerUser string            `json:"containerUser"`
	RemoteUser    string            `json:"remoteUser"`
}
type image struct {
	Reference    string `json:"reference"`
	ID           string `json:"id"`
	OS           string `json:"os"`
	Architecture string `json:"architecture"`
}
type settings struct {
	Schema   int      `json:"schema"`
	Profile  string   `json:"profile"`
	Proxy    *proxy   `json:"proxy,omitempty"`
	Host     host     `json:"host"`
	Checkout checkout `json:"checkout"`
	Engine   engine   `json:"engine"`
	Policy   policy   `json:"policy"`
	Manifest manifest `json:"manifest"`
	Image    image    `json:"image"`
}
type envelope struct {
	Schema      int      `json:"schema"`
	Project     string   `json:"project"`
	Generation  string   `json:"generation"`
	Fingerprint string   `json:"fingerprint"`
	Settings    settings `json:"settings"`
}

func canonical(v any) []byte {
	raw, _ := json.Marshal(v)
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var obj any
	_ = decoder.Decode(&obj)
	result, _ := json.Marshal(obj)
	return result
}
func digest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func project(s settings) string {
	return digest(canonical(map[string]any{"schema": 1, "uid": s.Host.UID, "checkout": s.Checkout}))
}

// Project uses the same identity binding without loading mutable requests.
func (JSON) Project(c readiness.CheckoutIdentity, uid string) string {
	return project(settings{Host: host{UID: uid}, Checkout: checkout{c.Root, c.Device, c.Inode, c.GitDevice, c.GitInode}})
}
func candidate(s settings) port.Candidate {
	data := canonical(s)
	return port.Candidate{UID: s.Host.UID, GID: s.Host.GID, Root: s.Checkout.Root, Project: project(s), Fingerprint: "sha256:" + digest(data), Canonical: string(data)}
}

// Candidate fills every canonical member, including all effective defaults.
func (JSON) Candidate(r readiness.Result, uid, gid string) port.Candidate {
	c, e, i, m := r.Checkout, r.Engine, r.Image, r.Manifest.Effective
	env := m.ContainerEnv
	if env == nil {
		env = map[string]string{}
	}
	s := settings{Schema: 1, Profile: "offline-v1", Host: host{uid, gid}, Checkout: checkout{c.Root, c.Device, c.Inode, c.GitDevice, c.GitInode}, Engine: engine{e.Endpoint, e.ID, e.OS, e.Architecture}, Policy: policy{r.Policy.Policy.Version, r.Policy.Policy.Manifest, network{Mode: r.Policy.Policy.Network.Mode}}, Manifest: manifest{m.Name, m.Image, m.WorkspaceFolder, env, uid, uid}, Image: image{i.Reference, i.ID, i.OS, i.Architecture}}
	if s.Policy.Network.Mode == "allowlist" {
		s.Profile = "allowlist-v1"
		s.Policy.Network.Allow = r.Policy.Policy.Network.Allow
		s.Proxy = &proxy{Image: ProxyImage}
	}
	return candidate(s)
}

// Encode emits compact recursively sorted maps, default HTML escaping, no LF.
func (JSON) Encode(record port.Record) []byte {
	var s settings
	if json.Unmarshal([]byte(record.Canonical), &s) != nil {
		return nil
	}
	return canonical(envelope{1, record.Project, record.Generation, record.Fingerprint, s})
}

// Decode bounds depth before typed decoding; exact re-encoding rejects duplicates,
// equivalent noncanonical escapes, whitespace, omissions and unknown members.
func (JSON) Decode(current port.Candidate, data []byte) (port.Record, port.Failure) {
	invalid := func() (port.Record, port.Failure) { return port.Record{}, "state.invalid" }
	if len(data) == 0 || len(data) > 1024*1024 || !bounded(data) {
		return invalid()
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var record envelope
	if decoder.Decode(&record) != nil {
		return invalid()
	}
	if !bytes.Equal(data, canonical(record)) {
		return invalid()
	}
	n, err := strconv.ParseUint(record.Generation, 10, 64)
	if err != nil || n == 0 || strconv.FormatUint(n, 10) != record.Generation || record.Schema != 1 {
		return invalid()
	}
	s := record.Settings
	var expected settings
	if json.Unmarshal([]byte(current.Canonical), &expected) != nil {
		return invalid()
	}
	if !valid(s) || s.Host.UID != current.UID || s.Checkout != expected.Checkout {
		return invalid()
	}
	c := candidate(s)
	if record.Project != c.Project || record.Project != current.Project || record.Fingerprint != c.Fingerprint {
		return invalid()
	}
	return port.Record{Candidate: c, Generation: record.Generation}, ""
}
func bounded(data []byte) bool {
	d := json.NewDecoder(bytes.NewReader(data))
	depth := 0
	for {
		token, err := d.Token()
		if err == io.EOF {
			return depth == 0
		}
		if err != nil {
			return false
		}
		if delimiter, ok := token.(json.Delim); ok {
			switch delimiter {
			case '{', '[':
				depth++
				if depth > 32 {
					return false
				}
			case '}', ']':
				depth--
			}
		}
	}
}
func decimal(s string) bool {
	n, err := strconv.ParseUint(s, 10, 64)
	return err == nil && strconv.FormatUint(n, 10) == s
}
func sha(s string) bool {
	return strings.HasPrefix(s, "sha256:") && len(s) == 71 && strings.Trim(s[7:], "0123456789abcdef") == ""
}
func valid(s settings) bool {
	return s.Schema == 1 && validProfile(s) && decimal(s.Host.UID) && s.Host.UID != "0" && decimal(s.Host.GID) &&
		s.Checkout.Root != "" && decimal(s.Checkout.Device) && decimal(s.Checkout.Inode) && decimal(s.Checkout.GitDevice) && decimal(s.Checkout.GitInode) &&
		s.Engine.Endpoint != "" && s.Engine.ID != "" && s.Engine.OS == "linux" && s.Engine.Architecture == "amd64" &&
		s.Policy.Manifest != "" &&
		s.Manifest.ContainerUser == s.Host.UID && s.Manifest.RemoteUser == s.Host.UID && s.Manifest.Env != nil && s.Manifest.Workspace != "" &&
		s.Manifest.Image == s.Image.Reference && sha(s.Image.ID) && s.Image.OS == "linux" && s.Image.Architecture == "amd64"
}

func validProfile(s settings) bool {
	n := s.Policy.Network
	switch s.Profile {
	case "offline-v1":
		return (s.Policy.Version == 1 || s.Policy.Version == 2) && n.Mode == "offline" && n.Allow == nil && s.Proxy == nil
	case "allowlist-v1":
		if s.Policy.Version != 2 || n.Mode != "allowlist" || len(n.Allow) == 0 || s.Proxy == nil {
			return false
		}
		name, digest, ok := strings.Cut(s.Proxy.Image, "@")
		if !ok || name == "" || !sha(digest) {
			return false
		}
		for i, name := range n.Allow {
			if !policydomain.ValidCanonicalHost(name) || (i > 0 && n.Allow[i-1] >= name) {
				return false
			}
		}
		return true
	default:
		return false
	}
}
