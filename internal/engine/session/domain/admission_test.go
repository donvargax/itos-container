package domain

import "testing"

func TestNestedCheckouts(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want bool
	}{
		{"/work", "/work", false}, {"/work", "/work/child", true},
		{"/work/middle/child", "/work", true}, {"/work", "/work-more", false},
		{"/projects/work", "/other/work", false}, {"/", "/work", true},
		{"/work", "/", true},
	} {
		if got := NestedCheckouts(tc.a, tc.b); got != tc.want {
			t.Errorf("%q vs %q: %v", tc.a, tc.b, got)
		}
	}
}
