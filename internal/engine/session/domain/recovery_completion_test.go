package domain

import (
	"bytes"
	"strings"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/session/port"
)

func TestFinishSessionRequiresAConcreteFullWorkerID(t *testing.T) {
	for _, id := range []string{"", strings.Repeat("c", 63), strings.Repeat("C", 64)} {
		t.Run("worker="+id, func(t *testing.T) {
			_, registry, _, _, sessionID := registeredSessionFixture(t)
			r := registry.Records[sessionID]
			store := registry.Store
			i, failure := store.Codec.Decode(store.Records[r.Intent])
			if failure != "" {
				t.Fatal(failure)
			}
			// The stateful fake can supply an intent without valid concrete-ID
			// evidence, independently of disk/codec validation. Completion must
			// refuse it before trying to publish an absent intent or tombstone.
			i.ContainerID = id
			store.Records[r.Intent] = store.Codec.Encode(i)
			before := bytes.Clone(store.Records[r.Intent])
			writes := len(store.Events)
			entry, failure := registry.Open(t.Context(), sessionID, false)
			if failure != "" {
				t.Fatal(failure)
			}
			defer entry.Close()

			if failure = finishSession(entry); failure != port.Uncertain {
				t.Fatal("completion accepted missing or invalid concrete identity", failure)
			}
			if registry.Records[sessionID] != r || registry.Writes != 1 || registry.Syncs != 0 || len(store.Events) != writes || !bytes.Equal(store.Records[r.Intent], before) {
				t.Fatal("unproven absence changed the retained intent or session")
			}
		})
	}
}
