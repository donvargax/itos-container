package port

import (
	"context"
	"errors"
	"testing"
)

func TestDimensionBounds(t *testing.T) {
	for _, d := range []Dimensions{{1, 1}, {65535, 65535}, {93, 31}} {
		if !d.Valid() {
			t.Errorf("valid dimensions refused: %v", d)
		}
	}
	for _, d := range []Dimensions{{0, 1}, {-1, 1}, {65536, 1}, {1, 0}, {1, -1}, {1, 65536}} {
		if d.Valid() {
			t.Errorf("invalid dimensions accepted: %v", d)
		}
	}
}

func TestFailureRetainsTypedCause(t *testing.T) {
	err := &Failure{Kind: Cancelled, Err: context.Canceled}
	if err.Error() != "terminal: cancelled" || !errors.Is(err, context.Canceled) {
		t.Fatalf("failure lost its kind/cause: %v", err)
	}
}
