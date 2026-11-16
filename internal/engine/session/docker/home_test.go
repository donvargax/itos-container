package docker

import (
	"archive/tar"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	rp "github.com/donvargax/itos-container/internal/engine/readiness/port"
	sp "github.com/donvargax/itos-container/internal/engine/session/port"
	st "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
)

// Hiding the optional catalog capability must fail closed, not pass an empty
// root to the ordinary checkout-aware opener.
type checkoutOnlyReadiness struct{ rp.Engine }

type catalogOnlyReadiness struct {
	*st.Readiness
	t *testing.T
}

func (r catalogOnlyReadiness) Open(string, string, time.Duration) (rp.Session, rp.Failure) {
	r.t.Fatal("catalog attempted checkout-aware open")
	return nil, rp.EngineSelection
}

func TestHomeCatalogRequiresExplicitOpeningCapability(t *testing.T) {
	engine, b, pin := sdkFixture(t)
	b.Checkout = rp.CheckoutIdentity{}
	engine.Readiness = checkoutOnlyReadiness{pin}
	if c, f := engine.OpenRun(t.Context(), b); c != nil || f != sp.Refused {
		t.Fatal("catalog accepted an engine without the capability", c, f)
	}
	engine.Readiness = catalogOnlyReadiness{pin, t}
	c, f := engine.OpenRun(t.Context(), b)
	if f != "" || c == nil {
		t.Fatal("explicit catalog opener failed", f)
	}
	c.Close()
}

func homeAdapterConnection(t *testing.T, handler http.HandlerFunc) (*runConnection, *st.Readiness) {
	t.Helper()
	e, b, pin := sdkHandlerFixture(t, func(w http.ResponseWriter, r *http.Request, _ sp.Binding) { handler(w, r) })
	c, failure := e.OpenRun(t.Context(), b)
	if failure != "" {
		t.Fatal(failure)
	}
	t.Cleanup(c.Close)
	return c.(*runConnection), pin
}

func TestHomeInspectionPreservesEvidenceAndClassifiesErrors(t *testing.T) {
	for _, status := range []int{200, 404, 503} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			c, _ := homeAdapterConnection(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" || !strings.HasSuffix(r.URL.Path, "/volumes/retained") {
					t.Error("unexpected inspection", r.Method, r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				_ = json.NewEncoder(w).Encode(map[string]any{"Name": "retained", "CreatedAt": "original", "Driver": "local", "Scope": "local", "Labels": map[string]string{"io.itos-container.kind": "harness-home", "io.itos-container.scope": "checkout", "io.itos-container.token": "token"}})
			})
			got, inspection := c.InspectHome(t.Context(), "retained")
			want := sp.HomeObservation{Name: "retained", Created: "original", Driver: "local", Scope: "local", Token: "token", Key: "checkout", EmptyOptions: true, ExactLabels: true}
			if status == 200 {
				if got != want || inspection != sp.Observed {
					t.Fatal(got, inspection)
				}
			} else {
				state := sp.Unavailable
				if status == 404 {
					state = sp.NotFound
				}
				if got != (sp.HomeObservation{}) || inspection != state {
					t.Fatal(got, inspection)
				}
			}
		})
	}
}

func TestHomeRootRequiresBothStatAndArchiveEvidence(t *testing.T) {
	for _, defect := range []string{"valid", "directory without slash", "unavailable", "malformed archive", "not directory", "stat mode", "setuid", "setgid", "sticky", "link", "archive type", "archive name", "archive mode", "uid", "gid"} {
		t.Run(defect, func(t *testing.T) {
			mode, link := os.ModeDir|0700, ""
			h := tar.Header{Name: "itos/", Typeflag: tar.TypeDir, Mode: 0700, Uid: 1000, Gid: 1000}
			want := sp.HomeRefused
			switch defect {
			case "valid":
				want = ""
			case "directory without slash":
				h.Name, want = "itos", ""
			case "unavailable", "malformed archive":
				want = sp.HomeUnavailable
			case "not directory":
				mode = 0700
			case "stat mode":
				mode = os.ModeDir | 0755
			case "setuid":
				mode |= os.ModeSetuid
			case "setgid":
				mode |= os.ModeSetgid
			case "sticky":
				mode |= os.ModeSticky
			case "link":
				link = "elsewhere"
			case "archive type":
				h.Typeflag, h.Name = tar.TypeReg, "itos"
			case "archive name":
				h.Name = "another/"
			case "archive mode":
				h.Mode = 0755
			case "uid":
				h.Uid = 1001
			case "gid":
				h.Gid = 1001
			}
			var archive bytes.Buffer
			w := tar.NewWriter(&archive)
			if err := w.WriteHeader(&h); err != nil {
				t.Fatal(err)
			}
			if err := w.Close(); err != nil {
				t.Fatal(err)
			}
			stat, err := json.Marshal(map[string]any{"name": "itos", "mode": mode, "linkTarget": link})
			if err != nil {
				t.Fatal(err)
			}
			c, _ := homeAdapterConnection(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" || r.URL.Query().Get("path") != "/home/itos" {
					t.Error("unexpected root read", r.Method, r.URL.String())
				}
				if defect == "unavailable" {
					w.WriteHeader(503)
					return
				}
				w.Header().Set("X-Docker-Container-Path-Stat", base64.StdEncoding.EncodeToString(stat))
				if defect == "malformed archive" {
					_, _ = w.Write([]byte("not a tar header"))
					return
				}
				_, _ = w.Write(archive.Bytes())
			})
			if got := c.VerifyHomeRoot(t.Context(), strings.Repeat("c", 64), "1000", "1000"); got != want {
				t.Fatalf("root verification = %s, want %s", got, want)
			}
		})
	}
}

func TestHomeInvalidRequestsAndLostPinDoNotReachEngine(t *testing.T) {
	var calls atomic.Int32
	c, pin := homeAdapterConnection(t, func(w http.ResponseWriter, _ *http.Request) { calls.Add(1); w.WriteHeader(500) })
	id := strings.Repeat("a", 64)
	fresh := sp.HomeReference{ID: id, Scope: strings.Repeat("b", 64), Name: "itos-container-home-" + id}
	owned := fresh
	owned.Created = "original"
	for _, h := range []sp.HomeReference{{}, owned} {
		if got := c.AllocateHome(t.Context(), h); got != sp.HomeRefused {
			t.Fatal(got)
		}
	}
	for _, args := range [][3]string{{"invalid", "1000", "1000"}, {id, "0", "1000"}, {id, "bad", "1000"}, {id, "1000", "bad"}} {
		if got := c.InitializeHome(t.Context(), args[0], args[1], args[2]); got != sp.HomeRefused {
			t.Fatal(args, got)
		}
	}
	r := c.binding.Intent(id)
	r.Home = sp.HomeReference{ID: "invalid"}
	if _, got := c.CreateRun(t.Context(), r, []string{"probe"}); got != sp.HomeRefused {
		t.Fatal(got)
	}
	pin.Current = false
	if got := c.AllocateHome(t.Context(), fresh); got != sp.HomeRefused {
		t.Fatal(got)
	}
	if _, got := c.InspectHome(t.Context(), fresh.Name); got != sp.Changed {
		t.Fatal(got)
	}
	if got := c.InitializeHome(t.Context(), id, "1000", "1000"); got != sp.HomeRefused {
		t.Fatal(got)
	}
	if got := c.VerifyHomeRoot(t.Context(), id, "1000", "1000"); got != sp.HomeRefused {
		t.Fatal(got)
	}
	if got := c.RemoveHome(t.Context(), fresh.Name); got != sp.Refused {
		t.Fatal(got)
	}
	if calls.Load() != 0 {
		t.Fatal("refused request reached engine", calls.Load())
	}
}

func TestHomeRemovalRechecksPinAfterReply(t *testing.T) {
	c, pin := homeAdapterConnection(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" || r.URL.Query().Get("force") == "1" {
			t.Error("unsafe removal", r.Method, r.URL.String())
		}
		w.WriteHeader(204)
	})
	replaceSocketOnReply(t, c.connection, pin, c.binding.Endpoint, "")
	if got := c.RemoveHome(t.Context(), "owned"); got != sp.Refused {
		t.Fatal("removal accepted changed engine identity", got)
	}
}

func TestHomeMutationsRequireAcknowledgementAndUnchangedPin(t *testing.T) {
	id := strings.Repeat("a", 64)
	home := sp.HomeReference{ID: id, Scope: strings.Repeat("b", 64), Name: "itos-container-home-" + id}
	for _, operation := range []string{"allocate", "initialize", "remove"} {
		for _, reply := range []string{"success", "unavailable", "changed engine"} {
			t.Run(operation+"/"+reply, func(t *testing.T) {
				c, pin := homeAdapterConnection(t, func(w http.ResponseWriter, r *http.Request) {
					path := strings.TrimPrefix(r.URL.Path, "/v1.56")
					switch operation {
					case "allocate":
						var request struct {
							Name, Driver       string
							Labels, DriverOpts map[string]string
						}
						if r.Method != "POST" || path != "/volumes/create" || json.NewDecoder(r.Body).Decode(&request) != nil || request.Name != home.Name || request.Driver != "local" || len(request.DriverOpts) != 0 || len(request.Labels) != 3 || request.Labels["io.itos-container.kind"] != "harness-home" || request.Labels["io.itos-container.scope"] != home.Scope || request.Labels["io.itos-container.token"] != home.ID {
							t.Error("allocation did not carry the exact volume identity")
						}
					case "initialize":
						archive := tar.NewReader(r.Body)
						h, err := archive.Next()
						if r.Method != "PUT" || path != "/containers/"+id+"/archive" || r.URL.Query().Get("path") != "/home/itos" || r.URL.Query().Get("copyUIDGID") != "true" || err != nil || h.Name != "/" || h.Typeflag != tar.TypeDir || h.Mode != 0700 || h.Uid != 2001 || h.Gid != 3001 || h.Size != 0 {
							t.Error("initialization did not carry only the private-root metadata", h, err)
						}
						if _, err := archive.Next(); err != io.EOF {
							t.Error("initialization archive contained more than root metadata", err)
						}
					case "remove":
						if r.Method != "DELETE" || path != "/volumes/"+home.Name || r.URL.Query().Get("force") == "1" {
							t.Error("unsafe home removal", r.Method, r.URL.String())
						}
					}
					if reply == "unavailable" {
						w.WriteHeader(503)
						return
					}
					if operation == "allocate" {
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(201)
						_ = json.NewEncoder(w).Encode(map[string]string{"Name": home.Name})
					} else {
						w.WriteHeader(200)
					}
				})
				if reply == "changed engine" {
					replaceSocketOnReply(t, c.connection, pin, c.binding.Endpoint, "")
				}
				var got sp.Outcome
				switch operation {
				case "allocate":
					got = c.AllocateHome(t.Context(), home)
				case "initialize":
					got = c.InitializeHome(t.Context(), id, "2001", "3001")
				case "remove":
					got = c.RemoveHome(t.Context(), home.Name)
				}
				want := sp.Outcome("")
				if reply != "success" {
					want = sp.HomeUnavailable
					if operation == "remove" {
						want = sp.Incomplete
						if reply == "changed engine" {
							want = sp.Refused
						}
					}
				}
				if got != want {
					t.Fatalf("mutation acknowledgement = %s, want %s", got, want)
				}
			})
		}
	}
}
