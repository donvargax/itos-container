package features

import (
	"fmt"
	"github.com/cucumber/godog"
)

func (w *world) sessionLocalSteps(sc *godog.ScenarioContext) {
	sc.Step(`^an approved session contract fixture with frozen image inspection "([^"]*)"$`, func(value string) error { return w.sessionLocalFixture("image", value) })
	sc.Step(`^each backend inspects the frozen image config ID before any create$`, func() error {
		return w.sessionEach(func(w *world) error {
			for _, r := range w.session.api.requests {
				if r.Method == "POST" {
					return fmt.Errorf("create preceded frozen image inspection")
				}
				if r.Method == "GET" && r.Path == "/images/"+w.doctor.imageID+"/json" {
					return nil
				}
			}
			return fmt.Errorf("frozen image config ID was never inspected")
		})
	})
	sc.Step(`^a session recovery fixture with "([^"]*)" and stage "([^"]*)"$`, func(storage, stage string) error {
		if err := w.sessionLocalFixture("storage", storage); err != nil {
			return err
		}
		return w.sessionEach(func(w *world) error {
			record := w.sessionExpectedRecord(stage, w.session.seedID)
			if stage == "intent" {
				record["container_id"] = ""
			}
			return writeManifestFixture(w.sessionRecordPath(), approvalJSON(record), 0600)
		})
	})
	sc.Step(`^an approved session contract fixture with child umask "([^"]*)"$`, func(value string) error { return w.sessionLocalFixture("umask", value) })
	sc.Step(`^an approved session contract fixture with post\-create inspection "([^"]*)"$`, func(value string) error { return w.sessionLocalFixture("inspect", value) })
	sc.Step(`^an approved session contract fixture with intent fault "([^"]*)"$`, func(value string) error { return w.sessionLocalFixture("intent", value) })
	sc.Step(`^an approved session contract fixture with token condition "([^"]*)"$`, func(value string) error { return w.sessionLocalFixture("token", value) })
	sc.Step(`^an approved session contract fixture with persistent owned\-record fault "([^"]*)"$`, func(value string) error { return w.sessionLocalFixture("owned", value) })
	sc.Step(`^an approved session contract fixture with create reply "([^"]*)"$`, func(value string) error { return w.sessionLocalFixture("reply", value) })
	sc.Step(`^an approved session contract fixture with a delayed create and lookup "([^"]*)"$`, func(value string) error { return w.sessionLocalFixture("delayed", value) })
	sc.Step(`^an approved session contract fixture with a post\-handoff change "([^"]*)"$`, func(value string) error { return w.sessionLocalFixture("handoff", value) })
	sc.Step(`^a session recovery fixture whose ownership evidence has "([^"]*)"$`, func(value string) error { return w.sessionLocalFixture("evidence", value) })
	sc.Step(`^a session recovery fixture with identity condition "([^"]*)"$`, func(value string) error { return w.sessionLocalFixture("identity", value) })
	sc.Step(`^a session recovery fixture with pre\-removal container inspection "([^"]*)"$`, func(value string) error { return w.sessionLocalFixture("pre-remove", value) })
	sc.Step(`^a session recovery fixture with current input change "([^"]*)"$`, func(value string) error { return w.sessionLocalFixture("changed", value) })
	sc.Step(`^a session recovery fixture with absent\-record fault "([^"]*)"$`, func(value string) error { return w.sessionLocalFixture("absent", value) })
	sc.Step(`^an intent\-only session recovery fixture with absent\-record fault "([^"]*)"$`, func(value string) error { return w.sessionLocalFixture("reconcile-absent", value) })
	sc.Step(`^an intent\-only session recovery fixture with owned\-record fault "([^"]*)"$`, func(value string) error { return w.sessionLocalFixture("reconcile-owned", value) })
	sc.Step(`^each removal follows durable publication of the independently reconciled full ID$`, func() error { return w.sessionEach((*world).sessionReconciledBeforeRemoval) })
	sc.Step(`^each first cleanup reports ownership\-write\-failed and retains the independently observed worker without removal$`, func() error { return w.sessionEach((*world).sessionReconciledWriteFailed) })
	sc.Step(`^an approved session contract fixture with two distinct entropy results$`, func() error { return w.sessionLocalFixture("twice", "") })
	sc.Step(`^an approved session contract fixture with owned\-record fsync failure followed by unavailable cleanup inspection$`, func() error { return w.sessionLocalFixture("compound", "") })
	sc.Step(`^a session recovery fixture with a recorded absent container and a same\-named foreign replacement$`, func() error { return w.sessionLocalFixture("already-absent", "") })
	sc.Step(`^a session recovery fixture with removal result "([^"]*)" and following ID observation "([^"]*)"$`, func(value, other string) error {
		if err := w.sessionLocalFixture("remove", ""); err != nil {
			return err
		}
		return w.sessionEach(func(w *world) error { w.session.removal, w.session.following = value, other; return nil })
	})
	sc.Step(`^the session contract creates one worker and cleans it through each local backend$`, func() error { return w.sessionLocalRun("cycle") })
	sc.Step(`^the session contract completes A's preflight, applies the fixture change, then creates and cleans through each local backend$`, func() error { return w.sessionLocalRun("cycle") })
	sc.Step(`^the session contract attempts creation and bounded cleanup through each local backend$`, func() error { return w.sessionLocalRun("cycle") })
	sc.Step(`^a new session controller attempts cleanup through each local backend$`, func() error { return w.sessionLocalRun("cycle") })
	sc.Step(`^the session contract attempts creation through each local backend$`, func() error { return w.sessionLocalRun("create") })
	sc.Step(`^creation is attempted and a new controller cleans the surviving intent through each local backend$`, func() error { return w.sessionLocalRun("reconstruct") })
	sc.Step(`^creation and cleanup finish with both faults active, then a new controller retries cleanup after both faults are cleared through each local backend$`, func() error { return w.sessionLocalRun("retry") })
	sc.Step(`^creation and cleanup finish with the write fault active, then a new controller retries cleanup after the fault is cleared through each local backend$`, func() error { return w.sessionLocalRun("retry") })
	sc.Step(`^creation and first cleanup finish before the delayed worker appears and a new controller then retries cleanup$`, func() error { return w.sessionLocalRun("retry") })
	sc.Step(`^cleanup is attempted, the fault is cleared and a new controller retries through each local backend$`, func() error { return w.sessionLocalRun("retry") })
	sc.Step(`^two new controllers successively clean the recorded ID through each local backend$`, func() error { return w.sessionLocalRun("repeat") })
	sc.Step(`^two sequential full preflight, create and cleanup cycles reuse the same approved snapshot through each local backend$`, func() error { return w.sessionLocalRun("two") })
	sc.Step(`^each backend returns owned\-unstarted with the independently observed full container ID$`, func() error { return w.sessionEach((*world).sessionOwned) })
	sc.Step(`^each backend returns owned\-unstarted after exactly one independently observed create using A's frozen image identity$`, func() error { return w.sessionEach((*world).sessionOwned) })
	sc.Step(`^each create follows the exact durable intent and precedes the durable owned record$`, func() error { return w.sessionEach((*world).sessionIntentBeforeCreate) })
	sc.Step(`^each captured create request uses A's host UID:GID and the specified minimal unstarted configuration$`, func() error { return w.sessionEach((*world).sessionIntentBeforeCreate) })
	sc.Step(`^each worker is inspected as never started before removal by its full ID$`, func() error { return w.sessionEach((*world).sessionNeverStarted) })
	sc.Step(`^each cleanup completes only after ID absence and durable absent recording$`, func() error { return w.sessionEach((*world).sessionCleanupComplete) })
	sc.Step(`^each cleanup completes only after ID absence and durable absent recording on the later attempt$`, func() error { return w.sessionEach((*world).sessionCleanupComplete) })
	sc.Step(`^each backend returns cleanup\-complete after removing only the original recorded worker and confirming its ID absent$`, func() error { return w.sessionEach((*world).sessionCleanupComplete) })
	sc.Step(`^the intent advances to durable absent with its original snapshot binding and full container ID$`, func() error { return w.sessionEach((*world).sessionCleanupComplete) })
	sc.Step(`^each retry confirms the same ID absent and completes only after successful record durability$`, func() error { return w.sessionEach((*world).sessionCleanupComplete) })
	sc.Step(`^session operations preserve unrelated resources, checkout edits, approval and frozen settings$`, func() error { return w.sessionEach((*world).sessionUnrelated) })
	sc.Step(`^the rejected ownership evidence and all outside targets are unchanged$`, func() error { return w.sessionEach((*world).sessionUnrelated) })
	sc.Step(`^no existing intent, frozen snapshot or outside target was rewritten$`, func() error { return w.sessionEach((*world).sessionUnrelated) })
	sc.Step(`^the original snapshot and changed current inputs remain unchanged by cleanup$`, func() error { return w.sessionEach((*world).sessionUnrelated) })
	sc.Step(`^the same\-named replacement and unrelated state are unchanged$`, func() error { return w.sessionEach((*world).sessionUnrelated) })
	sc.Step(`^the intent, owned and absent records retain exactly A's generation, fingerprint and snapshot binding$`, func() error { return w.sessionEach((*world).sessionSnapshotBindings) })
	sc.Step(`^the original durable intent remains unchanged with an empty container ID and no owned or absent update$`, func() error { return w.sessionEach((*world).sessionIntentRetained) })
	sc.Step(`^the post\-create session journal contains inspection requests only$`, func() error { return w.sessionEach((*world).sessionOnlyInspections) })
	sc.Step(`^no container create or later mutation was attempted$`, func() error { return w.sessionEach((*world).sessionNoMutations) })
	sc.Step(`^no container create or later engine mutation was attempted$`, func() error { return w.sessionEach((*world).sessionNoMutations) })
	sc.Step(`^each operation owns a different random\-named container with a separate durable intent$`, func() error { return w.sessionEach((*world).sessionTwo) })
	sc.Step(`^both intents bind the same unchanged approved snapshot and their own full container IDs$`, func() error { return w.sessionEach((*world).sessionTwo) })
	sc.Step(`^cleanup of either operation never mutates the other's resource or record$`, func() error { return w.sessionEach((*world).sessionTwo) })
	sc.Step(`^both cleanups complete after their own ID absence and durable absent recording$`, func() error { return w.sessionEach((*world).sessionTwo) })
	sc.Step(`^each backend retains ownership\-write\-failed with separate incomplete cleanup after exactly one observed unstarted create$`, func() error { return w.sessionEach((*world).sessionCompound) })
	sc.Step(`^the first attempt leaves the original complete intent unchanged and the created worker independently observable$`, func() error { return w.sessionEach((*world).sessionCompound) })
	sc.Step(`^the first attempt returns the exact surviving intent reference without a successful ownership handoff$`, func() error { return w.sessionEach((*world).sessionSafeReference) })
	sc.Step(`^each cleanup returns the exact surviving intent reference without a successful ownership handoff$`, func() error { return w.sessionEach((*world).sessionSafeReference) })
	sc.Step(`^the first cleanup attempts fresh engine inspection without removal or an absent update$`, func() error { return w.sessionEach((*world).sessionFirstNoRemoval) })
	sc.Step(`^the first cleanup makes no absent update, replacement create or resource removal$`, func() error { return w.sessionEach((*world).sessionFirstNoRemoval) })
	sc.Step(`^the new controller reconciles the exact random name and rechecks its full ID against the durable intent$`, func() error { return w.sessionEach((*world).sessionRecoveryInspection) })
	sc.Step(`^each local backend reports create\-uncertain and retains the exact intent after the first cleanup$`, func() error { return w.sessionEach((*world).sessionFirstUncertain) })
	sc.Step(`^the later cleanup reconciles and removes exactly the originally delayed worker$`, func() error { return w.sessionEach((*world).sessionRemovalVerified) })
	sc.Step(`^only the fully revalidated recorded ID was submitted for removal$`, func() error { return w.sessionEach((*world).sessionRemovalVerified) })
	sc.Step(`^the ownership record and every candidate resource are retained unchanged$`, func() error { return w.sessionEach((*world).sessionRetained) })
	sc.Step(`^all container inspection requests target the recorded full ID without a name lookup$`, func() error { return w.sessionEach((*world).sessionNoNameLookup) })
	sc.Step(`^both cleanups return cleanup\-complete after confirming the recorded ID absent$`, func() error { return w.sessionEach((*world).sessionRepeated) })
	sc.Step(`^the first cleanup durably records absence and the second preserves that record's bytes, inode and mtime$`, func() error { return w.sessionEach((*world).sessionRepeated) })
	sc.Step(`^each first cleanup reports completion\-write\-failed despite independently confirmed engine absence$`, func() error { return w.sessionEach((*world).sessionCompletionFailed) })
	sc.Step(`^the first attempt retains a complete owned or absent record without claiming durable completion$`, func() error { return w.sessionEach((*world).sessionCompletionFailed) })
	sc.Step(`^each backend reports "([^"]*)" without a successful ownership handoff$`, func(value string) error {
		return w.sessionEach(func(w *world) error { return w.sessionOutcome(value) })
	})
	sc.Step(`^each backend reports intent\-write\-failed without a successful ownership handoff$`, func() error {
		return w.sessionEach(func(w *world) error { return w.sessionOutcome("intent-write-failed") })
	})
	sc.Step(`^each backend reports ownership\-refused without a successful ownership handoff$`, func() error {
		return w.sessionEach(func(w *world) error { return w.sessionOutcome("ownership-refused") })
	})
	sc.Step(`^each backend reports "([^"]*)" after one acknowledged create and an attempted inspection of its returned full ID$`, func(value string) error {
		return w.sessionEach(func(w *world) error {
			if err := w.sessionObservedCreate(); err != nil {
				return err
			}
			if err := w.sessionInspected(); err != nil {
				return err
			}
			return w.sessionOutcome(value)
		})
	})
	sc.Step(`^each backend reports ownership\-write\-failed after exactly one observed unstarted create$`, func() error {
		return w.sessionEach(func(w *world) error {
			if err := w.sessionObservedCreate(); err != nil {
				return err
			}
			return w.sessionOutcome("ownership-write-failed")
		})
	})
	sc.Step(`^each backend records create\-uncertain after exactly one successful engine create$`, func() error {
		return w.sessionEach(func(w *world) error {
			if err := w.sessionObservedCreate(); err != nil {
				return err
			}
			return w.sessionOutcome("create-uncertain")
		})
	})
	sc.Step(`^each backend reports cleanup\-incomplete after attempting to inspect the recorded full container ID$`, func() error {
		return w.sessionEach(func(w *world) error {
			if err := w.sessionInspected(); err != nil {
				return err
			}
			return w.sessionOutcome("cleanup-incomplete")
		})
	})
	sc.Step(`^each backend returns cleanup outcome "([^"]*)" after inspecting the recorded ID$`, func(value string) error {
		return w.sessionEach(func(w *world) error {
			if err := w.sessionInspected(); err != nil {
				return err
			}
			return w.sessionOutcome(value)
		})
	})
	sc.Step(`^the ownership record has progress "([^"]*)" and no worker outcome is invented$`, func(value string) error {
		return w.sessionEach(func(w *world) error { return w.sessionProgress(value) })
	})
	sc.Step(`^intent publication leaves "([^"]*)" and never accepts partial or temporary records$`, func(value string) error {
		return w.sessionEach(func(w *world) error { return w.sessionVisibleIntent(value) })
	})
	sc.Step(`^cleanup uses the durable intent and fresh matching inspections before removal by full ID$`, func() error { return w.sessionEach((*world).sessionRemovalVerified) })
	sc.Step(`^the last complete ownership record remains until confirmed absence can be durably recorded$`, func() error { return w.sessionEach((*world).sessionCleanupComplete) })
	sc.Step(`^neither retry creates a container nor deletes a same\-named replacement$`, func() error {
		return w.sessionEach(func(w *world) error {
			for _, r := range w.session.api.requests[w.session.cleanupStart:] {
				if r.Method == "POST" || r.Method == "DELETE" {
					return fmt.Errorf("retry mutated engine")
				}
			}
			return nil
		})
	})
	sc.Step(`^no policy, manifest or current approval is read again after A's durable handoff$`, func() error {
		return w.sessionEach(func(w *world) error {
			if len(w.session.selectedReads) != 0 {
				return fmt.Errorf("startup inputs reread after handoff")
			}
			return nil
		})
	})
}
