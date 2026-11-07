# Reserve teardown reporting time and retain the original cause

This refines arc42's “Bounded operations” and decisions 0013/0014. Registered
foreground teardown previously let cleanup consume the complete 30-second total,
then dispatched diagnostics against an already expired deadline. The command
could return before writing even to writable stderr; main immediately exits.
LIFECYCLE-04's harness wait after command return masked the missing output.

Registered run now ends cleanup at 29 seconds and reserves the final second of
the original total for reporting. The first-signal grace still ends at ten
seconds. Calls remain capped at ten seconds or the remaining cleanup time, and
public status/stop keep their separate ten/thirty-second totals. Blocked stderr
cannot extend run beyond its original thirty seconds; caller handles stay open.
This uses the existing standard-library contexts and monotonic deadlines with
no dependency or configurable timeout change.

The run result carries teardown exhaustion separately from its original failure.
Both are reported, preserving engine loss and other wait failures, exact recovery
instructions, cancellation codes and authoritative nonzero workload exits. Cleanup
timeout cannot turn an unknown workload result or unconfirmed absence into success.

Step checkpoints a020fe5 and d606209 add LIFECYCLE-18/19. With their pending tags
removed and production unchanged, eight rows failed at their Then assertions:
the four writable/delayed diagnostic rows lacked output at command return; the
four loss rows erased engine/wait failure during termination or removal. The two
blocked-stderr rows passed through the existing slice-12 controls, accepted under
the standing regression rule. The repaired delayed-writer fixture finishes its
abandoned writer only after freezing the observation; it supplies no late bytes.
All ten rows pass with the fix through port fakes and protected record adapters.
LIFECYCLE-04 now also captures diagnostics before its post-return disposal wait.

The coordinator owns fresh native-engine checks and changed-code proof at the
pushed head. Existing native evidence and the q-11 ordering remain unchanged.
