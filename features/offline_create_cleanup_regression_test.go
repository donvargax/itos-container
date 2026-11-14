package features

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

// T-50: cleanup accounts for every forwarded create before it releases a
// volume. Each case runs the cleanup closeOfflineRuntime runs, against the
// disposable engine, and reads the order of the engine's requests.
func TestOfflineRecorderCleanupRemovesAuthorizedWorkers(t *testing.T) {
	token := createTokens[0]
	name := "/containers/itos-container-" + token + "/json"
	volume := "/volumes/itos-container-anchor-" + token
	worker := func(id string) []string {
		return []string{"DELETE /containers/" + id + "?force=true&v=false", "GET /containers/" + id + "/json"}
	}
	volumes := []string{"GET " + volume, "GET " + volume, "DELETE " + volume, "GET " + volume}
	// What every case must leave alone: the product journal, and on a refusal or
	// an error the worker and the volume.
	type fixture struct {
		w       *world
		e       *createEngine
		a       *offlineAPI
		mark    int
		journal int
	}
	prepare := func(t *testing.T) *fixture {
		w, e := createRecorderWorld(t, "recovery")
		e.anchor(w, token)
		return &fixture{w: w, e: e, a: w.offline.api}
	}
	checkpoint := func(f *fixture) {
		f.mark = len(f.e.journal())
		f.a.mu.Lock()
		f.journal = len(f.a.requests)
		f.a.mu.Unlock()
	}
	requireOrder := func(t *testing.T, f *fixture, want ...string) {
		t.Helper()
		if got := f.e.journal()[f.mark:]; !reflect.DeepEqual(got, want) {
			t.Errorf("engine requests of cleanup:\n got %q\nwant %q", got, want)
		}
		f.a.mu.Lock()
		defer f.a.mu.Unlock()
		if len(f.a.requests) != f.journal {
			t.Errorf("cleanup added %d entries to the product journal", len(f.a.requests)-f.journal)
		}
	}
	requireNoDeletion := func(t *testing.T, f *fixture) {
		t.Helper()
		for _, q := range f.e.journal()[f.mark:] {
			if strings.HasPrefix(q, "DELETE ") {
				t.Errorf("a refusal or error still sent %q", q)
			}
		}
		f.e.mu.Lock()
		defer f.e.mu.Unlock()
		if len(f.e.workers) != 1 || len(f.e.volumes) != 1 {
			t.Errorf("a refusal or error changed the engine: workers=%d volumes=%d", len(f.e.workers), len(f.e.volumes))
		}
	}
	requireClean := func(t *testing.T, f *fixture) {
		t.Helper()
		f.e.mu.Lock()
		defer f.e.mu.Unlock()
		if len(f.e.workers) != 0 || len(f.e.volumes) != 0 {
			t.Errorf("cleanup left workers=%d volumes=%d in the engine", len(f.e.workers), len(f.e.volumes))
		}
	}

	t.Run("create still unanswered when cleanup starts", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			f := prepare(t)
			startCreate(t, f.w, f.e, token).abandon(t)
			checkpoint(f)
			done := make(chan error, 1)
			go func() { done <- f.a.cleanup() }()
			synctest.Wait()
			select {
			case err := <-done:
				t.Fatalf("cleanup finished while the create's exchange was open: %v", err)
			default:
			}
			if sent := f.e.journal()[f.mark:]; len(sent) != 0 {
				t.Errorf("cleanup sent %q before the open exchange ended", sent)
			}
			if !f.a.mu.TryLock() {
				t.Fatal("cleanup holds the recorder lock while it waits, so the answer cannot be recorded")
			}
			f.a.mu.Unlock()
			f.e.finish()
			if err := <-done; err != nil {
				t.Errorf("cleanup after the late answer: %v", err)
			}
			id := strings.Repeat("a", 64)
			inspect := "GET /containers/" + id + "/json"
			// The recorder's own inspection of the late worker, then cleanup's.
			requireOrder(t, f, append(append([]string{inspect, inspect, inspect}, worker(id)...), volumes...)...)
			requireClean(t, f)
		})
	})

	// The engine finished the create and dropped the answer: the exchange ended
	// with no worker listed, and the authorized name holds the exact identity.
	unlistedWorker := func(t *testing.T) (*fixture, string) {
		f := prepare(t)
		f.e.drop = true
		f.e.finish()
		startCreate(t, f.w, f.e, token).wait(t)
		if listed := createListed(f.a); len(listed) != 0 {
			t.Fatalf("a dropped answer listed a worker: %v", listed)
		}
		checkpoint(f)
		return f, f.e.worker()
	}
	t.Run("ended without a worker and the name holds the exact identity", func(t *testing.T) {
		f, id := unlistedWorker(t)
		if err := f.a.cleanup(); err != nil {
			t.Errorf("cleanup of an authorized, unlisted worker: %v", err)
		}
		requireOrder(t, f, append(append([]string{"GET " + name}, worker(id)...), volumes...)...)
		requireClean(t, f)
		if listed := createListed(f.a); !reflect.DeepEqual(listed, []string{id}) {
			t.Errorf("cleanup did not record the worker it removed: %v", listed)
		}
	})
	for _, mismatch := range []string{"labels", "image", "name", "short ID"} {
		t.Run("mismatched "+mismatch, func(t *testing.T) {
			f, id := unlistedWorker(t)
			f.e.override = func(method, uri string) (int, any) {
				if method != "GET" || uri != name {
					return 0, nil
				}
				f.e.mu.Lock()
				document := approvalClone(f.e.workers[id])
				f.e.mu.Unlock()
				switch mismatch {
				case "labels":
					object(object(document["Config"])["Labels"])["io.itos-container.project"] = "private-project"
				case "image":
					document["Image"] = "sha256:private-image"
				case "name":
					document["Name"] = "/itos-container-" + createTokens[1]
				case "short ID":
					document["Id"] = id[:12]
				}
				return http.StatusOK, document
			}
			err := f.a.cleanup()
			if err == nil || !strings.Contains(err.Error(), "offline harness refused cleanup without matching full identity") {
				t.Errorf("mismatched %s was not refused with the identity refusal: %v", mismatch, err)
			}
			requireOrder(t, f, "GET "+name)
			requireNoDeletion(t, f)
		})
	}
	t.Run("failed lookup", func(t *testing.T) {
		f, _ := unlistedWorker(t)
		f.e.override = func(method, uri string) (int, any) {
			if method == "GET" && uri == name {
				return http.StatusInternalServerError, map[string]any{"message": "private engine text"}
			}
			return 0, nil
		}
		err := f.a.cleanup()
		if err == nil || !strings.Contains(err.Error(), "HTTP 500") || strings.Contains(err.Error(), "private") || strings.Contains(err.Error(), token) {
			t.Errorf("failed lookup: %v", err)
		}
		requireOrder(t, f, "GET "+name)
		requireNoDeletion(t, f)
	})
	t.Run("absent name", func(t *testing.T) {
		f := prepare(t)
		f.e.override = func(method, _ string) (int, any) {
			if method == "POST" {
				return http.StatusInternalServerError, map[string]any{"message": "synthetic engine refusal"}
			}
			return 0, nil
		}
		f.a.serve(httptest.NewRecorder(), createRequest(t.Context(), f.w, token))
		f.e.override = nil
		checkpoint(f)
		if err := f.a.cleanup(); err != nil {
			t.Errorf("cleanup with nothing under the authorized name: %v", err)
		}
		requireOrder(t, f, append([]string{"GET " + name}, volumes...)...)
		requireClean(t, f)
	})
	t.Run("exchange ended at its allowance", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			f := prepare(t)
			started := time.Now()
			startCreate(t, f.w, f.e, token).abandon(t)
			checkpoint(f)
			err := f.a.cleanup()
			if elapsed := time.Since(started); elapsed != engineCreateOutcomeAllowance {
				t.Errorf("cleanup waited %s for the open exchange, want the %s allowance", elapsed, engineCreateOutcomeAllowance)
			}
			// Reported even though the name is absent: the engine may still finish it.
			if err == nil || !strings.Contains(err.Error(), "allowance") || strings.Contains(err.Error(), token) {
				t.Errorf("an exchange that ended at its allowance was not a failure of its own: %v", err)
			}
			requireOrder(t, f, "GET "+name)
			for _, q := range f.e.journal()[f.mark:] {
				if strings.HasPrefix(q, "DELETE ") {
					t.Errorf("cleanup released %q after an expired exchange", q)
				}
			}
		})
	})

	// One worker listed by an ordinary create, as every passing row leaves it.
	listedWorker := func(t *testing.T) (*fixture, string) {
		f := prepare(t)
		f.e.finish()
		startCreate(t, f.w, f.e, token).wait(t)
		id := f.e.worker()
		if listed := createListed(f.a); !reflect.DeepEqual(listed, []string{id}) {
			t.Fatalf("ordinary create listed %v", listed)
		}
		checkpoint(f)
		return f, id
	}
	t.Run("one listed worker and its volume", func(t *testing.T) {
		f, id := listedWorker(t)
		if err := f.a.cleanup(); err != nil {
			t.Errorf("cleanup of a listed worker: %v", err)
		}
		inspect := "GET /containers/" + id + "/json"
		requireOrder(t, f, append(append([]string{inspect, inspect}, worker(id)...), volumes...)...)
		requireClean(t, f)
	})
	for _, listing := range []string{"available", "unavailable"} {
		t.Run("conflict counts "+listing, func(t *testing.T) {
			f, id := listedWorker(t)
			var listings []url.Values
			f.e.override = func(method, uri string) (int, any) {
				switch {
				case method == "DELETE" && uri == volume:
					return http.StatusConflict, map[string]any{"message": "remove private-volume: volume is in use - [private-container]"}
				case method == "GET" && strings.HasPrefix(uri, "/containers/json?"):
					query, _ := url.ParseQuery(strings.TrimPrefix(uri, "/containers/json?"))
					listings = append(listings, query)
					if listing == "unavailable" {
						return http.StatusInternalServerError, map[string]any{"message": "private engine text"}
					}
					// The listed worker, already reported absent by full ID, and a foreign holder.
					return http.StatusOK, []any{map[string]any{"Id": id, "Names": []any{"/private-name"}}, map[string]any{"Id": strings.Repeat("f", 64)}}
				}
				return 0, nil
			}
			err := f.a.cleanup()
			want := "native anchor harness cleanup HTTP 409 reason=in-use holders=2 recorded=1"
			if listing == "unavailable" {
				want = "native anchor harness cleanup HTTP 409 reason=in-use holders=unavailable"
			}
			if err == nil || err.Error() != want {
				t.Errorf("volume conflict = %v, want %q", err, want)
			}
			deletions := 0
			for _, q := range f.e.journal()[f.mark:] {
				if q == "DELETE "+volume {
					deletions++
				}
			}
			var filters map[string][]string
			if len(listings) == 1 {
				_ = json.Unmarshal([]byte(listings[0].Get("filters")), &filters)
			}
			if deletions != 1 || len(listings) != 1 || listings[0].Get("all") != "1" || !reflect.DeepEqual(filters, map[string][]string{"volume": {strings.TrimPrefix(volume, "/volumes/")}}) {
				t.Errorf("conflict diagnosis sent %d deletions and listings %v, want one deletion and one listing of every container mounting the volume", deletions, listings)
			}
			f.a.mu.Lock()
			defer f.a.mu.Unlock()
			if len(f.a.requests) != f.journal {
				t.Error("conflict diagnosis entered the product journal")
			}
		})
	}
}
