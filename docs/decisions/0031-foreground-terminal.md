# Foreground terminal attachment

Slice-37 updates arc42's **Building-block view** and **Runtime view — Foreground
run and its failure paths**. T-56's lease now binds both run descriptors. Distinct
stdout owns its saved state and passive hangup observation, but stdin alone
supplies dimensions. Two descriptors for one terminal acquire and restore once.
Linux's terminal-device identity resolves supplied `/dev/tty` aliases before any
raw-state snapshot; filesystem inode identity cannot identify those aliases.

Only `run --tty -- <argv>` selects this attachment. JSON and argv refusals precede
terminal acquisition, which shares startup's deadline. The observer outlives
startup and host cancellation; every return closes the lease. Acquisition of
distinct stdout rolls back stdin on failure. Restoration failures remain separate
from acquisition, workload and cleanup outcomes.

The session engine receives the lease through its streams contract. Its Docker
adapter selects TTY and initial console dimensions before creation, verifies both
before start, copies one raw output stream, and uses the pinned SDK resize API.
Nonterminal output keeps Docker demultiplexing. Each resize reopens protected
session authority and checks the exact owned running worker. Terminal loss or
observation/resize failure ends foreground wait and enters slice-12's existing
bounded stop/drain/removal path. Raw output remains active through the final
bounded drain so trailing LF bytes and shutdown diagnostics are unchanged. The
drain restores the terminal on completion or expiry independently of cleanup;
blocked output cannot retain raw settings past that original bound. Restoration
failures use the same bounded diagnostic path and original teardown deadline as
other failures. An observed nonzero child exit still wins.

This corrects the earlier restoration-before-drain order in the same runtime
view: restoring output processing before the stream finishes changed trailing
bytes even on a successful run. No extra cleanup or diagnostic budget is added.

No policy, approval fingerprint, recovery schema, environment inheritance,
permission, dependency or architecture-policy exception is added. The existing
term/sys and Moby pins supply terminal state, observation and transport primitives.

The coordinator accepted engine red evidence for RUNTTY-06/07/08/09/12/13 at
23270b6465078a0268302b18fcffc75f08a76071 in dispatch 43837837169, job 133198815706.
All fourteen rows failed at launch with exit 2 and `unknown flag --tty`; no step
was undefined. Later steps were not reached. Green native-engine evidence remains
the coordinator's CI responsibility; local HTTP/PTY fixtures make no engine claim.
