package domain

import (
	"reflect"
	"strings"
	"testing"

	policy "github.com/donvargax/itos-container/internal/engine/policy/port/porttest"
	"github.com/donvargax/itos-container/internal/engine/readiness/port"
	"github.com/donvargax/itos-container/internal/engine/readiness/port/porttest"
)

const proxyReference = "proxy.test/egress@sha256:" + "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func egressFixture() (Inspector, port.Input, *porttest.Engine, *porttest.Checkout) {
	i, input, e, c := fixture()
	i.Policy = &policy.Source{Value: version2(allow("api.example.com"))}
	i.Names = &policy.Names{Table: map[string]string{"api.example.com": "api.example.com"}}
	i.Executables = &porttest.Executables{Observed: port.Executable{Path: "/installed/itos-container", Regular: true, OS: "linux", Architecture: "amd64"}}
	i.ProxyImage = proxyReference
	i.Digests.(porttest.Digests)[proxyReference] = proxyReference
	e.Session.VersionData.Release = "28.0.0"
	e.Session.VersionData.Negotiated = "1.48"
	e.Session.Images[proxyReference] = port.Image{ID: "sha256:" + strings.Repeat("d", 64), OS: "linux", Architecture: "amd64", ConfigPresent: true, RepoDigests: []string{proxyReference}}
	return i, input, e, c
}

func TestEgressReadinessObservesOriginalProxyConfigID(t *testing.T) {
	i, input, e, _ := egressFixture()
	r := i.Inspect(input)
	if !r.Ready || r.ProxyImage == nil || r.ProxyImage.ID != e.Session.Images[proxyReference].ID || r.ProxyImage.Reference != proxyReference || i.Executables.(*porttest.Executables).Calls != 1 || !reflect.DeepEqual(e.Session.ImageReads, []string{porttest.Reference, proxyReference}) {
		t.Fatalf("egress observations: %#v", r)
	}
}

func TestEgressPrerequisiteFailures(t *testing.T) {
	for _, defect := range []string{"release", "api", "missing", "config", "id", "digest", "platform", "volumes", "executable", "no observer"} {
		t.Run(defect, func(t *testing.T) {
			i, input, e, _ := egressFixture()
			image := e.Session.Images[proxyReference]
			want := port.EgressImage
			switch defect {
			case "release":
				e.Session.VersionData.Release = "unknown"
				want = port.EgressEngine
			case "api":
				e.Session.VersionData.Negotiated = "1.47"
				want = port.EgressEngine
			case "missing":
				delete(e.Session.Images, proxyReference)
				want = port.EgressImageMissing
			case "config":
				image.ConfigPresent = false
			case "id":
				image.ID = "sha256:bad"
			case "digest":
				image.RepoDigests = nil
			case "platform":
				image.Architecture = "arm64"
			case "volumes":
				image.Volumes = true
			case "executable":
				i.Executables.(*porttest.Executables).Failure = port.EgressExecutable
				want = port.EgressExecutable
			case "no observer":
				i.Executables = nil
				want = port.EgressExecutable
			}
			if defect != "missing" {
				e.Session.Images[proxyReference] = image
			}
			r := i.Inspect(input)
			if r.Ready || r.ProxyImage != nil || len(r.Issues) != 1 || r.Issues[0].Kind != want {
				t.Fatalf("%s: %#v", defect, r)
			}
		})
	}
}

func TestEgressVersionMinimum(t *testing.T) {
	for _, release := range []string{"28.0.0", "28.0.1", "29.1.2"} {
		if !egressVersion(port.Version{Release: release, Negotiated: "1.48"}) {
			t.Fatal(release)
		}
	}
	for _, release := range []string{"", "unknown", "27.9.9", "28", "28.0", "28.0.0.1", "28.-1.0", "28.0.x", "999999999999999999999999.0.0"} {
		if egressVersion(port.Version{Release: release, Negotiated: "1.48"}) {
			t.Fatal(release)
		}
	}
	for _, api := range []string{"", "1.47", "bad"} {
		if egressVersion(port.Version{Release: "28.0.0", Negotiated: api}) {
			t.Fatal(api)
		}
	}
}

func TestUnusableProxyInspectionRetainsSafeTypedFailure(t *testing.T) {
	i, _, engine, checkout := egressFixture()
	engine.Session.ImageFailure = port.EngineTimeout
	r := port.Result{Checkout: &checkout.Identity}
	i.inspectProxy(&r, engine.Session)
	if r.ProxyImage != nil || !reflect.DeepEqual(r.Issues, []port.Issue{{Kind: port.EgressImage, Subject: proxyReference}}) {
		t.Fatal("unusable proxy observation lost its safe refusal", r)
	}
}
