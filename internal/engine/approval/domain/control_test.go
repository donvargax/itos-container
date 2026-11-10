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

func TestOverlapIsSymmetricAcrossWholeComponents(t *testing.T) {
	for _, tc := range []struct {
		location, worker string
		want             bool
	}{
		{"/work", "/work", true}, {"/work/state/itos-container", "/work", true},
		{"/state/itos-container", "/state/itos-container/approvals", true},
		{"/state/itos-container", "/state/itos-container/approvals/deep/checkout", true},
		{"/work-more/state", "/work", false}, {"/work", "/work-more/state", false},
		{"/state/itos-container-more", "/state/itos-container/approvals", false},
		{"/state/itos-container", "/state/itos-container-more/approvals", false},
		{"/a/work/state", "/b/work", false}, {"/state", "/", true}, {"/", "/work", true},
	} {
		if got := Overlaps(tc.location, tc.worker); got != tc.want {
			t.Errorf("overlap(%q,%q)=%v", tc.location, tc.worker, got)
		}
		if got := Overlaps(tc.worker, tc.location); got != tc.want {
			t.Errorf("overlap(%q,%q)=%v", tc.worker, tc.location, got)
		}
	}
}
