package domain

import "testing"

func TestExposureUsesWholeCanonicalComponents(t *testing.T) {
	for _, tc := range []struct {
		location, worker string
		want             bool
	}{
		{"/work", "/work", true}, {"/work/state/itos-container", "/work", true},
		{"/work-more/state", "/work", false}, {"/a/work/state", "/b/work", false},
		{"/work", "/work/child", false}, {"/state", "/", true},
	} {
		if got := Exposed(tc.location, tc.worker); got != tc.want {
			t.Errorf("exposure(%q,%q)=%v", tc.location, tc.worker, got)
		}
	}
}
