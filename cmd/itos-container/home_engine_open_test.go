package main

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	rd "github.com/donvargax/itos-container/internal/engine/readiness/docker"
	sd "github.com/donvargax/itos-container/internal/engine/session/docker"
	sp "github.com/donvargax/itos-container/internal/engine/session/port"
)

func TestHomeCatalogOpensOriginalDaemonWithoutCheckout(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("native Unix engine adapter target is Linux")
	}
	root, err := os.MkdirTemp("", "slice-34-catalog-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	path := filepath.Join(root, "engine.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{ReadHeaderTimeout: time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch strings.TrimPrefix(r.URL.Path, "/v1.56") {
		case "/_ping":
			w.Header().Set("API-Version", "1.56")
		case "/version":
			_ = json.NewEncoder(w).Encode(map[string]any{"Platform": map[string]string{"Name": "Docker Engine - Community"}, "Os": "linux", "Arch": "amd64", "ApiVersion": "1.56", "MinAPIVersion": "1.44"})
		case "/info":
			_ = json.NewEncoder(w).Encode(map[string]string{"ID": "original-daemon", "OSType": "linux", "Architecture": "amd64", "OperatingSystem": "Linux"})
		case "/volumes/retained-home":
			_ = json.NewEncoder(w).Encode(map[string]string{"Name": "retained-home"})
		default:
			t.Error("unexpected catalog request", r.Method, r.URL)
			w.WriteHeader(400)
		}
	})}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })
	endpoint := "unix://" + path
	for _, defect := range []string{"none", "endpoint", "daemon", "remote", "missing", "checkout socket"} {
		t.Run(defect, func(t *testing.T) {
			scope := sp.HomeScope{UID: "1000", GID: "1001", Engine: sp.HomeEngineIdentity{Endpoint: endpoint, ID: "original-daemon"}}
			b := scope.Binding()
			selection := endpoint
			switch defect {
			case "endpoint":
				b.Endpoint += "-other"
			case "daemon":
				b.EngineID = "other-daemon"
			case "remote":
				selection = "tcp://127.0.0.1:2375"
			case "missing":
				selection += "-missing"
			case "checkout socket":
				b.Checkout.Root = root
			}
			c, f := (sd.Engine{Readiness: rd.Engine{}, Selection: selection}).OpenRun(t.Context(), b)
			if defect != "none" {
				want := sp.Refused
				if defect == "missing" {
					want = sp.EnvironmentMissing
				}
				if c != nil || f != want {
					t.Fatalf("catalog accepted changed authority: connection %v, failure %s", c, f)
				}
				return
			}
			if f != "" || c == nil {
				t.Fatalf("catalog rejected original daemon without checkout: %s", f)
			}
			defer c.Close()
			if f := c.Check(t.Context()); f != "" {
				t.Fatal("daemon check", f)
			}
			o, status := c.(sp.HomeConnection).InspectHome(t.Context(), "retained-home")
			if status != sp.Observed || o.Name != "retained-home" {
				t.Fatal("catalog did not inspect volume", o, status)
			}
			if err := os.Rename(path, path+"-original"); err != nil {
				t.Fatal(err)
			}
			if f := c.Check(t.Context()); f != sp.Refused {
				t.Fatal("catalog lost socket pin", f)
			}
			if err := os.Rename(path+"-original", path); err != nil {
				t.Fatal(err)
			}
		})
	}
}
