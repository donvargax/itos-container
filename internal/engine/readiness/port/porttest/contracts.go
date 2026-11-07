package porttest

import (
	"reflect"
	"strings"
	"time"

	manifest "github.com/donvargax/itos-container/internal/engine/manifest/port"
	"github.com/donvargax/itos-container/internal/engine/readiness/port"
)

// Reporter is the shared testing boundary, without production I/O.
type Reporter interface {
	Helper()
	Fatalf(string, ...any)
	Errorf(string, ...any)
}

// Reference is explicit fixture data, never an image acquisition suggestion.
var Reference = "worker.test/agent@sha256:" + strings.Repeat("a", 64)

// HealthySession supplies a native-engine observation, not an approval.
func HealthySession() *Session {
	return &Session{
		Address:     "unix:///fixture.sock",
		VersionData: port.Version{Product: "Docker Engine - Community", OS: "linux", Architecture: "amd64", Minimum: "1.44", Maximum: "1.56", Negotiated: "1.56", SDKMinimum: "1.40", SDKMaximum: "1.56"},
		InfoData:    port.Info{ID: "engine-A", OS: "linux", Architecture: "x86_64", OperatingSystem: "Debian"},
		Images:      map[string]port.Image{Reference: {ID: "sha256:" + strings.Repeat("b", 64), OS: "linux", Architecture: "amd64", User: "1000:1000", RepoDigests: []string{Reference}, ConfigPresent: true, Label: manifest.Label{Present: true, Text: `[{"name":"display"}]`}}},
	}
}

// EngineContract exercises real SDK translation and the memory fake unchanged.
// Factories configure only fixture state; expectations live here, not in them.
func EngineContract(t Reporter, create func(*Session) (port.Engine, string, string)) {
	t.Helper()
	for _, operation := range []string{"healthy", "version failure", "info failure", "image failure", "missing config", "absent label"} {
		data := HealthySession()
		switch operation {
		case "version failure":
			data.VersionFailure = port.EngineInspect
		case "info failure":
			data.InfoFailure = port.EngineInspect
		case "image failure":
			data.ImageFailure = port.ImageInspect
		case "missing config":
			i := data.Images[Reference]
			i.ConfigPresent = false
			i.User = ""
			i.Label = manifest.Label{}
			data.Images[Reference] = i
		case "absent label":
			i := data.Images[Reference]
			i.Label = manifest.Label{}
			data.Images[Reference] = i
		}
		engine, endpoint, root := create(data)
		session, failure := engine.Open(endpoint, root, time.Second)
		if failure != port.None || session == nil {
			t.Fatalf("%s open = %v", operation, failure)
		}
		if session.Endpoint() != endpoint || !session.Unchanged() {
			t.Errorf("%s lost endpoint or socket identity", operation)
		}
		version, failure := session.Version()
		if failure != data.VersionFailure {
			t.Fatalf("%s version failure = %v", operation, failure)
		}
		if failure != port.None {
			session.Close()
			continue
		}
		if version != data.VersionData {
			t.Errorf("version = %#v, want %#v", version, data.VersionData)
		}
		info, failure := session.Info()
		if failure != data.InfoFailure {
			t.Fatalf("%s info failure = %v", operation, failure)
		}
		if failure != port.None {
			session.Close()
			continue
		}
		if info != data.InfoData {
			t.Errorf("info = %#v, want %#v", info, data.InfoData)
		}
		image, failure := session.Image(Reference)
		if failure != data.ImageFailure {
			t.Fatalf("%s image failure = %v", operation, failure)
		}
		if failure != port.None {
			session.Close()
			continue
		}
		if !reflect.DeepEqual(image, data.Images[Reference]) {
			t.Errorf("%s image = %#v, want %#v", operation, image, data.Images[Reference])
		}
		image.RepoDigests[0] = "caller mutation"
		again, failure := session.Image(Reference)
		if failure != port.None || again.RepoDigests[0] != Reference {
			t.Errorf("image storage aliases earlier result")
		}
		if _, failure = session.Image(Reference + "missing"); failure != port.ImageMissing {
			t.Errorf("missing image = %v", failure)
		}
		session.Close()
	}
}

// CheckoutContract checks stable identities, observed replacement and release.
func CheckoutContract(t Reporter, create func() (port.Checkout, port.CheckoutIdentity, func())) {
	t.Helper()
	c, want, replace := create()
	if c.Unchanged() {
		t.Errorf("unopened checkout claims a stable pin")
	}
	if failure := c.Pin(want.Root); failure != port.None {
		t.Fatalf("pin = %v", failure)
	}
	got, failure := c.Inspect()
	if failure != port.None || got != want || !c.Unchanged() {
		t.Fatalf("checkout = %#v, %v; want %#v", got, failure, want)
	}
	replace()
	if c.Unchanged() {
		t.Errorf("replacement retained old identity")
	}
	c.Close()
	c.Close()
	if c.Unchanged() {
		t.Errorf("closed checkout claims a stable pin")
	}
}

// CheckoutIndependenceContract requires fresh admission and revalidation, with
// the same assertions on real filesystem layouts and memory-only observations.
func CheckoutIndependenceContract(t Reporter, create func() (port.Checkout, string, func(string))) {
	t.Helper()
	for _, change := range []string{"shared", "unavailable"} {
		c, root, edit := create()
		if failure := c.Pin(root); failure != port.None {
			t.Fatalf("pin = %v", failure)
		}
		original, failure := c.Inspect()
		if failure != port.None || !c.Unchanged() {
			t.Fatalf("independent checkout refused: %v", failure)
		}
		edit(change)
		if c.Unchanged() {
			t.Errorf("%s layout retained admission", change)
		}
		want := port.CheckoutHardlink
		if change == "unavailable" {
			want = port.CheckoutUnavailable
		}
		if got, failure := c.Inspect(); failure != want || got != (port.CheckoutIdentity{}) {
			t.Errorf("%s inspection = %#v, %v", change, got, failure)
		}
		edit("independent")
		if got, failure := c.Inspect(); failure != port.None || got != original || !c.Unchanged() {
			t.Errorf("independent replacement changed identity or retained refusal: %#v, %v", got, failure)
		}
		c.Close()
	}
}
