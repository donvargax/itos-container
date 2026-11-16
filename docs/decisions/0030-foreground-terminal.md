# Foreground terminal attachment

Slice-37 updates arc42's **Building-block view** and **Runtime view — Foreground
run and its failure paths**. T-56's lease now binds both run descriptors. Distinct
stdout owns its saved state and passive hangup observation, but stdin alone
supplies dimensions. Two descriptors for one terminal acquire and restore once.

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
bounded stop/drain/removal path. Restoration precedes that cleanup so blocked
output cannot retain raw host settings. An observed nonzero child exit still wins.

No policy, approval fingerprint, recovery schema, environment inheritance,
permission, dependency or architecture-policy exception is added. The existing
term/sys and Moby pins supply terminal state, observation and transport primitives.

The coordinator accepted engine red evidence for RUNTTY-06/07/08/09/12/13 at
23270b6465078a0268302b18fcffc75f08a76071 in dispatch 43837837169, job 133198815706.
All fourteen rows failed at launch with exit 2 and `unknown flag --tty`; no step
was undefined. Later steps were not reached. Green native-engine evidence remains
the coordinator's CI responsibility; local HTTP/PTY fixtures make no engine claim.
