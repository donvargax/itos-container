package domain

import (
	"reflect"
	"strings"
	"testing"
	"time"

	manifest "github.com/donvargax/itos-container/internal/engine/manifest/port"
	manifestfake "github.com/donvargax/itos-container/internal/engine/manifest/port/porttest"
	policy "github.com/donvargax/itos-container/internal/engine/policy/port"
	policyfake "github.com/donvargax/itos-container/internal/engine/policy/port/porttest"
	"github.com/donvargax/itos-container/internal/engine/readiness/port"
	"github.com/donvargax/itos-container/internal/engine/readiness/port/porttest"
)

func fixture() (Inspector, port.Input, *porttest.Engine, *porttest.Checkout) {
	s := porttest.HealthySession()
	image := s.Images[porttest.Reference]
	image.Label = manifest.Label{}
	s.Images[porttest.Reference] = image
	e := &porttest.Engine{Session: s}
	c := &porttest.Checkout{Identity: port.CheckoutIdentity{Root: "/checkout", Device: "1", Inode: "2", GitDevice: "1", GitInode: "3"}}
	i := Inspector{
		Checkout: c, Engine: e,
		Policy: &policyfake.Source{Value: policy.Value{Kind: policy.Object, Fields: map[string]policy.Value{
			"version": {Kind: policy.Number, Number: policy.Decimal{Coefficient: "1", Exponent: "0"}}, "manifest": {Kind: policy.String, Text: "worker.json"},
		}}},
		Files: &manifestfake.Files{Data: map[string][]byte{"worker.json": []byte("manifest data")}},
		Parser: &manifestfake.Parser{Value: manifest.Value{Kind: manifest.Object, Fields: map[string]manifest.Value{
			"image": {Kind: manifest.String, Text: porttest.Reference}, "containerUser": {Kind: manifest.String, Text: "1000"}, "remoteUser": {Kind: manifest.String, Text: "1000"},
		}}},
		References: &manifestfake.References{Images: map[string]bool{porttest.Reference: true}},
		Digests:    porttest.Digests{porttest.Reference: porttest.Reference},
	}
	return i, port.Input{OS: "linux", Architecture: "amd64", Root: "/checkout", Host: s.Address}, e, c
}

func TestInspectSnapshot(t *testing.T) {
	i, input, e, c := fixture()
	result := i.Inspect(input)
	if !result.Ready || len(result.Issues) != 0 || *result.Checkout != c.Identity || result.Engine.ID != "engine-A" || result.Engine.Architecture != "amd64" || result.Image.Reference != porttest.Reference || !result.Manifest.Metadata.Checked || *result.Manifest.Metadata.Present {
		t.Fatalf("snapshot = %#v", result)
	}
	if c.Opened || !e.Session.Closed || e.Budget != 5*time.Second || e.Selection != input.Host || e.Root != input.Root || e.Session.InfoReads != 2 || !reflect.DeepEqual(e.Session.ImageReads, []string{porttest.Reference}) {
		t.Fatalf("capability lifecycle = %#v, %#v, %#v", c, e, e.Session)
	}
}

func TestRepositoryDigestIdentity(t *testing.T) {
	for _, tc := range []struct {
		name       string
		requested  string
		candidates []string
		ready      bool
	}{
		{"normalized", "canonical", []string{"other", "normalized"}, true},
		{"different repository", "canonical", []string{"other"}, false},
		{"invalid observation", "canonical", []string{"invalid"}, false},
		{"invalid request", "", []string{"invalid"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			i, input, engine, _ := fixture()
			i.Digests = porttest.Digests{porttest.Reference: tc.requested, "normalized": "canonical", "other": "different"}
			image := engine.Session.Images[porttest.Reference]
			image.RepoDigests = tc.candidates
			engine.Session.Images[porttest.Reference] = image
			result := i.Inspect(input)
			if result.Ready != tc.ready || result.Image.Reference != porttest.Reference || !reflect.DeepEqual(engine.Session.ImageReads, []string{porttest.Reference}) {
				t.Fatalf("digest identity or original reference lost: %#v", result)
			}
			if !tc.ready && !reflect.DeepEqual(result.Issues, []port.Issue{{Kind: port.ImageDigest, Subject: porttest.Reference}}) {
				t.Fatalf("digest refusal = %#v", result.Issues)
			}
		})
	}
}

func TestPrerequisitesStopBeforeEngine(t *testing.T) {
	for _, name := range []string{"darwin", "windows", "arm64", "root", "policy", "manifest", "context", "tls", "cert", "api"} {
		t.Run(name, func(t *testing.T) {
			i, input, e, c := fixture()
			want := port.EngineSelection
			switch name {
			case "darwin", "windows":
				input.OS = name
				want = port.HostUnsupported
			case "arm64":
				input.Architecture = name
				want = port.HostUnsupported
			case "root":
				c.Failure = port.CheckoutPath
				want = port.CheckoutPath
			case "policy":
				i.Policy.(*policyfake.Source).Issues = []policy.Issue{{Kind: policy.Missing}}
				want = port.None
			case "manifest":
				i.Files.(*manifestfake.Files).Data = nil
				want = port.None
			case "context":
				input.Context = "default"
			case "tls":
				input.TLSVerify = "1"
			case "cert":
				input.CertPath = "/unused"
			case "api":
				input.APIVersion = "1.44"
			}
			result := i.Inspect(input)
			if result.Ready || e.Opens != 0 || c.Opened {
				t.Fatalf("unsafe prerequisite: %#v, %#v, %#v", result, e, c)
			}
			if want != port.None && (len(result.Issues) != 1 || result.Issues[0].Kind != want) {
				t.Fatalf("issues = %#v, want %v", result.Issues, want)
			}
			if want == port.HostUnsupported && result.Checkout != nil {
				t.Fatal("unsupported host acquired checkout identity")
			}
		})
	}
}

func TestEngineAndImageFailures(t *testing.T) {
	for _, tc := range []struct {
		name   string
		want   port.Failure
		change func(*porttest.Engine, *porttest.Checkout)
	}{
		{"open", port.EngineUnavailable, func(e *porttest.Engine, _ *porttest.Checkout) { e.Failure = port.EngineUnavailable }},
		{"version", port.EngineInspect, func(e *porttest.Engine, _ *porttest.Checkout) { e.Session.VersionFailure = port.EngineInspect }},
		{"api", port.EngineAPI, func(e *porttest.Engine, _ *porttest.Checkout) { e.Session.VersionData.Minimum = "1.57" }},
		{"info", port.EngineTimeout, func(e *porttest.Engine, _ *porttest.Checkout) { e.Session.InfoFailure = port.EngineTimeout }},
		{"identity", port.EngineIdentityMissing, func(e *porttest.Engine, _ *porttest.Checkout) { e.Session.InfoData.ID = " " }},
		{"product", port.EngineUnsupported, func(e *porttest.Engine, _ *porttest.Checkout) { e.Session.VersionData.Product = "Podman" }},
		{"desktop", port.EngineUnsupported, func(e *porttest.Engine, _ *porttest.Checkout) { e.Session.InfoData.OperatingSystem = "Docker Desktop" }},
		{"platform", port.EnginePlatform, func(e *porttest.Engine, _ *porttest.Checkout) { e.Session.VersionData.OS = "windows" }},
		{"image", port.ImageMissing, func(e *porttest.Engine, _ *porttest.Checkout) { e.Session.Images = nil }},
		{"socket", port.EngineChanged, func(e *porttest.Engine, _ *porttest.Checkout) { e.Session.Changed = true }},
		{"daemon", port.EngineChanged, func(e *porttest.Engine, _ *porttest.Checkout) { e.Session.LaterInfo = &port.Info{ID: "engine-B"} }},
		{"recheck", port.EngineInspect, func(e *porttest.Engine, _ *porttest.Checkout) { e.Session.LaterInfoFailure = port.EngineInspect }},
		{"checkout", port.CheckoutChanged, func(_ *porttest.Engine, c *porttest.Checkout) { c.Changed = true }},
		{"layout", port.CheckoutLayout, func(_ *porttest.Engine, c *porttest.Checkout) { c.InspectFailure = port.CheckoutLayout }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			i, input, e, c := fixture()
			tc.change(e, c)
			result := i.Inspect(input)
			if result.Ready || len(result.Issues) != 1 || result.Issues[0].Kind != tc.want {
				t.Fatalf("result = %#v, want %v", result, tc.want)
			}
			if c.Opened || (e.Opens != 0 && e.Failure == port.None && !e.Session.Closed) {
				t.Fatal("leaked inspection pin/session")
			}
		})
	}
}

func TestIndependentImageDefectsAndUnusableImage(t *testing.T) {
	for _, kind := range []string{"independent", "missing id", "invalid id", "missing config", "metadata"} {
		t.Run(kind, func(t *testing.T) {
			i, input, e, _ := fixture()
			image := e.Session.Images[porttest.Reference]
			switch kind {
			case "independent":
				image.RepoDigests = nil
				image.Architecture = "arm64"
				image.User = "root"
			case "missing id":
				image.ID = ""
			case "invalid id":
				image.ID = "sha256:" + strings.Repeat("z", 64)
			case "missing config":
				image.ConfigPresent = false
			case "metadata":
				image.Label = manifest.Label{Present: true, Text: "not-an-array"}
			}
			e.Session.Images[porttest.Reference] = image
			result := i.Inspect(input)
			if result.Ready {
				t.Fatal("unusable image accepted")
			}
			switch kind {
			case "independent":
				want := []port.Issue{{Kind: port.ImageDigest, Subject: porttest.Reference}, {Kind: port.ImagePlatform, Subject: porttest.Reference}}
				if !reflect.DeepEqual(result.Issues, want) {
					t.Fatalf("issues = %#v", result.Issues)
				}
			case "metadata":
				if result.Manifest.Effective != nil || !result.Manifest.Metadata.Checked || result.Manifest.Issues[0].Kind != manifest.MetadataFailure {
					t.Fatal(result.Manifest)
				}
			default:
				if result.Issues[0].Kind != port.ImageInspect || result.Manifest.Metadata.Checked || result.Image != nil {
					t.Fatal(result)
				}
			}
		})
	}
}

func TestImageUserDoesNotSelectWorkerIdentity(t *testing.T) {
	for _, user := range []string{"", "root", "0:0", "2000:3000", "worker"} {
		inspector, input, engine, _ := fixture()
		image := engine.Session.Images[porttest.Reference]
		image.User = user
		engine.Session.Images[porttest.Reference] = image
		if result := inspector.Inspect(input); !result.Ready {
			t.Fatalf("image user %q affected readiness: %#v", user, result)
		}
	}
}

func TestAPIRangeIncludesNegotiatedVersion(t *testing.T) {
	valid := porttest.HealthySession().VersionData
	if !compatibleAPI(valid) {
		t.Fatal(valid)
	}
	for _, tc := range []struct{ field, value string }{
		{"minimum", "1.57"}, {"minimum", ""}, {"minimum", "9.98"}, {"maximum", "1.39"},
		{"used", "1.39"}, {"used", "1.57"}, {"used", "1.-1"}, {"used", "1.99999999999999999999999"},
		{"sdkmin", "1.57"}, {"sdkmax", "1.39"},
	} {
		v := valid
		switch tc.field {
		case "minimum":
			v.Minimum = tc.value
		case "maximum":
			v.Maximum = tc.value
		case "used":
			v.Negotiated = tc.value
		case "sdkmin":
			v.SDKMinimum = tc.value
		case "sdkmax":
			v.SDKMaximum = tc.value
		}
		if compatibleAPI(v) {
			t.Fatalf("accepted %#v", v)
		}
	}
}

func TestRecoveryEngineValidationRequiresCompatibleAPI(t *testing.T) {
	healthy := porttest.HealthySession()
	if failure := ValidateEngine(healthy.VersionData, healthy.InfoData); failure != port.None {
		t.Fatal("healthy recovery engine refused", failure)
	}
	tooNew, tooOld := healthy.VersionData, healthy.VersionData
	tooNew.Minimum = "1.57"
	tooOld.Negotiated = "1.39"
	for _, version := range []port.Version{{}, tooNew, tooOld} {
		if failure := ValidateEngine(version, healthy.InfoData); failure != port.EngineAPI {
			t.Fatalf("recovery API validation = %v, want %v", failure, port.EngineAPI)
		}
	}
	foreign := healthy.InfoData
	foreign.OperatingSystem = "Docker Desktop"
	if failure := ValidateEngine(healthy.VersionData, foreign); failure != port.EngineUnsupported {
		t.Fatal("compatible API bypassed native engine validation", failure)
	}
}

func TestLocalCheckpointRetainsPinAndCanRefuseEngine(t *testing.T) {
	for _, accept := range []bool{false, true} {
		inspector, input, engine, checkout := fixture()
		calls := 0
		result := inspector.InspectBeforeEngine(input, func(local port.Result) bool {
			calls++
			if !checkout.Opened || engine.Selection != "" || local.Checkout == nil || local.Engine != nil || local.Image != nil || local.Ready {
				t.Fatal("checkpoint was not local and pinned")
			}
			return accept
		})
		if calls != 1 || result.Ready != accept || checkout.Opened {
			t.Fatalf("checkpoint lifecycle: %d %#v", calls, result)
		}
		if !accept && (engine.Selection != "" || engine.Session.InfoReads != 0) {
			t.Fatal("refused checkpoint contacted engine")
		}
	}
}
