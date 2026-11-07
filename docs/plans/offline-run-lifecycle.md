# Offline owned-run lifecycle contract

T-11 records the lifecycle required by the [charter](../charter.md), its
[milestone plan](../../PLAN.md) and the binding [pilot contract](container-pilot.md).
Read it with the [architecture and Slices rules](../architecture.md),
[CLI contract](../CLI.md) and [configuration boundary](../CONFIG.md). Those authorities
take precedence. This document supplies planning cases for later behaviour specs;
it implements no commands, scenario steps, configuration or engine client.

At T-11's writing, container commands and isolation were **not implemented**. The
count, version and completion entry point supplies no runtime proof. Linux amd64
with native Docker is the first target, contingent on passing runtime evidence,
not a supported-host claim.
The scope is one approved foreground worker, no network, credentials or ingress, and
the checkout as its only host filesystem mount. Proxy/egress work, accounts, network
Git, LAN publishing, Compose, other hosts, remote engines and linked worktrees remain
outside this milestone, including in recovery examples.

Requirements below come from the pilot as limited by the charter. **Interface choice**
marks an ordinary presentation or lifecycle convention recorded here. **Blocked**
marks a security detail the person must settle; the named questions are collected in
[Security decisions](#security-decisions) and repeated at their points of impact.
Completing T-11 neither approves those questions nor makes runtime implementation
ready to start. Existing sibling items are dependencies, not evidence of consent.

For controller loss, the later settled contract in
[features/lifecycle.feature](../../features/lifecycle.feature) supersedes the original
planning blockers below. Under A3, a surviving worker stays isolated until explicit
stop; no watchdog, liveness lease or automatic kill/restart is authorized. The person's
q-11 assigns the existing ID-LIFECYCLE-15 to slice-15 after slice-12 and slice-14,
preserving one copy and its comments. It is now live: q-32 accepts all eight steps
passing in native CI run 42390615242 at
`e22aa6366a7bf024322d8b59f2b9f112c4cb26dd` as regression coverage of slice-12's
behavior. This is passing evidence, not assertion-level red. See the
[evidence and activation checks](../../features/README.md#the-ci-engine-tier);
ordinary CI at the actual pushed head, including required proof, still gates closure.

Later commands must each own a vertical slice under the current Slices rules: thin
handlers delegate to domains, domains reach the world through ports and use fakes in
tests, and adapters perform I/O. Behaviour shared by commands belongs in an engine
package, never in another slice's domain. Do not grow count into run. T-14 owns the
layout migration; this document fixes no competing package paths. Policy, manifest,
readiness and approval contracts belong to slice-1 through slice-4 respectively; reuse
their contracts rather than inventing loaders, identity formats or state schemas here.
Engine library selection belongs to slice-3. This documentation needs no dependency.

## Arguments and JSON

The public invocation is `itos container run -- <argv...>`. Core itos dispatches to
`itos-container`, passing extension arguments unread and returning its exit status.
Flags before `container` belong to core itos; wrapper options in the extension stop at
run's separator. Require an executable argument after `--`, and require that first
argument to be nonempty. No argv, or an empty executable, is usage failure 2 before
resource creation. Its diagnostic must say that run requires a nonempty command after
`--` and explain how to supply it. An unknown-command diagnostic is not this refusal.

Preserve each subsequent argument exactly, including empty strings, spaces, tabs,
newlines, Unicode, literal quotes, glob characters, dollar signs, semicolons,
redirection-looking text, `--help`, `--json` and another `--`. Never join argv into a
shell string, evaluate it on the host, infer a shell or inject agent permission-skipping
flags. A user explicitly naming a shell makes that shell the worker command, under the
same approved boundary. Any shell interpretation then occurs inside that worker.

**Interface choice:** run is a streaming foreground interface even with redirected
streams. A wrapper `--json`, or inherited `ITOS_JSON=1`, is refused before resources or
workload output. Redirection never enables batch JSON execution. Emit exactly one
stdout object of this shape and exit 2:

```json
{
  "schema": 1,
  "ok": false,
  "problems": [
    {
      "rule": "run.json-unsupported",
      "exit": 2,
      "message": "run does not support wrapper JSON mode",
      "fix": "Remove the wrapper --json option and the inherited ITOS_JSON=1 request."
    }
  ]
}
```

The rule and structural fields are the interface; message and fix are human guidance.
Diagnostic prose goes to stderr, prefixed `itos-container:`. This is a refusal object,
not a JSON wrapper around workload output. An argument `--json` after the separator
belongs to the workload and causes no wrapper refusal by itself. A workload argument
cannot cancel an independent wrapper JSON request.

`ITOS_ROOT`, `ITOS_CONFIG`, `ITOS_JSON`, `ITOS_BIN` and `ITOS_VERSION` are host dispatch
inputs to the controller as described in CLI.md. `ITOS_CONFIG` is core configuration,
not the sandbox policy. Neither dispatch's inherited host environment nor its effective
root establishes permission to forward environment or credentials into the worker or
to trust a checkout identity. This spec introduces no new flag, product environment
variable or TTY mode. **Blocked — Approval and identity; Manifest and writable state:**
safe root resolution and permitted worker environment follow those security contracts.

## Ownership and startup

Startup is ordered. No preflight failure may fall back to an unrestricted session.

1. Validate the invocation, including the JSON refusal, before allocating resources.
   Inspect strict policy and the restricted single-worker manifest without executing
   checkout code, host hooks, repository-configured helpers or build scripts.
2. Validate the selected local engine and checkout identity, the cached digest-pinned
   image and its architecture/compatibility/metadata, and current host-owned approval
   of the effective settings. Missing, invalid, mismatched or superseded approval
   refuses startup. All these checks finish before any session resource is created.
   The person prepares images through the separately specified acquisition process;
   run never pulls, builds, substitutes another image or invokes a fallback runtime.
3. Freeze the validated effective configuration in trusted session material outside
   worker control. Startup and setup use that snapshot, not a re-read of checkout
   files. Policy/manifest edits and a later approval cannot widen an existing session,
   including an attempted in-container restart. New permissions require a new approved
   run after stopping the old one. Slice-4 owns approval and snapshot storage semantics.
4. Create only the needed resources, without executing any worker process. Durably
   record concrete engine resource IDs and ownership in host-owned state outside the
   writable checkout before **any** image entrypoint, setup hook or foreground command
   can execute. Failure to persist ownership prevents execution and enters bounded
   cleanup or explicit recovery; it never permits an unrecorded run.
5. Verify every required boundary before declaring ready. An unverifiable boundary is
   a startup failure requiring cleanup/recovery, not permission to try the workload.
   Only then may the supported setup hooks, if selected by the manifest spec, run under
   the approved identity and frozen settings. A failed hook prevents subsequent setup
   and the foreground command and begins owned cleanup.
6. Launch and attach to the explicit foreground argv only after successful setup.
   Its exit ends the session and starts cleanup. Do not keep the worker alive for a
   later command or detach it from the foreground lifecycle.

**Blocked — Approval and identity; Manifest and writable state; Image readiness:**
slice-1 through slice-4 must settle strict resolution, safe checkout/engine identity,
approval matching and supersession, hook and metadata semantics, identity/storage
mapping and trusted image readiness. An item being specified does not settle these.
No approval fingerprint, discovery path, schema or acquisition command is chosen here.

**Blocked — Crash-safe ownership:** the engine may create a resource and lose its
response, or the controller may die between creation and recording the returned ID.
The reconciliation design must account for this uncertain create before implementation.
A requested resource name or matching label alone cannot prove ownership or authorize
deletion. No process may start while its ownership is uncertain. A failed durable
write must retain or recover enough trusted evidence for safe cleanup; this document
does not pretend an in-memory ID survives a crash or choose a journal protocol.

The offline boundary applies from the first worker instruction, including image
entrypoints and supported setup. It requires:

- No external network path or published port; denial does not depend on cooperative
  clients, proxy variables or the absence of networking tools.
- No host namespaces, administrative capabilities, sudo, engine sockets, SSH-agent
  sockets, host environment forwarding or credential forwarding.
- `no-new-privileges`, a read-only image root and the approved non-root UID/GID.
- The checkout as the only host filesystem mount, with explicit writable checkout,
  isolated persistent project state and container-owned temporary storage. No host
  temporary directory, host home or extra host path; no host-wide chown as a workaround.
- No unreviewed command, user or permission supplied by hidden image metadata.

**Blocked — Offline boundary:** the mechanism and verification before the first process
need a security decision and runtime evidence. Engine option strings alone are not
proof. **Blocked — Manifest and writable state; Image readiness:** writable locations,
user mapping, setup ordering/repetition and image treatment cannot be inferred from
these requirements. Do not add a guard/proxy or session network merely because the
online pilot later needs them.

**Interface choice:** interpret the pilot's state names as follows. State and workload
outcome are distinct; `stopped` does not mean the workload exited zero.

| State | Meaning and allowed progress |
| --- | --- |
| preparing | Preflight, snapshot preparation and resource creation/verification; the foreground command has not launched. A refusal before creation has no session resources; partial creation remains accountable. Success reaches ready; failure or cancellation enters stopping if owned resources need cleanup, otherwise records the failure. |
| ready | Required boundaries are verified and ownership is durable. Supported setup may run within that boundary. Only successful setup permits launch and transition to running; setup failure/cancellation goes to stopping. |
| running | The foreground command has launched under frozen settings. Observed exit or cancellation enters stopping; controller/engine loss cannot be treated as observed termination. |
| stopping | Bounded owned cleanup is in progress, including after partial startup or an already stopped worker. Completion reaches stopped; incomplete cleanup reaches failed with recovery information. |
| stopped | Required ephemeral cleanup is confirmed, including that absent recorded resources need no further removal. Preserve the workload result and edits/project state. No restart is authorized. |
| failed | Startup, setup or controller work failed, or required cleanup remains incomplete. Report which and whether resources remain uncertain/present; the state alone never claims removal. A later authorized stop may resume stopping using trusted ownership evidence. |

The safe persistence of these observations, uncertain transitions and concurrent
updates is **blocked — Crash-safe ownership; Safe recovery**. A state name alone never
authorizes a destructive operation.

## Timeouts and cancellation

Every controller wait must be bounded. Finite startup and teardown budgets cover
individual calls and all retries; neither retries nor progress may reset the total
budget. A startup timeout still requires a separately bounded cleanup attempt.
Recovery attempts are bounded too; a later explicit retry does not extend an earlier
attempt indefinitely. Intentional foreground waiting while a workload runs is separate:
this contract invents no maximum workload duration.

| Controller wait | Required bound or transition |
| --- | --- |
| Preflight, engine inspection and state operations | Bound each operation and the overall preflight portion of startup; a stalled engine/state port cannot block indefinitely. No resource creation after failed preflight. |
| Resource creation | Bound calls and retries within startup; reconcile an in-flight or uncertain create on timeout/cancellation. Never retry blindly into an untracked duplicate. |
| Startup and readiness verification | Bound verification and retries within startup. Failure to verify cannot become ready. |
| Supported setup hooks | Include setup execution and waits in the startup budget. Failure, timeout or cancellation prevents later setup and workload launch. |
| Attach establishment | Bound the establishment operation within startup. A launch followed by failed attach requires outcome reconciliation and cleanup, not an assumed workload exit. |
| Foreground streaming and termination observation | No invented workload lifetime limit. Individual controller/engine requests remain bounded and cancellation must remain observable; a stalled request is not proof the workload is still healthy or terminated. |
| Signal delivery and stop | Bound delivery, acknowledgements and termination waits within teardown. Delivery is not evidence of termination. |
| Removal | Bound each removal, retry and absence confirmation within teardown. Failure retains recovery data. |
| Recovery/status inspection and repeated stop | Bound state/engine reconciliation and cleanup calls within the recovery attempt. Engine unavailability must yield an actionable incomplete result, not a hang. |

**Blocked — Resource and wait budgets:** the person must choose resource limits,
numeric budgets and their enforcement, including stalled calls, setup and backpressure.
**Blocked — Cancellation and controller loss:** signal targets, forwarding/escalation,
grace periods and platform outcomes are not specified by naming these waits. No timeout
value, signal number or timeout exit code is approved here.

Once cancellation is observed, never start subsequent setup/workload phases. Account
for operations already in flight; cancellation cannot erase a create request or prove
that a process did not launch. Required observations are:

| Cancellation point or event | Required observation |
| --- | --- |
| Before resource creation | No later create or worker launch; report cancellation under the eventual platform mapping. |
| During partial startup, verification, setup or attach | No later phase starts; reconcile any accepted create/launch and attempt bounded cleanup of confirmed owned resources. Retain uncertain outcomes for recovery. |
| SIGINT or SIGTERM after launch | Observe the settled signal policy at the worker, its actual outcome if available, and bounded owned cleanup. Do not report success merely because a delivery call succeeded. |
| Workload handles a signal | Observe its handler and resulting termination or continued execution; use the settled grace/escalation policy, never infer termination from handler output. |
| Workload ignores a signal | Apply the settled bounded stop/escalation policy; confirm termination/removal or retain recovery information. No unbounded wait or invented child status. |
| Repeated cancellation, including during cleanup | Do not restart setup, duplicate unsafe cleanup or discard ownership evidence. Complete or interrupt cleanup according to the settled policy and preserve a retry if incomplete. |
| Uncatchable controller loss | No cleanup handler can run in a dead controller. Under settled A3, a surviving worker stays isolated until explicit stop. Status identifies the owned stop candidate; stop revalidates ownership and confirms removal. Never claim a workload exit from controller death or cleanup success. |

Normal cancellation attempts owned cleanup. Unavailable or interrupted cleanup retains
recovery information. Controller death does not trigger automatic worker termination:
the settled A3 policy requires retained isolation and permits explicit owned stop,
without a watchdog, lease or restart. ID-LIFECYCLE-15 exercises the merged recovery
and termination contract with slice-14's independent boundary probes and controls.
Actual signal, interruption and retained-isolation observations require CI on the
target engine. Local fake tests prove controller decisions only.

## Workload exits

Separate a wrapper refusal, an observed workload termination and an unavailable
workload outcome. An engine disconnect or lost controller can leave the last outcome
unknown; do not manufacture a child status from it. Successful signal delivery alone
does not establish termination. If the controller is dead, it cannot emit a run result;
a later recovery interface reports the uncertainty.

Use the wrapper meanings in CLI.md, not itos-cc's different codes:

| Code | Wrapper meaning |
| --- | --- |
| 0 | Success. |
| 1 | Policy or check refusal. |
| 2 | Usage or configuration error, including unknown command. |
| 3 | Missing environment requirement. |
| 70 | Unclassified internal error. |
| 75 | Temporary failure that may succeed when repeated unchanged. |

The missing-command and JSON refusals above are 2. For cleanup failures, classify the
actual cause: refusal to mutate an ambiguous/foreign identity is 1; invalid recovery
configuration is 2; a missing required engine environment is 3; transient engine
unavailability is 75; an unclassified controller defect is 70. Never import a helper's
exit status or call every cleanup failure transient. Report cleanup problems separately
from the workload result, with the owned recovery reference whenever cleanup is pending.

| Situation | Result precedence | Cleanup and reporting |
| --- | --- | --- |
| Wrapper refusal before launch | Return the wrapper code for the specific cause; missing argv/JSON is 2. No workload status exists. | Explain the refusal; no resources for preflight refusals. Partial startup failures require cleanup below. |
| Launch fails with confirmed absence of a launched workload | Return the classified launch failure, never a fabricated child exit. | Clean partial resources; report additional cleanup failures and recovery separately. |
| Workload exits zero; required cleanup succeeds | Return 0. | Report completion only after cleanup is confirmed. |
| Workload exits nonzero; required cleanup succeeds | Preserve that observed nonzero workload exit status. | Cleanup completion does not erase the workload failure. |
| Workload exits nonzero; required cleanup fails | Preserve that observed nonzero workload exit status even if it resembles a wrapper code. | Emit a separate cleanup diagnostic with session ID and retry; retain recovery. |
| Workload exits zero; required cleanup fails | Return nonzero, classified by the cleanup cause within the wrapper meanings above. | Distinguish successful workload from failed cleanup and retain recovery. |
| Startup/setup/attach timeout | No synthetic workload status. Exact timeout outcome is blocked on Resource and wait budgets and Cancellation and controller loss. | Reconcile in-flight launch/create; perform bounded cleanup and retain recovery if incomplete. An observed nonzero workload exit still must be preserved. |
| Cancellation before or after launch | Preserve an observed nonzero workload exit; other signal-derived or cancellation mappings are blocked on Cancellation and controller loss. | Never equate delivery with termination; report observed outcome and cleanup independently. No implicit interrupt code is selected. |
| Workload result unavailable after controller/engine loss | Never claim workload success or an invented exit. A surviving controller reports a nonzero failure classified by its observed cause; cancellation/loss-specific mappings remain blocked. A dead controller cannot return a result. | Retain an unknown outcome and recovery evidence; later status/stop must not rewrite unknown as successful execution. |

Return zero only when both workload success and required cleanup success are known.
This is necessary, not sufficient to choose a cancellation mapping: cancellation with
an observed zero result still awaits the person's decision. Signal-derived workload
status representation and competing controller causes must be specified in later
scenarios under **Cancellation and controller loss** and **Resource and wait budgets**.
Codes, JSON, typed problem IDs and presentation stay outside the domain; domains return
outcomes through the command boundary, not numeric UI policy.

## Cleanup and recovery

Cleanup is complete only after the session's owned ephemeral resources are removed
or confirmed absent: the worker even if partially created or already stopped, and
session temporary storage. Preserve checkout edits and persistent isolated project
state. The offline slice must not create proxy/guard or session-network resources to
imitate the online pilot's cleanup list. Never prune foreign resources, delete profiles,
remove a shared image, discard state/evidence or perform host-wide chown as a shortcut.

Authorize each destructive operation with trusted ownership and concrete resource
identity. Labels and names are corroborating information only; ambiguous, mismatched,
wrong-user or foreign identities are refused, not adopted. Repeated cleanup is safe
when a recorded resource is already absent and must leave other projects/sessions
untouched. An unreachable engine cannot confirm absence. Recovery must retain enough
trusted ownership information to finish cleanup after controller loss, including the
uncertain-create case, without trusting checkout-controlled data.

Keep recovery information until required cleanup is confirmed. On incomplete cleanup,
report the session ID and the exact retry command with that ID substituted. For a
synthetic session named `session-example`, the form is
`itos container stop session-example`, not an unresolved `<session>` placeholder or a
prune command. The valid identifier grammar and safe rendering still depend on
**Safe recovery**; this example neither chooses an ID format nor licenses injection.

`status` and `stop` need their own specifications before a later run implementation can
promise an actionable retry. This task adds neither command. Status inspects owned
session state, effective policy identity and recovery information. Stop cancels a live
owned session or completes its owned cleanup. Neither grants an unattended workload
restart. There is no detached/keep-alive mode. Primary foreground exit ends the session;
preserve the pilot's future exec invariant by refusing new commands and terminating
active ones at that exit, without pulling an exec implementation into this milestone.

Controller diagnostics and recovery metadata must not dump workload argv, inherited
host environment, workload output or synthetic sensitive markers. Retain safe session
identity, ownership, lifecycle/cleanup progress and typed failure information sufficient
for recovery; exact fields and storage are **blocked — Safe recovery**. Approved frozen
configuration is separate trusted session material governed by slice-4, and must include
the effective settings the pilot requires. This logging restriction does not prohibit
that snapshot. Deliberate foreground workload streaming is also separate from controller
logging; there is no promise to redact arbitrary workload text or copy it into recovery.

**Blocked — Safe recovery; Crash-safe ownership:** settle protection of records,
identity checks at mutation time, concurrent run/stop/status, uncertain creates and
safe retries. Do not choose locking or reconciliation rules by example. For the settled
controller-loss case, consume slice-11's recovery and shared termination contract
through merged slice-12; retained-isolation evidence uses slice-14's probes and
controls. Forced controller death cannot execute cleanup. The same surviving worker
must retain its boundary until explicit stop, with fresh post-loss observations and
confirmed owned removal as ID-LIFECYCLE-15 requires.

## Acceptance scenarios

These are future black-box planning cases, not scenario IDs, steps or passing tests.
The pilot's AC labels are traceability labels only. Each setup prepares synthetic
inputs, fixtures, barriers or injected failures; it never invokes run/stop and asserts
the subject behaviour as a Given. When a case needs a live/interrupted session, its
Action performs that lifecycle. A harness can prepare a trusted recorded-resource
fixture directly for recovery cases, subject to the settled state format.

At T-11's writing the binary had no run command. An unknown-command failure,
undefined step or failed setup is not red behaviour evidence. Refusal assertions must
identify the specific problem/diagnostic **and** absence of side effects; absence alone
passes on that baseline binary. Expected-failure cells below name the behaviour
assertion that a broken implementation must fail, not a setup failure to count as evidence.

Use these evidence tiers:

- **Local CLI:** pure argument/JSON refusals against the public executable, without
  engine access or a Docker socket. Observe output, status and fixture files. Companion
  domain port-fake checks establish zero resource calls; an actual engine-resource
  audit is a separate CI test.
- **Local domain:** sequencing/failure tests against the domain's engine and state-port
  fakes. These supplement public cases with call/state observations; they prove neither
  OS isolation nor actual resource/signal behaviour. Do not add a product fake-engine
  flag or environment variable to make these tests possible.
- **CI runtime:** all actual isolation, engine-resource, signal and interruption probes,
  only on Linux amd64/native Docker with disposable inputs and independently observed
  resources. Engine/state fakes are not substitutes for this evidence.

The table's **blocked** question names refer to Security decisions. All CI runtime
cases also depend on Image readiness, Resource and wait budgets, and the common
Approval and identity, Manifest and writable state, Offline boundary, Crash-safe
ownership and Safe recovery foundations. Additional case-specific blockers are named
in their rows. A positive runtime control must itself use reviewed trusted test
material, outside the tested worker; it never relaxes a subject run's permissions.

Synthetic fixtures are a small argv recorder that emits its received argument array,
a marker writer, fixed zero/nonzero exit programs (for example an observed exit 17),
signal-handling/ignoring programs and first-instruction boundary probes in a prepared
digest-pinned test image. Fresh disposable checkouts and state hold only harmless
markers. Trusted engine fault controls delay, lose or fail operations without executing
checkout code on the host. These fixtures need later specs; their mention is no image
or library selection. Public argv is supplied as an argument array by the harness so
the harness's own shell cannot consume the literal test values.
Network fixtures represent external, gateway, private and metadata-address routes
using controlled disposable listeners, never real metadata services or credentials.
For filesystem denial probes, independently verify the host sentinel exists and is
readable by the trusted fixture, and pair denied operations with successful allowed
reads/writes by the same probe. Missing tools or failed controls invalidate evidence;
they cannot count as enforcement.

| Case | Setup | Action | Observable result | Evidence tier | Expected failing assertion |
| --- | --- | --- | --- | --- | --- |
| Literal argv (AC-21) | Approved disposable checkout; argv recorder image; arguments including empty string, space/tab/newline, `café`, literal quotes, `*`, `$HOME`, `$(touch host-marker)`, `;`, `> host-marker`, `--help`, `--json`, `--`. | Invoke `itos container run --` with recorder plus the prepared array. | Recorded array equals every input element; host-marker is absent; no inserted permission flags; normal cleanup completes. | CI runtime; local domain supplements argv handoff. | Exact array equality and host-marker absence fail if arguments are split, expanded, joined or interpreted; unknown-command cannot emit the array. |
| Explicit shell stays inside (AC-21, AC-07) | Approved image with a shell; writable checkout and an unmounted host sentinel; a script writes a checkout marker and attempts the host sentinel. | Invoke run with explicitly supplied shell and script arguments. | Script executes only as worker, writes the checkout marker and cannot access host sentinel; identity/boundary probes pass. | CI runtime; blocked on Manifest and writable state. | Checkout marker plus host denial and non-root identity fail if the shell is inferred elsewhere, run on the host or never launched. |
| Missing or empty executable (AC-21) | Empty disposable checkout, no engine access; two argv variants, separator alone and separator followed by empty executable. | Invoke each run variant. | Exit 2; diagnostic requires a nonempty command after `--`; no resource attempt or workload output. | Local CLI; local domain fake checks zero resource calls. | Required-command diagnostic and companion zero-call assertion fail for unknown-command or permissive empty argv handling. |
| Wrapper and inherited JSON (AC-21, AC-22) | Marker workload argv; separately wrapper `--json`, core `--json` before container, and controller-inherited `ITOS_JSON=1`; terminal and redirected-stream fixtures. | Invoke each run variant, including a workload `--json` variant; exercise inherited environment at the extension entry point, since core dispatch sets its own protocol variables. | Exactly one schema-1 false object, rule `run.json-unsupported`, problem exit and process exit 2, message/fix; no workload output or resource calls in every variant. | Local CLI; local domain fake checks zero resource calls. | Exact refusal object, absent workload marker/output and companion zero-call assertion fail for unknown-command, mixed output or a redirected batch exception. |
| Workload JSON argument (AC-21) | Approved recorder image, no wrapper JSON request. | Invoke `itos container run --` with recorder and `--json`. | Recorder sees that literal argument; no `run.json-unsupported` object; cleanup succeeds. | CI runtime; local domain supplements argument classification. | Recorded `--json` and successful workload result fail if wrapper parsing crosses the separator. |
| Strict inputs refuse (AC-01, AC-40) | Separate missing/malformed policy, duplicate/unknown fields, missing manifest, and unsupported build/Compose/host-hook/extra-mount/runtime-flag/host-interpolation requests; harmless host execution markers. | Invoke run for each input. | Specific key/path or missing-file diagnostic, appropriate wrapper refusal and no resource creation, host marker or fallback execution. | CI runtime for side effects; local domain fakes for sequencing. Blocked on Approval and identity; Manifest and writable state. | Each specific refusal plus no side effects fails if validation is bypassed or an unknown-command failure substitutes for it. |
| Approval absent, invalid, mismatched or superseded (AC-02) | Separate trusted approval fixtures: absent, malformed, wrong effective image/identity/settings, and superseded; valid current settings otherwise. | Invoke run with each fixture. | Specific approval reason, policy/configuration classification as specified by slice-4, no creation or execution and no self-approval. | CI runtime; local domain state/engine fakes. Blocked on Approval and identity. | Reason-specific refusal and no creation fail if a valid file is mistaken for consent or old approval is reused. |
| Frozen policy and manifest (AC-03, AC-41) | Approved marker setup/workload with barriers; separate edits requesting forbidden network/mount access and a valid in-scope change to approved hook/identity settings. | Start run; edit policy/manifest at barriers and attempt forbidden access; record later approval only for the valid in-scope change, then release setup; after exit try a new run with stale approval. | Live setup/workload retain original settings and denied access; forbidden requests gain no approval; the later run refuses stale approval with its specific diagnostic. | CI runtime; local domain snapshot test. Blocked on Approval and identity; Manifest and writable state. | Original hook/settings and denied wider access fail if startup re-reads mutable requests; stale-approval diagnostic fails if supersession is ignored. |
| Unsupported engine or checkout (AC-05) | Remote-engine selection, linked-worktree fixture and checkout path/symlink substitution variants; unmounted harmless host targets. | Invoke run, applying a prepared substitution at the identity-check barrier for that variant. | Specific unsupported/changed-identity diagnostic and refusal before worker execution, without reaching a remote engine or mounting the substituted target. | CI runtime; local domain identity fakes. Blocked on Approval and identity. | Specific refusal plus no substituted mount/worker fails if basename/path text is trusted instead of settled identity. |
| Cached image readiness (AC-38) | Separate missing cached digest, wrong digest, incompatible architecture and unapproved replacement image fixtures; acquisition-call recorder. | Invoke run for each image. | Specific image readiness/approval diagnostic; no pull, build, image substitution or worker launch. | CI runtime; local domain engine fake. Blocked on Image readiness. | Readiness diagnostic and zero acquisition/launch observations fail if fallback acquisition or an unapproved image is used. |
| Hidden image metadata (AC-40, AC-41) | Prepared image with unreviewed command/user/mount metadata; visible manifest otherwise approved. | Invoke run. | Specific unsupported or approval-mismatch diagnostic; no metadata command marker or widened permission. | CI runtime; local domain resolution fake. Blocked on Manifest and writable state; Image readiness. | Diagnostic and absence of hidden-command execution fail if metadata is silently merged or ignored contrary to the settled treatment. |
| Ownership write fails (AC-23) | Approved entrypoint marker image; trusted state-port fault rejects durable resource recording after create. | Invoke run and observe engine/state independently. | No entrypoint/setup/workload marker; persistence diagnostic; bounded owned cleanup or explicit recovery, never an unrecorded run. | CI runtime; local domain engine/state fakes. Blocked on Crash-safe ownership. | Zero worker instructions and accountable created resources fail if launch precedes the durable write. |
| Create response lost or controller interrupted (AC-23) | Trusted barrier after engine accepts create, before response or durable recording; entrypoint marker image. | Invoke run; lose the response or kill controller at the barrier; later invoke status and stop. | No worker instruction; uncertain create is safely reconciled or explicitly refused with recoverable evidence; no label-only deletion or foreign adoption. | CI runtime only for interruption/resources; local domain uncertain-create fake. Blocked on Crash-safe ownership; Safe recovery; Cancellation and controller loss. | No entrypoint marker and accountable reconciliation fail if create loss permits launch, loses ownership or triggers name-based removal. |
| Non-root writable boundary (AC-07, AC-25) | Separate approved UID/GID 1000/1000 and 2001/3001 fixtures with fresh project state, checkout and temporary storage; unmounted host sentinel and read-only image marker. | Run a probe reporting IDs, reading the image marker, writing allowed locations and attempting root write, host sentinel/home/socket access, host namespaces, sudo and capability elevation. | Exact approved IDs; checkout/state/temp writes work; image read works but write is denied; host/privilege probes deny; only checkout is a host mount, root is read-only, no-new-privileges holds, temp is container-owned; host ownership unchanged. | CI runtime; blocked on Manifest and writable state; Offline boundary. | Successful allowed reads/writes paired with denied operations and independent mount/identity evidence fail for root, extra mounts, broken probes or host-wide chown. |
| First-instruction IPv4/IPv6 and gateway denial (AC-10, AC-37) | Controlled IPv4/IPv6 TCP/UDP listeners and a host-gateway listener with trusted reachability controls; approved first-instruction probe image ignoring proxy variables. | Run trusted positive controls, then run probes as earliest entrypoint/setup instructions and during foreground execution. | Controls reach listeners; worker never does via direct address, gateway, private/metadata route or other host route; no published port; denied-operation reports distinguish unavailable controls from enforcement. | CI runtime; blocked on Offline boundary; Manifest and writable state. | Positive-control success plus worker denial/no listener contact fails for a direct route or a broken network fixture; an engine flag alone cannot satisfy it. |
| DNS denial from first instruction (AC-11, AC-37) | Controlled resolver and DoH endpoint, trusted working client controls, probes for normal resolution, Docker `127.0.0.11`, external UDP/TCP DNS, alternate loopback and DoH. | Exercise controls, then run earliest-instruction and foreground worker probes to each path. | Controls work; worker has no unauthorized resolution or endpoint contact; reports distinguish enforcement from missing clients or failed fixture DNS. | CI runtime; blocked on Offline boundary. | Every path's denial paired with working controls fails if a DNS bypass exists or the probe never ran. |
| Setup failure blocks later work (AC-42 offline) | Approved hooks, where supported: first emits a marker then fails, later hook/workload would emit distinct markers; first also attempts controlled denied egress. | Invoke run. | First hook executes inside verified boundary; egress denied with working external control; later markers absent, setup diagnostic specific, owned cleanup performed. | CI runtime; local domain phase fake. Blocked on Manifest and writable state; Offline boundary. | First marker plus absent later markers, denial and cleanup fail if setup precedes isolation or failure is ignored. |
| Bounded startup, calls and retries (AC-22, AC-23) | Trusted faults separately stall preflight, create, readiness, setup and attach; retryable responses; person-set total budgets; marker workload. | Invoke run for each fault and retry sequence, measuring elapsed time and independent resources. | Specific timeout/stall diagnostic within settled startup plus cleanup budgets; retries do not reset totals; no later workload after cancellation/timeout; uncertain operations recoverable. | CI runtime; local domain engine/state fakes with controlled time. Blocked on Resource and wait budgets; Cancellation and controller loss. | Elapsed bound, phase markers and retry accounting fail if any wait or retry loop can extend startup indefinitely. |
| Cancellation before creation (AC-22, AC-23) | Trusted pre-create barrier and marker workload; separate SIGINT and SIGTERM actions. | Invoke run, deliver signal at barrier and release it. | Cancellation observation under settled mapping; no create/worker; no subsequent phase. | CI runtime; local domain cancellation fake. Blocked on Cancellation and controller loss. | Cancellation-specific outcome and zero later calls/resources fail if startup continues or an unknown-command exit masks it. |
| Cancellation in partial startup (AC-22, AC-23) | Barriers separately at create, verification, setup and attach; durable ownership faults controlled; phase markers. | Invoke run and deliver SIGINT and SIGTERM variants at each barrier. | Already accepted operations reconciled, later phases absent, owned cleanup attempted within budget; unresolved resources have safe recovery. | CI runtime; local domain phase fakes. Blocked on Cancellation and controller loss; Crash-safe ownership. | Phase ordering and reconciled resources fail if cancellation forgets an in-flight create/launch or starts the next phase. |
| Handler, ignoring workload and repeated signals (AC-22, AC-23) | Separate handler workload that records received signals/settled result and ignoring workload; cleanup barrier. | Run each, deliver SIGINT and SIGTERM variants after launch, repeat cancellation and again during cleanup. | Selected forwarding/escalation policy observed; actual outcome distinguished from delivery, teardown bounded, no duplicate unsafe removal and recovery retained if interrupted. | CI runtime; local domain supplements cancellation ordering. Blocked on Cancellation and controller loss; Resource and wait budgets; Safe recovery. | Worker observations, bounded termination/cleanup and preserved record fail if delivery is treated as exit, ignoring work hangs or repetition loses state. |
| Zero/nonzero exit and successful cleanup (AC-22) | Separate fixed-exit workloads, zero and 17, with checkout/state marker writes. | Invoke each run and await completion. | Return 0 and 17 respectively; owned ephemeral resources absent; checkout/state markers preserved. | CI runtime; local domain result/cleanup fakes. | Exact result plus independent absence/preserved files fail if nonzero is erased, zero reported early or cleanup deletes persistent state. |
| Zero/nonzero exit and failed cleanup (AC-22, AC-23) | Same two fixed-exit workloads; trusted removal fault yields a known transient engine failure. | Run each; inspect result, diagnostic and recovery state; restore engine and invoke printed stop. | Nonzero 17 is preserved; zero becomes wrapper 75 for this transient cause; separate cleanup diagnostic, session ID and concrete retry; stop removes only owned ephemeral resources. | CI runtime; local domain classification fakes for other wrapper causes. Blocked on Safe recovery. | Result precedence, actionable retry and retained identity fail if cleanup replaces 17, zero is returned or recovery is discarded. |
| Engine loss leaves unknown workload result (AC-22, AC-23) | Workload barrier before exit observation and trusted engine-disconnect fault. | Run, disconnect before result can be observed, then inspect retained state; restore engine and invoke status/stop. | Surviving controller reports temporary failure for the injected cause, unknown child outcome and recovery; never synthesizes zero/nonzero child exit; safe cleanup reconciles resources. | CI runtime; local domain unknown-result fake. Blocked on Safe recovery; Cancellation and controller loss. | Explicit unknown result and nonzero wrapper failure fail if engine loss is misreported as workload termination/success. |
| Forced controller loss retains isolation (AC-23, AC-10/AC-11 offline) | Running-boundary probe and trusted controller kill barrier; working network controls; marker edits/state. | Start run, kill controller uncatchably, observe fresh rounds from the same worker before and after status, then invoke its exact stop retry. | The surviving worker stays isolated until explicit stop; owned removal preserves edits/state and unknown workload history without restart or synthetic success. | CI runtime; live ID-LIFECYCLE-15 in slice-15 after merged slice-12 and slice-14. Accepted regression under q-32. | Denied access after loss with positive controls, unchanged worker identity and eventual owned cleanup fail if controller death opens access or loses recovery. |
| Interrupted or unavailable cleanup (AC-23) | Workload exits with markers; barriers before removal and before absence confirmation. | Run; interrupt cleanup or make engine unavailable at each barrier; later restore access and invoke status and printed stop. | Incomplete state retained; no false stopped/absent claim, bounded failure, session-specific retry; later cleanup finishes without losing markers. | CI runtime; local domain state/engine fakes. Blocked on Safe recovery; Cancellation and controller loss; Resource and wait budgets. | Recoverable incomplete status and eventual confirmed removal fail if interruption drops ownership or unavailability is treated as absence. |
| Bounded teardown and recovery (AC-22, AC-23) | Trusted faults separately stall signal delivery, stop, removal and absence inspection, with retryable responses and person-set budgets; marker workload/state. | Run and cancel or finish the workload as appropriate; measure teardown, then invoke status/stop under the recovery fault variants. | Each attempt ends within its settled total budget despite retries; incomplete cleanup retains a specific diagnostic and usable recovery reference; no unknown resource is declared absent. | CI runtime; local domain controlled-time engine/state fakes. Blocked on Resource and wait budgets; Cancellation and controller loss; Safe recovery. | Per-attempt elapsed bounds and retained evidence fail if a stalled call, retry or repeated cancellation resets teardown/recovery indefinitely. |
| Repeated stop, absent and stopped resources (AC-23) | Trusted owned-record fixtures for absent worker, stopped worker and partial creation including temporary storage; persistent markers. | Invoke status and stop, then stop again. | Confirmed-absent resources are harmless; remaining ephemeral resources removed; persistent state unchanged; repetition safe without restarting work. | CI runtime; local domain cleanup fake. Blocked on Safe recovery; Crash-safe ownership. | Idempotent cleanup and preserved persistent markers fail if absence is fatal, stopped workers leak or a retry restarts/deletes state. |
| Foreign, wrong-user and ambiguous ownership (AC-06, AC-23) | Foreign resource and records with mismatched IDs, wrong owner, matching labels/name or ambiguous identity; independent resource observer. | Invoke stop for each presented session and inspect status. | Specific ownership refusal; foreign/ambiguous resources unchanged and not adopted; no labels-only removal. | CI runtime; local domain ownership fake. Blocked on Approval and identity; Safe recovery. | Specific diagnostic plus intact resource identity fails if label resemblance authorizes deletion or unknown-command masks refusal. |
| Two independent projects and concurrent cleanup (AC-06, AC-28 offline) | Two disposable projects with equal basenames, separate approval/state and marker workloads; stop concurrency barriers. | Run both; stop one, including concurrent repeated stop attempts; observe the other, then finish its run. | Other workload/resources and both projects' edits/persistent state survive the first cleanup; no cross-project mutation or double-cleanup damage. | CI runtime; local domain state concurrency fake. Blocked on Approval and identity; Safe recovery. | Independent resource continuity and preserved markers fail if basename identity, races or broad pruning cross session boundaries. |
| Sensitive markers stay out of controller records (AC-36 offline) | Distinct harmless `SYNTHETIC_ARG_MARKER`, `SYNTHETIC_ENV_MARKER` and `SYNTHETIC_OUTPUT_MARKER` in argv, host environment and workload output; safe approved snapshot; faults for startup, attach and cleanup. | Invoke run variants; capture workload streams separately from controller stderr, status and recovery records; retry stop after faults. | Deliberately streamed output may contain its marker; controller diagnostics/status/recovery contain none of these markers or raw argv/environment/output; worker does not inherit host marker; trusted effective snapshot remains available under slice-4. | CI runtime; local domain sanitization fixtures. Blocked on Safe recovery; Approval and identity; Manifest and writable state. | Marker exclusion from controller channels and worker host-environment denial fail if errors dump raw inputs or recovery captures output; suppressing all workload output cannot satisfy the stream control. |

AC-01/02/03, AC-05/06/07, AC-21/22/23/25 and AC-38/40/41 are traced above. The offline
portions of AC-10/11 deny all external paths, rather than testing approved egress.
AC-28 covers independent projects and cleanup; publishing/port-collision cases wait
for ingress. AC-36 covers controller metadata, not a proxy log implementation. AC-37
requires recorded image digests, host/engine/architecture, exact probes and outcomes
for this one target only. AC-42 covers only the hooks the manifest spec eventually
selects. AC-24 exec and AC-26/27 ingress are explicitly deferred. No network Git,
accounts, proxy, alternate engine or additional platform coverage is implied.

The pending `p1-engine-scenario-tier` work must provide explicit engine-test selection
before CI runtime cases become executable. An `@wip` tag alone is not an engine-access
boundary. Later behaviour specs mint IDs through `itos tests next-id` and commit their
scenarios as `test` with the implementing item. Their implementer commits step
definitions red first while scenarios remain `@wip`, proves each fails at its behaviour
assertion with `@wip` removed only at the permitted tier, then implements and proves
green at that tier. Undefined steps, setup failures and pre-existing unknown-command
refusals provide no evidence. Never run real-engine cases locally or give an agent the
Docker socket.

T-11 creates no steps and claims no red/green runtime evidence or sandbox support.
Its ledger checks cover document presence, headings, authority links and these table
headers; semantic review against the task is also required. It touches no `proof.code`
path and needs no mutation results. Let configured hooks/CI run wider checks; the loop
alone pushes, watches the actual head and closes the item once required checks pass.

## Security decisions

These record T-11's named questions, not invented decision numbers or approved
designs. Controller loss is now settled as described above; the remaining entries
retain their original planning scope. The person must settle them through the
appropriate specifications before dependent runtime work proceeds. Listing a sibling
item does not declare its work finished. No default timeout, resource limit, selected
library, network-service dependency, security exception or stronger support claim is
authorized by this spec.

| Question | Required decision | Blocks |
| --- | --- | --- |
| Approval and identity | Through slice-3/slice-4, settle canonical checkout/host-user/local-engine identity, safe path and substitution handling, approval canonicalization/fingerprint/storage, supersession and atomic frozen effective settings. Reuse strict policy/manifest contracts from slice-1/slice-2. | Pre-creation validation and snapshot use; Approval absent/invalid/mismatched/superseded, Frozen policy and manifest, Unsupported engine or checkout, Foreign ownership and Two independent projects cases. |
| Manifest and writable state | Through slice-2 and readiness work, settle supported hook syntax/order/failure/repetition, image metadata treatment, worker environment restrictions, approved non-root identity and checkout/project-state/temp mappings. Hidden settings must be included in approval or explicitly refused/unused as specified. | Ready-to-setup/launch transition; Explicit shell, Strict inputs, Hidden image metadata, Non-root writable boundary, first-instruction probes, Setup failure and Sensitive markers cases. |
| Image readiness | Settle worker digests, provenance/trust, architecture/compatibility checks, update approval and explicit person-operated acquisition. No automatic pull/build or substitute image. | Cached image preflight and every runtime fixture; Cached image readiness and Hidden image metadata cases. |
| Offline boundary | Settle enforcement and independent verification before the first worker process, including IPv4/IPv6, gateway/private routes, DNS and no publishing. Determine how closure persists through setup failure, interruption and controller loss; no topology selected here. | Ready state and every isolation claim; First-instruction IPv4/IPv6, DNS denial, Non-root writable boundary, Setup failure and Forced controller loss cases. |
| Crash-safe ownership | Settle durable ownership before execution and reconciliation when create succeeded but its response or durable record was lost. Define sufficient trusted evidence to distinguish owned, absent, foreign and uncertain resources without deletion by name/label alone. | Any resource creation/launch and recovery after partial startup; Ownership write fails, Create response lost, Cancellation in partial startup and Repeated stop cases. |
| Resource and wait budgets | Settle resource limits and finite per-call/startup/teardown/recovery budgets, retry accounting, stalled operations, setup/attach and stream backpressure. Do not infer a maximum workload duration. | All bounded waits and timeout classifications; Bounded startup/calls/retries, ignoring-workload, Interrupted cleanup and recovery cases. |
| Cancellation and controller loss | The original cancellation questions concern platform outcomes, signal targets/forwarding/escalation/grace, repetition and result precedence. Controller loss is settled by A3: retained isolation until explicit owned stop, with ID-LIFECYCLE-15's independent post-loss evidence accepted as regression coverage under q-32. | Original cancellation/timeout and loss cases follow their later lifecycle specs. Controller loss no longer blocks slice-15; its required dependencies are slice-12 and slice-14. |
| Safe recovery | Settle protected recovery fields/storage, safe session-ID rendering, redaction, unknown-outcome reporting, ownership revalidation and concurrency among run/status/stop. Specify the status/stop interfaces and safe interrupted/repeated cleanup before promising an executable retry. Distinguish metadata from slice-4's trusted effective snapshot. | Cleanup completion, failed-state reporting and actionable recovery; cleanup-failure precedence, engine/controller loss, Repeated stop, Foreign ownership, Two independent projects and Sensitive markers cases. |

Security choices remain with the person. Completion of this documentation is not a
request to improvise them, change the charter/pilot or start any runtime experiment.
