package features

import (
	"net/http"
	"strings"
	"testing"
)

// launchObserved reports whether the controller with this PID has asked the
// engine to wait on the worker, which it does only after launch is complete.
// The path suffix is matched as the owned /kill gate matches it, so an API
// version prefix does not matter and the query string is not part of it.
func launchObserved(requests []doctorRequest, pid int, id string) bool {
	for _, request := range requests {
		if request.PID == pid && request.Method == http.MethodPost && strings.HasSuffix(request.Path, "/containers/"+id+"/wait") {
			return true
		}
	}
	return false
}

func TestLaunchObserved(t *testing.T) {
	const id = "0123456789abcdef"
	const pid = 4242
	wait := doctorRequest{Method: http.MethodPost, Path: "/v1.56/containers/" + id + "/wait", PID: pid}
	cases := []struct {
		name     string
		requests []doctorRequest
		want     bool
	}{
		{"accepts_wait_from_pid", []doctorRequest{wait}, true},
		{"rejects_wait_from_other_pid", []doctorRequest{{Method: wait.Method, Path: wait.Path, PID: pid + 1}}, false},
		{"rejects_other_method", []doctorRequest{
			{Method: http.MethodGet, Path: wait.Path, PID: pid},
			{Method: http.MethodPut, Path: wait.Path, PID: pid},
		}, false},
		{"rejects_other_requests", []doctorRequest{
			{Method: http.MethodPost, Path: "/v1.56/containers/" + id + "/start", PID: pid},
			{Method: http.MethodGet, Path: "/v1.56/containers/" + id + "/json", PID: pid},
			{Method: http.MethodPost, Path: "/v1.56/containers/" + id + "/attach", PID: pid},
			{Method: http.MethodPost, Path: "/v1.56/containers/" + id + "/kill", PID: pid},
		}, false},
		{"rejects_empty_journal", nil, false},
		{"accepts_query_string", []doctorRequest{{Method: http.MethodPost, Path: "/containers/" + id + "/wait", Query: "condition=not-running", PID: pid}}, true},
		{"rejects_other_container", []doctorRequest{{Method: http.MethodPost, Path: "/v1.56/containers/fedcba9876543210/wait", PID: pid}}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := launchObserved(c.requests, pid, id); got != c.want {
				t.Errorf("launchObserved(%v, %d, %q) = %t, want %t", c.requests, pid, id, got, c.want)
			}
		})
	}
}
