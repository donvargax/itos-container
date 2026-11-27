---
status: accepted
date: 2026-11-24
---

# slice-60's cancelled-sweep test pairs its absence check with a live-context positive control

## Context and Problem Statement

slice-60: its kept attempt fails the red check only because TestCancelledSweepDoesNotInspectDirectories (internal/engine/approval/disk/control_sweep_test.go) passes on red. How must that test prove the cancelled sweep?

Asked as q-97, about slice-60.

## Considered Options

The options are those the question names.

## Decision Outcome

The coordinator, 2026-11-24: TestCancelledSweepDoesNotInspectDirectories only asserts absence (with a cancelled context the sweep opens no directory and finds no names), so an implementation that does nothing passes it on red, and the red check is right to refuse it as proof. Keep the test and pair it, in the same test, with a positive control: the same tree swept with a live context opens the root and removes an orphaned .intent-<nonce>.tmp, and only the cancelled sweep opens nothing and removes nothing. Then it fails on red and passes on green. Fix it in the kept attempt; nothing else in the attempt changes for this.

### Consequences

The next implement turn of slice-60 continues its kept attempt and changes TestCancelledSweepDoesNotInspectDirectories so that it fails on red: the same tree swept with a live context opens the root and removes an orphan, and the cancelled sweep opens and removes nothing. A test that only asserts absence is not red evidence; the same pairing applies to any later cancelled or refused-path test in this slice.
