package features

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
)

// Readiness requests have no body. Keep all credential-fixture request and
// response bytes so the marker scanner audits the actual API transcript.
func (a *doctorAPI) serve(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	recording := a.credentialBodies
	a.mu.Unlock()
	if !recording {
		a.serveEngine(w, r)
		return
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, 65537))
	if err != nil || len(data) > 65536 {
		a.mu.Lock()
		a.bodyError = fmt.Errorf("credential fixture received an unreadable or oversized API request body")
		a.mu.Unlock()
		http.Error(w, "invalid fixture request", http.StatusBadRequest)
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(data))
	response := httptest.NewRecorder()
	a.serveEngine(response, r)
	a.mu.Lock()
	a.bodyJournal = append(a.bodyJournal, string(data), response.Body.String())
	a.mu.Unlock()
	for name, values := range response.Header() {
		w.Header()[name] = values
	}
	w.WriteHeader(response.Code)
	_, _ = w.Write(response.Body.Bytes())
}
