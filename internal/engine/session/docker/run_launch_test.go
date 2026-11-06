package docker

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"

	rp "github.com/donvargax/itos-container/internal/engine/readiness/port"
	sp "github.com/donvargax/itos-container/internal/engine/session/port"
	st "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
)

func TestOpenRunRefusesUnavailableOrChangedEngine(t *testing.T) {
	for _, fault := range []string{"canceled", "unavailable", "foreign endpoint", "foreign daemon"} {
		t.Run(fault, func(t *testing.T) {
			e, b, pin := sdkFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			want := sp.Refused
			switch fault {
			case "canceled":
				cancel()
				want = sp.Incomplete
			case "unavailable":
				pin.Failure = rp.EngineUnavailable
				want = sp.Incomplete
			case "foreign endpoint":
				b.Endpoint = "unix:///foreign"
			case "foreign daemon":
				pin.InfoValue.ID = "foreign"
			}
			c, got := e.OpenRun(ctx, b)
			if c != nil {
				c.Close()
				t.Fatal("failed open returned a usable run connection")
			}
			if got != want {
				t.Fatalf("open = %s, want %s", got, want)
			}
		})
	}
}

func TestCreateRunRequiresSafeImageAndCertainIdentity(t *testing.T) {
	for _, tc := range []struct {
		fault           string
		want            sp.Outcome
		images, creates int32
	}{
		{"healthy", "", 1, 1},
		{"socket after image", sp.Refused, 1, 0},
		{"invalid environment", sp.Invalid, 1, 0},
		{"create unavailable", sp.Uncertain, 1, 1},
		{"malformed create", sp.Uncertain, 1, 1},
		{"short ID", sp.Uncertain, 1, 1},
		{"socket after create", sp.Uncertain, 1, 1},
	} {
		t.Run(tc.fault, func(t *testing.T) {
			var images, creates atomic.Int32
			id := strings.Repeat("c", 64)
			e, b, pin := sdkHandlerFixture(t, func(w http.ResponseWriter, r *http.Request, b sp.Binding) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/v1.56/images/"+b.ImageID+"/json":
					images.Add(1)
					_ = json.NewEncoder(w).Encode(map[string]any{"Id": b.ImageID, "Config": map[string]any{}})
				case r.Method == http.MethodPost && r.URL.Path == "/v1.56/containers/create":
					creates.Add(1)
					switch tc.fault {
					case "create unavailable":
						w.WriteHeader(http.StatusServiceUnavailable)
					case "malformed create":
						_, _ = w.Write([]byte(`{"Id":`))
					case "short ID":
						_ = json.NewEncoder(w).Encode(map[string]string{"Id": "short"})
					default:
						w.WriteHeader(http.StatusCreated)
						_ = json.NewEncoder(w).Encode(map[string]string{"Id": id})
					}
				default:
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusBadRequest)
				}
			})
			b.Environment = `{}`
			if tc.fault == "invalid environment" {
				b.Environment = `{"APP":`
			}
			ctx := context.Background()
			c, failure := e.OpenRun(ctx, b)
			if failure != "" {
				t.Fatal(failure)
			}
			defer c.Close()
			run := c.(*runConnection)
			switch tc.fault {
			case "socket after image":
				replaceSocketOnReply(t, run.connection, pin, b.Endpoint, "/v1.56/images/"+b.ImageID+"/json")
			case "socket after create":
				replaceSocketOnReply(t, run.connection, pin, b.Endpoint, "/v1.56/containers/create")
			}
			gotID, got := c.CreateRun(ctx, b.Intent(strings.Repeat("1", 64)), []string{"probe", "two words", "$HOME"})
			if got != tc.want || (tc.want == "" && gotID != id) || (tc.want != "" && gotID != "") {
				t.Fatalf("create = %q, %s; want outcome %s", gotID, got, tc.want)
			}
			if (run.expected != nil) != (tc.want == "") {
				t.Fatal("uncertain create retained launch authority")
			}
			if images.Load() != tc.images || creates.Load() != tc.creates {
				t.Fatalf("requests image/create = %d/%d, want %d/%d", images.Load(), creates.Load(), tc.images, tc.creates)
			}
		})
	}
}

func TestVerifyRunRevalidatesSocketAroundInspection(t *testing.T) {
	for _, afterReply := range []bool{false, true} {
		e, b, _ := runSDKFixture(t)
		ctx := context.Background()
		c, failure := e.OpenRun(ctx, b)
		if failure != "" {
			t.Fatal(failure)
		}
		defer c.Close()
		argv := []string{"probe"}
		id, failure := c.CreateRun(ctx, b.Intent(strings.Repeat("1", 64)), argv)
		if failure != "" {
			t.Fatal(failure)
		}
		if got := c.VerifyRun(ctx, id, argv); got != "" {
			t.Fatal("healthy boundary control", got)
		}
		pin := e.Readiness.(*st.Readiness)
		if afterReply {
			replaceSocketOnReply(t, c.(*runConnection).connection, pin, b.Endpoint, "/v1.56/containers/"+id+"/json")
		} else {
			pin.Current = false
		}
		if got := c.VerifyRun(ctx, id, argv); got != sp.Refused {
			t.Fatalf("socket substitution accepted (after reply %v): %s", afterReply, got)
		}
	}
}

func TestStartRunRefusesChangedSocketAndRequestFailure(t *testing.T) {
	for _, changed := range []bool{false, true} {
		var starts atomic.Int32
		id := strings.Repeat("c", 64)
		e, b, pin := sdkHandlerFixture(t, func(w http.ResponseWriter, r *http.Request, _ sp.Binding) {
			starts.Add(1)
			if r.Method != http.MethodPost || r.URL.Path != "/v1.56/containers/"+id+"/start" {
				t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			}
			w.WriteHeader(http.StatusServiceUnavailable)
		})
		c, failure := e.OpenRun(context.Background(), b)
		if failure != "" {
			t.Fatal(failure)
		}
		defer c.Close()
		want, requests := sp.ExecutionFailed, int32(1)
		if changed {
			pin.Current = false
			want, requests = sp.Refused, 0
		}
		if got := c.Start(context.Background(), id); got != want || starts.Load() != requests {
			t.Fatalf("start = %s after %d requests, want %s after %d", got, starts.Load(), want, requests)
		}
	}
}

func TestVerifyRunAcceptsExplicitTmpfsMount(t *testing.T) {
	e, b, a := runSDKFixture(t)
	c, failure := e.OpenRun(context.Background(), b)
	if failure != "" {
		t.Fatal(failure)
	}
	defer c.Close()
	argv := []string{"probe"}
	id, failure := c.CreateRun(context.Background(), b.Intent(strings.Repeat("1", 64)), argv)
	if failure != "" {
		t.Fatal(failure)
	}
	a.mu.Lock()
	a.defect = func(r *container.InspectResponse) {
		r.Mounts = append(r.Mounts, container.MountPoint{Type: mount.TypeTmpfs, Destination: "/tmp"})
	}
	a.mu.Unlock()
	if got := c.VerifyRun(context.Background(), id, argv); got != "" {
		t.Fatal("explicit tmpfs mount refused", got)
	}
}
