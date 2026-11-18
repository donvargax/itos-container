package main

import (
	"testing"

	sf "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
)

func TestHomeAdapterContracts(t *testing.T) {
	for _, backend := range []string{"memory", "disk"} {
		t.Run(backend, func(t *testing.T) {
			t.Setenv("SLICE_34_HOME_BACKEND", backend)
			t.Run("store", func(t *testing.T) {
				f := newHomeLocalFixture(t, "1000", "1000")
				sf.HomeStoreContract(t, f.service().Store, f.m.b)
			})
			t.Run("engine", func(t *testing.T) {
				f := newHomeLocalFixture(t, "1000", "1000")
				sf.HomeEngineContract(t, f.runEngine(), f.m.b, homeFixtureCodec{}.Scope(f.m.b))
			})
		})
	}
}
