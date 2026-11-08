package docker

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	sp "github.com/donvargax/itos-container/internal/engine/session/port"
	st "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
	"github.com/moby/moby/api/types/volume"
)

func TestSDKAnchorContract(t *testing.T) {
	volumes := map[string]volume.Volume{}
	e, b, _ := sdkHandlerFixture(t, func(w http.ResponseWriter, r *http.Request, _ sp.Binding) {
		path := strings.TrimPrefix(r.URL.Path, "/v1.56/volumes/")
		switch r.Method {
		case "POST":
			var request struct {
				Name, Driver       string
				DriverOpts, Labels map[string]string
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil || path != "create" {
				t.Error("invalid allocation", err)
				w.WriteHeader(400)
				return
			}
			if _, exists := volumes[request.Name]; !exists {
				volumes[request.Name] = volume.Volume{Name: request.Name, Driver: request.Driver, Scope: "local", CreatedAt: "2026-11-07T17:00:00Z", Options: request.DriverOpts, Labels: request.Labels}
			}
			w.WriteHeader(201)
			_ = json.NewEncoder(w).Encode(volumes[request.Name])
		case "GET":
			v, ok := volumes[path]
			if !ok {
				w.WriteHeader(404)
				return
			}
			_ = json.NewEncoder(w).Encode(v)
		case "DELETE":
			if r.URL.Query().Get("force") != "" && r.URL.Query().Get("force") != "false" {
				t.Error("anchor deletion requested force")
			}
			delete(volumes, path)
			w.WriteHeader(204)
		default:
			t.Error("unexpected anchor request", r.Method, path)
			w.WriteHeader(400)
		}
	})
	c, f := e.OpenRun(context.Background(), b)
	if f != "" {
		t.Fatal(f)
	}
	defer c.Close()
	r := st.RunIntent(b)
	r.Stage, r.Anchor.Stage, r.Anchor.Created = "reserved", "intent", ""
	st.AnchorConnectionContract(t, c, r)
	if len(volumes) != 0 {
		t.Fatal("contract retained an anchor")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if f := c.AllocateAnchor(ctx, r); f != sp.MountUncertain {
		t.Fatal(f)
	}
	if _, status := c.InspectAnchor(ctx, r.Anchor.Name); status != sp.Unavailable {
		t.Fatal(status)
	}
}

func TestSDKAnchorRefusesChangedSocketBeforeAnyRequest(t *testing.T) {
	e, b, pin := sdkHandlerFixture(t, func(w http.ResponseWriter, r *http.Request, _ sp.Binding) {
		t.Errorf("changed socket reached engine: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusBadRequest)
	})
	c, failure := e.OpenRun(t.Context(), b)
	if failure != "" {
		t.Fatal(failure)
	}
	defer c.Close()
	r := st.RunIntent(b)
	r.Stage, r.Anchor.Stage, r.Anchor.Created = "reserved", "intent", ""
	pin.Current = false
	if got := c.AllocateAnchor(t.Context(), r); got != sp.MountRefused {
		t.Fatal("changed socket authorized allocation", got)
	}
	if o, got := c.InspectAnchor(t.Context(), r.Anchor.Name); got != sp.Changed || o != (sp.AnchorObservation{}) {
		t.Fatal("changed socket supplied anchor authority", o, got)
	}
	c.RemoveAnchor(t.Context(), r.Anchor.Name)
}

func TestSDKAnchorAndWorkerCreationRequireRecordedAnchorAuthority(t *testing.T) {
	for _, fault := range []string{"legacy", "foreign inode", "missing creation", "wrong stage"} {
		t.Run(fault, func(t *testing.T) {
			e, b, _ := sdkHandlerFixture(t, func(w http.ResponseWriter, r *http.Request, _ sp.Binding) {
				t.Errorf("invalid anchor reached engine: %s %s", r.Method, r.URL.Path)
				w.WriteHeader(http.StatusBadRequest)
			})
			c, failure := e.OpenRun(t.Context(), b)
			if failure != "" {
				t.Fatal(failure)
			}
			defer c.Close()
			r := st.RunIntent(b)
			switch fault {
			case "legacy":
				r.Schema, r.Anchor = 1, sp.Anchor{}
			case "foreign inode":
				r.Anchor.Inode = "replacement"
			case "missing creation":
				r.Anchor.Created = ""
			case "wrong stage":
				r.Anchor.Stage = "absent"
			}
			if got := c.AllocateAnchor(t.Context(), r); got != sp.MountRefused {
				t.Fatal("invalid allocation authority", got)
			}
			if id, got := c.CreateRun(t.Context(), r, []string{"probe"}); id != "" || got != sp.MountRefused {
				t.Fatal("invalid anchor authorized worker creation", id, got)
			}
		})
	}
}
