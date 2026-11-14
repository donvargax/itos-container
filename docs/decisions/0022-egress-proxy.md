# T-38: our own egress proxy, its topology, delivery, failure closure and refusal log

Recorded for T-38 on 2026-11-09. This is the design the second milestone's first step
needs fixed before any code: the [charter](../charter.md), "The second milestone", step 1.
It is a proposal under the architecture hypothesis, not runtime evidence and not a support
claim. T-39 has since built `internal/engine/egress`, T-40 has built the session
engine's networks, proxy lifecycle and log-copy protocol, and T-51 has built the durable
refusal-log writer. None of the three is bound until slice-27, and none of these tests is
runtime evidence. The record itself changed no dependency, scenario, step definition,
workflow or engine run. Command wiring alone remains with slice-27. Real-engine proof
belongs to slice-27 and slice-28, in CI only. T-39, T-40 and T-51 take their design from
this record.

[PLAN.md](../../PLAN.md), [README.md](../../README.md) and
[docs/ORCHESTRATING.md](../ORCHESTRATING.md) still call egress out of scope. The charter's
"Specify and start step 1 now" governs, and `p1-docs-admit-milestone-two-egress` owns their
edit; that wording is no reason to stop here.

## What the record settles, and what it does not

It settles the library question, the proxy's flow and parsing, the address rule, the
ClientHello check, the network topology, the delivery of the proxy, its failure closure
and teardown, and the refusal log. From the pilot contract's "Decisions still open" it
settles the concrete guard/proxy topology and its fail-closed handling, and for the guard
only its versions, digests, provenance and acquisition. Worker versions, final
destinations, resource limits and the platform matrix stay open.

It does not settle who may approve a sandbox that grants network. That stays with
`p3-approval-before-egress`, and no one should read this record as settling it.

## The proxy is our own code on Go's standard library

The person decided in q-51: the proxy is our own, on Go's standard library with no
dependencies. There is no library, no `go.mod` change, nothing new in
`THIRD_PARTY_NOTICES.md` and no network service. The person's estimate is about 300 lines.
Its flow, in this order:

1. read the CONNECT head;
2. check the name against the allowlist and the port against 443;
3. dial;
4. answer 200;
5. peek the ClientHello through `crypto/tls`;
6. replay the peeked bytes upstream untouched;
7. splice both ways;
8. log each refusal as one JSON line.

This is a deliberate exception to the charter's "prefer an existing, maintained library",
and the charter says so. The reasons, repeated from q-51: the proxy is security-critical
and small; the libraries either carry MITM machinery (goproxy) or are unmaintained
(cproxy); and q-49's check needs custom code anyway. If TLS interception is ever wanted,
the charter moves the proxy to a repository of its own first, and this code seeds it.

### No interception code exists anywhere in the tree

The proxy holds no CA, certificate or key and never completes a handshake. Its one
`tls.Config` sets `GetConfigForClient`, which always returns an error, so `crypto/tls`
stops once the ClientHello has parsed, before it chooses a version. T-39 proves this three
ways:

- A contract test in the adapter, `TestVerifiedThroughTheTunnel`, has a client verify
  through the tunnel and see the upstream's own certificate.
- `TestNoInterceptionCode`, an ordinary Go test in the adapter's package, parses every
  production Go file under cmd and internal, files go ignores on the runner included, and
  refuses any `crypto/x509` import, any `crypto/tls` import outside the adapter, any rename
  of `crypto/tls`, any `tls.` name in a file importing it but `Config`, `ClientHelloInfo` and
  `Server` (which refuses `tls.Certificate`, `tls.LoadX509KeyPair`, `tls.X509KeyPair`,
  `tls.Listen`, `tls.NewListener` and `tls.Client`), any key but `GetConfigForClient` in a
  `tls.Config` literal (which refuses `GetCertificate`), `ListenAndServeTLS` and `ServeTLS`
  in a file importing `net/http`, and `net/http/httptest`. It first proves itself on a table
  of offending sources, and fails unless it read at least 100 files, among them one under
  cmd and one in the adapter. This replaces the search for names this record first proposed:
  a correct peek writes `tls.ClientHelloInfo`, so `grep -F tls.Client` matches it, and a
  check in the ledger leaves it when the task closes. `TestNoRelaxingCode` holds the rest of
  "no flag, variable or build tag" and "recovers no panic" the same way, over egress's own
  production sources.
- `go list -deps` of `internal/engine/egress`'s packages shows no module but this one and
  no `net/http` (measured: `crypto/tls`, `net`, `io`, `os` and `encoding/json` do not
  import it).

The `go list` check runs only for T-39's own range. The two Go tests are ordinary tests, so
CI's `go test ./cmd/... ./internal/...` runs them on every later push. T-39 also adds three
depguard rules with fixtures, which hold the imports on every later push: `egress-tls` lets
production code import `crypto/tls` only in egress's adapter, `egress-x509` lets it import
`crypto/x509` nowhere, and `egress-imports` holds egress's own packages to a short list of
standard-library packages and their own, so `net/http`, `flag`, a library or another engine
needs a reviewed change. depguard cannot see a file go ignores on the runner, such as an
`_windows.go` file under cmd that the Windows release still compiles; the Go test walk reads
those. Test fakes generate their upstream's certificate in memory, so no key enters a file.

### Libraries considered and rejected

- **github.com/elazarl/goproxy v1.9.3** (BSD-3-Clause), the maintained candidate with
  eleven tags since 2026-01-17. Its `certs.go` init parses `CA_CERT` and `CA_KEY` on
  import, unconditionally, and its MITM path and signer are linked, so "no interception
  code" could not be said of it. It also brings about 3,700 production lines,
  golang.org/x/net and x/text at versions with OSV advisories, acceptance when a handler
  returns nil, plain-HTTP forwarding and `HTTPS_PROXY` read from the environment.
- **github.com/smarty/cproxy/v2 v2.1.1** (MIT), CONNECT only with no TLS code. Its last tag
  is 2024-07-09 and its production files are unchanged since v2.1.0 of 2023-06-22, and
  q-49's check would still have needed a guard socket, a replaying connector and one
  handler per request around it.
- **wzshiming/httpproxy v0.6.1** (MIT): one maintainer, a plain-HTTP forwarding path and a
  tunnel that closes both directions at the first EOF.
- **stripe/smokescreen v0.1.0** (MIT): purpose-built but with its own ACL engine and
  roles, Datadog, Prometheus, logrus and a private goproxy fork, so neither small nor a fit
  for a domain that owns the decision.
- **caddyserver/forwardproxy**: a Caddy plugin with no tagged release.

## Reading the CONNECT head

The proxy reads the head itself and imports no `net/http`, a choice q-51's "parse CONNECT"
leaves open. Nothing of the head is ever forwarded, so no second parser can disagree with
it, and all that is judged is one literal string against a list. `net/http`'s server would
only add what this design must then close: ErrorLog lines holding client addresses,
recovered handler panics, its own 400, 431 and 505 answers, and a parsed URL that hides the
target, since it reads `user:pw@name:443` as userinfo and host, and `name:443/x` and
`name:0443` also carry a usable `URL.Host` (each measured).

The adapter reads the socket up to the first CRLF CRLF, at most 8 KiB, under a 10 s read
deadline from accept, and every byte after that terminator belongs to the tunnel. The
domain parses the head as pure code, within its allowlist, so a domain test and the fuzz
test reach it. The request line is exactly a non-empty method, one space, a non-empty
target, one space, a version and CRLF, where the version is HTTP/1.1 or HTTP/1.0: curl and
Python 3.14 send 1.1 and `openssl s_client` sends 1.0 (measured), as Python 3.11's
`http.client` does (read in its source). A bare LF or CR in that line is malformed. Headers
are skipped unread: never judged, logged or forwarded, Host and Proxy-Authorization
included.

### What passes, and the order of refusals

A request passes only when its method is exactly CONNECT and its target is exactly a listed
name followed by `:443`. Anything else is refused with the first reason that fits, in this
order:

1. `malformed`: no complete head, or a request line of any other shape;
2. `not-connect`: any other method, compared exactly, whatever its target;
3. `malformed`: a CONNECT whose target is not a plain host, a colon and a port as the log's
   rule below defines them;
4. `literal-ip`: dotted IPv4 or bracketed IPv6;
5. `not-canonical`: its lowercase form without one trailing dot is listed;
6. `not-listed`;
7. `port`: the name is listed and the port is not the exact text 443, so `name:0443` and
   `name:80`.

So `GET http://listed.example/path HTTP/1.1`, what curl and urllib send for any http:// URL
once `http_proxy` is set, reads `not-connect`, not `malformed`. These are labels for the
person: only the exact match admits anything. T-39's domain decides on the name first, so a
refused name is never resolved and refusals are no DNS channel.

## The address rule (AC-12)

The contract's address rule stands, since the charter does not change it (AC-12). The
adapter dials with `net.Dialer`, network `tcp4` and a 10 s timeout, giving the rooted name
to the proxy's resolver, and checks each address in `net.Dialer`'s Control hook, which sees
the address actually connected, including CNAME targets and changed answers, with no second
lookup.

It refuses the IANA IPv4 special-purpose registry, including 10/8, 172.16/12, 192.168/16,
127/8, 169.254/16 and 100.64/10, plus multicast, from a table in the adapter: `net/netip` is
outside the domain allowlist, and a table this small needs no library
(`code.dny.dev/ssrf` rejected for that reason). An answer mixing valid and refused
addresses dials only the valid ones; one with none valid is a refusal with reason
`address`. IPv6 is out of this step: both networks have it off, so a listed name with only
AAAA answers fails as `dns`.

The adapter classifies a failed dial in four steps, in this order. Only the first reads the
error `Dial` returns, and only to learn whether resolution failed.

1. An error holding a `*net.DNSError` (`errors.As`) is `dns`. That covers a name that does
   not exist, a name with no IPv4 answer (what only AAAA answers give under `tcp4`) and a
   resolver that does not reply before the dial's timeout. Resolution runs once, before any
   address is tried, so the Control hook has not run and both flags below are unset. Nothing
   but the error tells this from step 4, and a flag read alone would take it for an address
   refusal and answer 403 where this record says `dns` and 502.
2. The Control hook sets an atomic `passed` flag for the dial when any address passes the
   table, and a `refused` flag when it refuses one. With `passed` set the failure is
   `connect`, whichever order the answer lists the refused addresses in.
3. With `passed` unset and `refused` set, the hook refused every address it saw: `address`.
4. With neither set, the dial failed before any address was tried and not in resolution (a
   socket that could not be created, say): `connect`.

The returned error cannot decide between steps 2 to 4, because `net.Dialer` keeps the first
address's error: for the answer 10.0.0.1 then 11.22.33.44, with the second timing out, the
error is the hook's, and for the reverse order it is the timeout (measured twice on Go
1.27.3 against a fake resolver: in the review, and again here with a fake DNS server, whose
hook refused 10/8 and let 11.22.33.44 pass). By the error alone one upstream fault would
read `address` and 403, or `connect` and 502, by answer order. The same fake DNS server
showed why step 1 exists: no such host, an empty answer and a resolver that never replied
each failed with a `*net.DNSError` (not found, not found, timeout), and the hook ran not once
(measured here).

T-39's adapter tests cover both orders of the mixed answer, expecting `connect` each time,
besides an answer with none valid (`address`), the three resolution failures just measured
(`dns` each, the last with a dial timeout short enough that the test does not wait 10 s) and
a refused connection (`connect`). The valid address in the mixed test must pass the
adapter's own table, so it cannot come from a documentation range such as 203.0.113.0/24,
which the table refuses; 11.22.33.44 passes.

T-39's adapter tests reach their loopback listeners by giving its constructor a test
resolver and dial function, and test the table directly. Porttest fakes stay free of I/O,
cmd always binds the real ones, and no flag, variable or build tag relaxes the table in the
product.

## The ClientHello peek (q-49)

The coordinator answered q-49 yes, provisionally; the person's q-51 and the charter have
since taken up its check, so this record treats it as settled. After the 200 the proxy
holds the upstream socket and moves nothing in either direction until the hello passes, so
a server that speaks first reaches no client. It sets a 10 s read deadline on the client and
runs `tls.Server` over a shim, a small `net.Conn` whose Read serves the bytes that came
after the head, then the socket, recording every byte and stopping at 64 KiB, and whose
Write discards, because `crypto/tls` sends an alert when the callback aborts (measured: 7
bytes) and a client must get nothing forged by the proxy.

`GetConfigForClient` captures `ClientHelloInfo`'s ServerName and Extensions. The domain
then judges them, in this order:

1. extension 0xfe0d, `encrypted_client_hello`, is `tls-ech`;
2. no ServerName is `tls-no-name`;
3. a ServerName other than the CONNECT name, byte for byte, is `tls-name`.

If the callback never ran the reason is `not-tls`, however that happened: bytes that are
not TLS, bytes that start like TLS and then fail to parse, a truncated hello, a client that
closes or stays silent to the deadline, or more than 64 KiB. Each refusal writes a record,
then closes both sockets with no byte forwarded. On a pass the proxy clears the deadline,
writes upstream the recorded bytes and then any unserved bytes from after the head, and
splices. The unserved bytes are those read with the head that `crypto/tls` never asked for:
they are never recorded, and the splice reads only the socket, so a replay of the recording
alone loses them. T-39 measured it: CONNECT, Go's 1,518-byte hello and a trailer in one write
gave the upstream 2,048 of 7,518 bytes at a 6,000-byte trailer, and this record first said
that a pass writes every recorded byte. Nothing crosses the boundary, since only the sender's
own stream breaks, but every byte after the head's terminator belongs to the tunnel. A failed
replay is event `upstream-failed`, reason `connect`, and closes.

## The splice

The splice is two goroutines, each an `io.Copy` between the raw `*net.TCPConn` pair, which
Go turns into splice(2) on Linux. The shim never reaches them and no wrapper type touches
the tunnel, so nothing can copy around the peek, which has finished before either starts.
When one direction reaches EOF the proxy calls CloseWrite on the other socket, so
half-close carries, and both sockets close once both directions have ended or either has
failed. There is no idle timeout, because agents stream for long periods; the contract's
resource limits stay open.

## Fuzz tests, and how the code proof sees the proxy

T-39 requires fuzz tests for CONNECT parsing and the ClientHello peek (q-51), beside
slice-27's real-engine scenarios. The proxy is the one process of this project a hostile
worker can talk to, and it has a route out, so a parser bug there is the boundary failing.
There is one target over the domain's head parser and one over the adapter's peek, written
as `internal/engine/policy/yaml/fuzz_test.go` is, with real hellos among the seeds.

- The head target asserts that nothing panics, that only an exact CONNECT of a listed name
  on 443 passes, and that the method, target and port it would log obey the log's rule
  below.
- The peek target asserts that nothing panics, that no byte reaches the client, that the
  recording is a prefix of the input within 64 KiB, that a pass has the exact ServerName and
  no 0xfe0d, and that a pass forwards the whole input: its recording, its unserved bytes and
  what the socket still holds together equal the input. The recording being a prefix cannot
  show the loss the replay sentence above describes, so that last assertion does.

Plain `go test` only replays seeds, which CI's `go test ./cmd/... ./internal/...` does on
every push, and every crash a run finds is committed as a seed under `testdata/fuzz`. So
T-39's done_when also runs each target with `-fuzz` for 30 s at `-parallel 2` and
`-fuzzminimizetime=0s`, which `itos ci run` runs for T-39's own range. Go's default spends
up to 60 s minimizing an interesting input while fuzzing waits: from a fresh corpus the
spike's peek target did 11,141 executions in its first 3 s and none in the other 27 s,
against 226,151 in all with the flag, and its head target found 18 new inputs against 109
(measured). A crash is still written to the corpus with the flag, just not minimized
(measured). Thirty seconds of each target found nothing and peaked at 120 MiB a process
here and 159 MiB in the spec review's runs, which is no firm bound, since the cached corpus
grows. This repository has no nightly tier for fresh runs after that, and this record adds
none.

The code proof measures changed lines with the engine off (arc42's check tiers), so the
proxy container never starts under it, and its exact Env and lack of a writable mount would
stop a `-cover` binary writing counters anyway (reasoned from the design, not run). T-39
therefore covers its serve loop with tests over loopback listeners, and slice-27 covers the
slice's handler and cmd's binding with tests that need no engine.

## Topology: two owned per-session bridge networks (AC-10, AC-11)

The internal network is created with Internal true, EnableIPv6 false and
`com.docker.network.bridge.gateway_mode_ipv4=isolated`. Plain internal is not enough:
Docker's port-publishing documentation says containers on an internal network can reach
host services on the bridge address, including those bound to 0.0.0.0, which is AC-10's
host gateway, and `features/offline_isolation_network_test.go` binds host listeners on just
such an address. Isolated mode assigns the bridge no address while containers on it still
reach each other (moby's `TestIsolated`), so the worker's closure depends on neither
iptables nor the proxy. It needs Docker 28.0, API 1.48; the recorded runners are 28.0.4 and
29.4.2. Doctor and run refuse an older engine as an unavailable control, and verify after
create that the network is internal, IPv6-off, isolated and has no IPAM gateway, because
old daemons ignore unknown options.

The egress network is a plain bridge with IPv6 off and only the proxy on it. It is owned
rather than the default bridge, which other containers share and a daemon may disable;
Docker's inter-bridge isolation keeps other networks from the proxy's port.

The proxy joins both networks at create, publishes no port and listens on 3128. Its default
route comes from the egress network, the only one of the two with a gateway. Its HostConfig
sets Sysctls `net.ipv4.ip_forward=0`, verified by inspect before start, so its kernel
cannot route between the two networks: a new network namespace inherits the host's value,
and a Docker host runs with forwarding on (measured under `unshare -rn`: 1 on a host at 1).

The worker joins the internal network only, with no published port and its existing
CapDrop ALL, so it can neither craft packets nor change routes. Its environment carries
`HTTP_PROXY`, `HTTPS_PROXY` and their lowercase forms with the proxy's literal internal
address, read by inspect after ready, plus `NO_PROXY` and `no_proxy` set to
`localhost,127.0.0.1,::1`; the manifest already reserves these names in any letter case.
Without the last two a test suite's local server would go to the proxy, since curl and
Python's urllib both send localhost through `http_proxy` (each measured), and each such
request would count toward the log's cap. Loopback stays inside the worker, so this does
not touch the boundary. The variables help cooperating clients and are not the boundary: a
client ignoring them has no route.

### DNS: the result replaces "block embedded Docker DNS" (q-50)

Docker's embedded resolver at 127.0.0.11 cannot be removed from a user-defined network, so
the contract's "block embedded Docker DNS" gives way to its result, AC-11's no unauthorized
resolution path. That is the coordinator's answer to q-50, provisional for the person's
review. For a sandbox with only internal endpoints libnetwork starts the resolver with
proxyDNS false, skips host-loopback servers and sends the rest from the container's own
namespace, where no route exists (`sandbox_dns_unix.go` and `resolver.go`; read, not run).
Clients need no DNS, since the proxy resolves. slice-27's AC-11 scenario must prove this on
the real engine; if it cannot, the topology reopens. Offline runs keep NetworkMode none,
with no proxy or network.

## Delivery: the host's own executable, an inert root filesystem

The proxy container runs the host's own itos-container executable, at its resolved
`os.Executable` path, bind-mounted read-only at `/itos-container` as that container's only
mount, through a hidden `__egress-proxy` command. That command is a slice of its own,
`internal/slices/egressproxy`, which slice-27 creates with only a command package: a thin
handler over the engine's port, with cmd binding the adapter. An owner needs no empty
domain or port directory, and a command cannot import `net`, so listener, dial, peek and
splice stay in `internal/engine/egress`'s adapter, which is T-39's "entrypoint". The hidden
`__complete`, which cmd handles itself, is no precedent: arc42's cmd row exempts only
version and completion infrastructure, and this is a long-running network server.

No image is built, published or pulled by a command, no second artifact ships, and a
scenario exercises the proxy of its own commit. The root filesystem is inert:
`docker.io/library/busybox@sha256:5624bb6777cf469b1fb50ae3f981b15d2715ee2fffaf20b9998cfb7843726835`,
T-20's pinned image, which the engine job already pulls. Docker needs some image; nothing
in this one runs. It is a product constant that neither policy nor manifest can change,
because the contract says the manifest cannot configure or replace the trusted proxy. The
person pulls it as they pull the worker's image, so this project distributes none of its
GPL-2.0 content; doctor names the exact pull when it is absent, commands only inspect it,
and approval binds the digest, so changing it needs a new approval. slice-26 names the
network-granting profile.

The executable must be a static Linux amd64 ELF, and doctor and run refuse anything else
(`debug/elf` behind a port). `.goreleaser.yaml` already builds with `CGO_ENABLED=0`.
`features/features_test.go` does not, and its build measured dynamically linked here;
`p1-acceptance-static-binary` makes the harness build what the release ships, and slice-27
depends on it.

Rejected:

- the worker image as root filesystem, which puts a checkout-requested image and its ENV
  under the trusted proxy;
- a published proxy image, which adds a registry service (the charter's stop) and a
  digest-in-source cycle;
- importing an image at run, which the contract's "no automatic pull/build path" bars.

### Keeping the proxy out of the worker's reach

The worker's mounts remain exactly the checkout and tmpfs, as `VerifyRun` already enforces;
the two containers share no volume or namespace; the worker has no engine access; and run
refuses an executable whose resolved path lies inside the checkout, which a worker could
replace for the next session (slice-20's scan already refuses hard links there). The proxy
runs as the effective host UID/GID like the worker, with read-only root, CapDrop ALL,
no-new-privileges, pids limit 4096, restart policy no, no tmpfs, and no checkout, state
volume or socket. The frozen allowlist is its Cmd, its Env is exact, and both are verified
by inspect before start, so nothing the worker does can change them and a restart cannot
widen them.

## Failure closure: no fallback network

There is no fallback network: nothing reconnects the worker, changes its network or
restarts the proxy. Creation order is the anchor as today, internal network, egress
network, proxy, verify, start, ready, then the worker as today, with durable intent before
each. The checkout's pin, its socket scan and the anchor stay first, so a refused checkout
starts no proxy; the scan's known race (q-30) only grows by the proxy's start.

Ready is one record, event `ready`, that the proxy prints on stdout only after validating
the list and listening; the controller reads it with bounded log calls inside run's
existing 120 s startup budget. A missing ready, a proxy exit or an unverifiable resource
means no worker is created. Policy is compiled into the same process, so the contract's
addon-failure window does not arise: an invalid list exits nonzero before the listener
opens. The proxy recovers no panic, so a panic in any goroutine ends the process, and an
Accept error or a failed write to its stdout ends it too; each is the same closure. (A
store failure behind that write is not visible to the proxy: see "What the log cannot
promise".)

During a run the controller also waits on the proxy, and its exit is a typed failure: the
worker gets the existing termination (first signal, 10 s from cancellation, SIGKILL), an
observed exit keeps its precedence and run returns nonzero. With the proxy gone the worker
has no peer. Under A3 a lost controller leaves both running under the frozen list until
stop, with no watchdog and no new route.

### Teardown reverses creation

Teardown order: worker, stop the proxy and confirm it has exited, refusal-log copy, remove
the proxy, egress network, internal network, then the anchor. The copy waits for that exit
because a record reaches the json-file asynchronously, through a pipe to dockerd's log
copier, so a log read of a running proxy could miss a workload's last refusal and removing
the proxy would then lose it for good. Docker waits for that path only when a container
exits: `handleContainerExit` in moby's `daemon/monitor.go` waits up to 2 s for the streams
and up to 10 s for the copier before it marks the container stopped (read at v27.5.1 and
v28.5.2, not run).

The stop is SIGKILL, then the engine's not-running wait and a fresh inspection, as for the
worker's exit: each record is one write with nothing buffered in the process, so the kill
loses none the proxy wrote, and a proxy that has already exited goes straight to the copy.
What dockerd's copier did with a record after that write is another matter ("What the log
cannot promise"). This stop is the controller's own act once the worker is gone, not the
typed proxy failure, so it changes neither the run's result nor its exit code. A failed
step retains its recovery record and exact retry. Removal is authorized as arc42's protocol
has it, by durable intent with a random name before each create and then the inspected full
ID, recorded before any instruction, never by labels or names alone (T-40).

## The refusal log

One JSON object per line on the proxy's stdout, at most 2 KiB: schema 1, seq, time (RFC
3339 UTC) and event, which is one of `ready`, `refused`, `upstream-failed` or `log-full`. A
`refused` or `upstream-failed` record adds method, target, port and reason; `ready` and
`log-full` add nothing. `seq` is a per-process counter: `ready` is 1 and every later
record, of any event, is the previous plus one, so a gap in what is stored shows that
records were lost.

- A `refused` record's reason is one of `not-listed`, `literal-ip`, `port`,
  `not-canonical`, `not-connect`, `malformed`, `address`, `not-tls`, `tls-name`,
  `tls-no-name`, `tls-ech`.
- An `upstream-failed` record's reason is `dns` or `connect`.

Method is written only when it is one to 16 ASCII uppercase letters. Target and port are
written only when the text before the request target's last colon is a non-empty plain
host, at most 255 bytes of ASCII letters, digits, hyphen, underscore and dot or a bracketed
IPv6 literal, and the text after it is one to 5 digits; otherwise both are omitted. A
request line of any other shape writes none of the three. So `secret-token@listed.example:443`,
`a/b?token=secret:443`, `listed.example:443/path?token=secret` and
`http://listed.example/path?token=secret` write neither target nor port, though
`net.SplitHostPort` accepts all four (measured), and slice-28's AC-36 marker scenario uses
these forms.

Never written: URL path or query, userinfo, headers, Proxy-Authorization, bodies, tunnel
bytes, the hello's SNI, client address or Go error text. Every way a head can fail leaves
one `malformed` record, a partial head and a deadline expiry included; a connection closed
before its first byte asked for nothing and leaves none. An allowed tunnel writes no
record. Each record is one write, made before the client is answered, and a failed write
to the proxy's stdout ends the process, because a refusal that cannot be written must stop
the session. That covers the proxy's own write only: whether dockerd then stores the
record is not visible to the proxy ("What the log cannot promise").

### What the client is told

A refusal decided before the 200 answers `HTTP/1.1 403 Forbidden`, an address refusal
included. A DNS or connect failure on a listed name is event `upstream-failed`, reason
`dns` or `connect`, answered `HTTP/1.1 502 Bad Gateway`, so the status and the log both
tell policy from failure. Both answers carry `Content-Length: 0` and `Connection: close`
with no body, and the proxy then closes. The tunnel's answer is
`HTTP/1.1 200 Connection Established`. Handshake refusals come after that 200, so they
close the connection with no answer.

### Where the log lands

The log lands in two places. During the run it is the proxy's json-file log (max-size 64m,
max-file 1), which survives controller loss. At teardown, after stopping the proxy and
before removing it, the controller copies every record but `ready`, fsynced and mode 0600,
to `sessions/<lookup>/<id>/egress-refusals.jsonl` under the protected state root.

`Publish` in `internal/engine/approval/disk/storage.go` refuses more than 1 MiB, which this
log can exceed, so T-51 adds a streaming writer with the same private-file, fsync,
rename and parent-fsync rules ("What T-51 settled"). A failed copy keeps the stopped proxy, its log and its
recovery record, so stop retries and the stored log is not lost. Cleanup never deletes that
file; status names it and the number of records stored, and display escapes a hostile name
as approval's does (slice-28).

The copy also checks `seq`, with `ready` included, and a gap is a reported failure: the
copy still writes every record it holds, each with its `seq`, status names the missing
range instead of a clean count, and the run or stop that tore the session down exits
nonzero with a typed error naming it. Retrying cannot recover a record dockerd never
stored, so a gap does not hold the proxy or its resources. slice-28 owns the check, its
error and a scenario with a log that has a hole.

The proxy stops after 20,000 refused or upstream-failed records, writing one `log-full`
record and exiting nonzero, which the run treats as a proxy failure. Every refusal the
proxy writes stays in the log, and a hostile workload can end its own session but cannot
rotate away its earlier attempts, since a session's records, each at most 2 KiB, stay under
the 64m cap even after Docker's escaping. That bounds one session at about 40 MiB of host
disk. Retention across
sessions stays open (`p3-session-state-retention`): completed sessions are not
garbage-collected (`features/recovery.feature`), so these files accumulate with them.

### What the log cannot promise

The proxy knows its own write to the pipe to dockerd, not what the json-file driver does
with it. When a store fails, dockerd's log copier reports the error and carries on
(`daemon/logger/copier.go` at v28.5.2 and v29.4.2, read in the review, not run), and
`handleContainerExit` bounds the copier's wait at exit to 10 s. So the proxy can write a
refusal that never reaches the json-file, and the teardown copy then succeeds on a log with
a hole. The case to expect: Docker's data root shares a filesystem with the checkout and
has no root reserve, as on XFS or btrfs, or the host disk is simply full; a hostile worker
fills it through its unquota'd checkout and sends refused CONNECTs, and every pipe write
still succeeds, so the proxy never exits and the session continues.

`seq` makes a hole inside the stored records visible. It cannot show a lost tail, since no
later record exists to reveal the gap, and a disk that stays full to teardown, or a copier
cut off at 10 s, loses exactly the tail. A log that ends early reads as a complete one.
The copy fails loudly only if its own write under the state root fails, which a full Docker
data root does not cause unless the two share a filesystem.

So the count that status shows is the number of records stored, not proof that every
refusal made it in, and the record's other claims about the log read that way. This is a
disclosed limit, not a closed one: approve, status and `docs/security.md` say so beside the
handshake guarantee (slice-26 and slice-27).

Closing it takes a security decision this record does not take, because either way changes
what the proxy may hold, and the project's rule is to stop for such decisions rather than
invent a protection. There are two candidates. A sink the proxy writes and fsyncs itself,
such as a volume mounted only into the proxy, lets it see a full disk and exit, so the
session fails closed mid-run; but it gives a proxy that now has one read-only mount a
writable one, and a Docker volume lives in the same data root, so it must be sized and
cleaned up as the proxy's own. A terminal record the proxy writes on a clean stop would
expose a lost tail at teardown, because the copy would find no such record and report the
log incomplete, with no second sink and no new mount; but it detects after the fact, so the
session is not stopped mid-run, and a killed proxy would read as incomplete too. Neither
is built or specified here. The person confirms or overturns this with the record's other
decisions ("What rests on an answered question"), and until they do, the limit above
stands and no claim in this record depends on a closure.

## The handshake guarantee (q-49)

The proxy reads the ClientHello without terminating TLS or holding a key or CA, which the
charter's "no TLS interception" permits, and T-39's "opaque tunnelling" means opaque after
that hello. The guarantee to disclose in approve, status and `docs/security.md`: the proxy
verifies the CONNECT name and the first ClientHello's SNI. It cannot see the HTTP Host
inside TLS or anything after that hello, a second hello after a HelloRetryRequest included,
so fronting remains a limit, as does a hello the upstream's TLS stack reads differently from
`crypto/tls`, and it cannot stop data sent to an allowed service. Clients that omit SNI or
send ECH, Chromium's GREASE included, and SSH over 443 are refused until a later mode names
them. Only the extension code point 0xfe0d is refused as ECH: a hello with ECH's older code
points 0xfe0a or 0xffce and the exact SNI passes (`TestOlderECHCodePointsPass`), because no
upstream is known to honour them, fronting is already disclosed here, and widening the
refused set is a security decision T-39 did not take.

That Codex and Claude Code send SNI without ECH is not yet known: the person's smoke check
confirms it, since no scenario uses a real harness. The disclosure has owners: slice-26
puts it in approve's presentation, and slice-27 puts it in status and edits
`docs/security.md`, which `p1-docs-admit-milestone-two-egress` does not cover. This settles
the contract's "Which passthrough handshake guarantees can be enforced".

## Scenarios need no product seam

In the engine job the harness creates its own bridge network with a subnet outside the
refused table (for example 11.22.33.0/24) and a fake TLS endpoint on it whose network alias
equals a listed name, then connects the running proxy container to that network, so the
proxy's embedded resolver answers the alias. A second network with a private subnet and
alias proves the address refusal. The workload waits on a checkout sentinel until the
harness has connected. No flag, variable or build tag enters the product; this is the
charter's "fake endpoint the proxy allows in tests".

## Contract passages this record supersedes or defers

The person can edit the pilot contract from this list. Nothing in the contract file is
edited here.

- The Agreed scope's Proxy row (mitmdump) and TLS row (inspection per destination) give way
  to the charter by the person's 2026-11-09 choice.
- Its Images row, for the proxy only.
- Its Mounts row's "do not bind-mount additional host paths" and the "Not included" list's
  "extra host-directory mounts", which must now read as the worker's, since the proxy has
  one read-only executable mount.
- Its Platforms row, since delivery needs the host's own static Linux amd64 ELF, so the
  darwin, windows and arm64 builds in `.goreleaser.yaml` cannot run egress this way, which
  the charter's "Not yet" allows for now; a later platform needs another delivery.
- "The precise guard/worker network topology is a blocking experiment" with Experiments
  item 1 (mitmdump), since this record settles the topology from reading and a loopback
  spike and leaves its proof to slice-27.
- The sentence "block embedded Docker DNS" (q-50).
- The HTTPS inspection and SSH tunnel modes with AC-15, AC-19 and AC-20, deferred.

## What rests on an answered question, and what this record decides itself

The Proxy and TLS rows rest on q-51 and the DNS sentence on q-50, which is still
provisional for the person's review. q-49 began as the coordinator's provisional answer,
and the person's q-51 and the charter have since taken up its check, so it is settled here.

The Images and Mounts readings, the two-network topology and the delivery have no question
behind them, though the charter's stop rule names contract changes and "Decisions still
open". The coordinator told the drafter to finish, and slice-27 must prove the topology and
delivery on the real engine, so the work goes on. This record marks all of these as its own
decisions, for the person to confirm or overturn. The refusal log's lost store is the one
thing it leaves open instead of deciding, and it is the person's ("What the log cannot
promise").

## Documents updated, under the charter's drift rule

[arc42](../architecture/arc42.md) is updated in the same change, in these sections:

- **Constraints** keeps offline's limits, names this record for the egress profile, and
  names q-51's exception beside its rule on dependencies.
- **Context and scope** replaces "Proxy egress, guard/proxy infrastructure" and the
  instruction not to create a guard, proxy or session network with egress's status.
- **Building-block view** adds `internal/engine/egress` (proposed, T-39, importing no other
  engine), `internal/slices/egressproxy` (proposed, slice-27) and session's networks and
  proxy (proposed, T-40). Its work-item table adds T-38 to T-40 and slice-25 to slice-28.
- **Runtime view** adds a subsection headed `### Egress run` with the order and failure
  closure.
- **Deployment view** adds the proxy and both networks and confines NetworkMode none to
  offline runs.
- **Cross-cutting concepts** adds the refusal log and, under dependencies, q-51's
  exception.
- **Decisions** links this record.
- **Quality requirements** maps AC-09 to AC-14, AC-16 to AC-18, AC-23 and AC-36 to slice-27
  and slice-28, marked unproven.
- **Risks and open questions** replaces the topology and handshake rows with pointers here,
  leaving TLS interception deferred.

The pilot contract, PLAN.md, README.md, `docs/ORCHESTRATING.md` and `docs/security.md` are
not edited here; the contract is the person's, and the others have the owners named above.

## Evidence and its limits

A scratch spike of the proxy's flow ran on loopback under Go 1.27.3, outside the tree: no
Docker, so it is no runtime evidence, and T-39's tests replace it.

- curl 8.23.0, Python 3.14's `http.client`, `openssl s_client` and Go's client each
  completed a verified handshake with the upstream's own certificate through it.
- A wrong SNI (curl `--connect-to` included), no SNI, an ECH hello from Go's client, plain
  HTTP, an SSH banner, silence, truncation, unparseable records and 70 KB of one-byte
  records each put 0 bytes upstream and none to the client after the 200, and left one
  record.
- A hello sent with the CONNECT, split byte by byte, split across two TLS records or
  followed by more bytes reached the upstream exactly as sent.
- Half-close carried both ways, and strace showed splice(2) on a 200,000-byte tunnel.
- The real resolver through the Control hook connected a mixed answer to its valid address,
  and gave `address` with none valid, `dns` for no such host and `connect` for a refused
  connection. A mixed answer whose valid address then timed out returned the hook's error
  when the refused address came first and the timeout when it came last, in the review's
  run and in a second scratch run with a fake DNS server on loopback, which is why the
  adapter classifies from the hook's flags. In a third run the same fake server answered no
  such host, an empty answer and silence: each failed with a `*net.DNSError` without the
  hook running once, which is why the adapter checks for that error before the flags.

Read, not run: Docker's port-publishing documentation, moby's `TestIsolated`, libnetwork's
resolver code and `handleContainerExit`. The topology, the missing IPAM gateway on inspect,
the sysctl, the file mount over BusyBox and the log-copy timing all rest on those readings
until slice-27 and slice-28 prove them on the real engine, in CI. Nothing here claims
container-host support.

## For the sibling items

T-39, T-40, T-51, slice-26, slice-27 and slice-28 take these decisions from this record,
including our own proxy and its fuzz tests, the `crypto/tls` depguard rule, the second
network, the forwarding sysctl, `NO_PROXY`, the address check, the hello peek, the
egressproxy slice, the inert image, the static harness build, the stop before the log copy,
the streaming log writer and the handshake disclosure. Their registry entries predate
these decisions, and this change edits none of them; where an entry differs, its drafter
reconciles it with this record.

## What T-40 settled

T-40 implements the session engine's part of this design without changing `cmd` or
introducing a command scenario. The existing codec still maps approval snapshots to an
offline binding. Slices 26 and 27 fill the grant and bind the executable; T-51 supplies
the durable refusal-log writer. Unit, fake and SDK HTTP-fixture tests exercise the engine;
only the later CI scenarios can establish runtime isolation.

- `Binding.Egress` is a comparable `EgressGrant`: approved names in order, separated by
  newlines without a final newline, and the inspected local proxy image ID. Zero means
  offline. Invalid nonzero grants and a missing refusal writer are refused immediately
  after preparation, before registration or any resource creation. Unregistered runs
  refuse any grant.
- Schema 3 retains schema 2's anchor rules and adds exactly `internal`, `network` and
  `proxy` resources, each with a name, ID and stage. Each create follows its durable
  intent; owned IDs are saved before isolation checks can refuse them. Worker creation
  requires all three owned. Cleanup reverses that order after worker absence and releases
  the anchor last. Completion and checkout admission require all resources gone. Schema
  1 and 2 encodings remain unchanged.
- Resource names are `itos-container-internal-<token>`, `itos-container-egress-<token>`
  and `itos-container-proxy-<token>`. Their labels bind intent, project, fingerprint,
  anchor session and kind (`internal-network`, `egress-network`, `egress-proxy`). Network
  labels are exactly those five; image labels may add to the proxy's. Fresh ownership
  inspection and a recorded full ID authorize removal, never a name or labels alone.
- The proxy runs `/itos-container`, `__egress-proxy`, `--`, then one argument per approved
  name. Its environment is the inspected image's own environment. The executable is its
  sole read-only bind mount. The adapter verifies the read-only root, UID/GID, dropped
  capabilities, no-new-privileges, private namespaces, pids limit, disabled forwarding,
  no restart and no additional mounts, devices or published ports. The json-file logger
  explicitly uses `max-size=64m`, `max-file=1` and `mode=blocking`; verification tolerates
  other daemon-inherited log options.
- Both container create requests pin full NetworkIDs in their endpoint settings. The
  proxy joins both networks and the worker only the internal one. Pre-start inspection
  requires those IDs without assuming an allocated EndpointID, address, gateway or MAC.
  The HTTP fixture preserves the submitted settings; only start allocates endpoints.
  Before worker start, both networks must be owned and exact, with only the proxy attached.
- The worker receives `HTTP_PROXY`, `HTTPS_PROXY`, `http_proxy` and `https_proxy` pointing
  to the running proxy's inspected internal IPv4 address on port 3128. `NO_PROXY` and
  `no_proxy` are `localhost,127.0.0.1,::1`. These override image and manifest values.
- Only stdout is a record stream. Readiness and copying request stdout alone, with no
  stderr, timestamps or details, and discard any stderr frames returned anyway. The ready
  read follows once within one call budget and judges the first complete stdout line,
  at most 2,048 bytes before its newline. It requires exactly schema 1, seq 1, event
  `ready`, and an RFC 3339 UTC time ending in Z, with no trailing bytes. Daemon stream
  errors fail the read. Copying reads once without following and preserves stdout bytes,
  including ready, for T-51's ready filter and slice-28's sequence check.
- Proxy exit is watched on a separate connection from the worker's wait. It triggers the
  existing worker termination path and preserves an exit subsequently observed during
  teardown. Teardown's own SIGKILL and not-running wait precede the log copy and do not
  change the workload result.
- `RefusalLog.Begin` opens a staged `RefusalCopy` for the public session ID. A failed
  Begin, stream, Write or Commit retains the stopped proxy, networks, anchor and recovery
  intent. Copies support concurrent Write, Commit and Close, including a close after a
  call times out. A successful Commit publishes either a clean report or a committed
  `RefusalLogGap` report with inclusive missing ranges. A gap preserves every copied byte
  and permits resource release and admission completion. It remains separate from cleanup
  failure, earlier run failure and workload exit in `Result.RefusalLog` and recovery's
  failures; a later removal failure retains both diagnoses.
- An uncertain create is looked up by its random name. A matching resource is adopted by
  saving its full ID before removal. A name-only miss remains incomplete because it cannot
  fence a delayed create. Status observes the same ownership without adopting or writing.

The lost-tail question remains with `p2-proxy-log-lost-tail-sink`. T-40 adds no writable
proxy mount or terminal record: it implements the existing SIGKILL and json-file design.
Its stdout reader can carry an additional terminal record if the person later selects
that design. The stored-record count still cannot prove that dockerd retained every
refusal. Minimum-engine and static-executable preflight, command classification, status
presentation and real-engine evidence remain with the sibling slices named above.

## What T-51 settled

T-51 builds the durable refusal-log writer as a ledger task ahead of slice-27, which can
neither run nor tear down an egress session without one: a registered run refuses a grant
with no writer bound, and a failed copy keeps the stopped proxy. It changes no command and
`cmd` binds nothing new, so it has checks and no scenario. Its tests use fakes and a
temporary state root, with no engine, and none of them is runtime evidence. The order is
T-40, T-51, slice-27, slice-28.

- Approval's port gains `SessionLogs`, with `Stage(context.Context, Candidate, string)
  (StagedLog, Failure)`, and `StagedLog`, with `Write([]byte) Failure`, `Publish() Failure`
  and `Close()`. The string is the public session ID and the candidate is the one
  `RegistryStorage.Open` takes for that session. `approval/disk`'s `Registry` implements
  it, so one value serves both ports, and porttest's `Registry` models it in memory.
  `SessionLogsContract` runs unchanged against both.
- The one file a session directory stages is `egress-refusals.jsonl`, fixed in the adapter.
  There is no name parameter, so no caller can aim a write at `record.json`, a lock or a
  path. The temporary is `.refusals-`, then the 32 lowercase hex digits of a fresh 128-bit
  nonce, then `.tmp`, created exclusively without following a link and set to mode 0600.
  Its prefix is not `Publish`'s `.intent-`, so a record's link check never takes it for a
  twin.
- The staged log neither takes nor checks the session's operation lock. Every copy starts
  inside recovery's stop, which already holds the session open for writing, and that lock
  is a flock held by the open file description that took it. A second `Open` of a held
  session waits out its context and answers `state.unavailable`
  (`TestRegistryLockBudgetAndIndependentSession`), which would cost every copy a whole 10 s
  call and a teardown that never completes. Nor can the writer go around approval's
  storage: the pinned, no-follow, private-ancestor checks live in `approval/disk`, which
  another engine's adapter may not import. So staging is a capability of the protected
  storage itself. `Stage` pins `sessions/<lookup>/<id>` exactly as `Open` does, holds the
  four managed levels to the candidate's UID and mode 0700, creates no directory, and
  answers nil with no failure for a session that was never registered. The file's
  integrity does not rest on the lock: each staged file has its own random name and is
  published by one rename of a file already fsynced, so two publications cannot tear it
  and a reader sees the old file or the new one.
- A late publication is possible, and safe. The copy's Begin, stream and Commit are finite
  calls outside the session's gate. When a Commit outlives its 10 s call, stop reports an
  incomplete cleanup and releases the operation lock while `Publish` may still be inside
  an fsync or the rename, and another controller's stop can take the lock and copy beside
  it. [Decision 0017](0017-controller-operation-budgets.md) says: "A pending storage
  operation keeps its lock until it returns, so another controller cannot overlap a
  publication whose result is unknown." T-51 narrows that sentence to the operations that
  hold a lock, which the staged refusal log does not. The decision's own text stays as
  recorded. The overlap is safe because of what a copy publishes, not because of a lock.
  Commit is called only after the log read has returned success, so every publication is
  one controller's complete read. Every copy of a session reads the same proxy container,
  which its controller saw not running before the copy began and which nobody removes
  until a copy has succeeded, so its log has stopped changing and two complete reads store
  the same bytes. Each publication is one rename of its own fsynced temporary, so the name
  holds one whole copy whichever rename lands last, never a mix of two. And a copy is not
  a record: nothing compares the earlier bytes before publishing, so a late rename undoes
  no later state, as a late record publication would. A `Publish` that reaches its check
  after its controller's phase has ended renames nothing and answers `state.path`. Keeping
  the lock instead would mean running Commit through the session's gate, where a pending
  commit would stall the failed record that stop saves next. That changes T-40's protocol
  and is not T-51's. The narrowing is T-51's own decision, for the person to confirm or
  overturn.
- `Publish` runs once, in the order a record's publication keeps after its write. A
  poisoned handle is `state.write`. Then come file fsync and close, the pin unchanged and
  the lifetime live (else `state.path`), the rename over `egress-refusals.jsonl`, the
  directory's fsync, and the same check again. A failure before the rename leaves the
  earlier file as it was. One after it leaves the new bytes visible without a durability
  claim, and the retry publishes again. The rename replaces an earlier file, or a symlink
  at that name, without following it; a directory there fails it.
- A handle with nothing written publishes an empty file, which is how a session whose
  proxy refused nothing shows that its log was copied.
- The copy drops only the ready record: a line of at most 2,048 bytes before its newline
  that the ready read accepts, which is one JSON object with exactly `schema` 1, `seq` 1,
  `event` `ready` and an RFC 3339 UTC `time` ending in Z, and nothing after it. The
  judgement is repeated in `session/disk`, because an adapter may not import a sibling
  adapter and the port's allowlist has no JSON.
- Every other byte is stored unchanged and in order: every other line, valid JSON or not,
  with its `seq`; a line longer than 2,048 bytes; and a last line with no newline. Each
  Write stores the complete lines it holds before it returns, and between calls the copy
  holds at most 2,048 bytes of an unfinished line.
- T-51 derives no report from the bytes, so Commit returns the zero report. slice-28 adds
  the `seq` check, ahead of the ready filter, and its gap.
- Write and Commit run one at a time, each to its end, and Close alone never waits, because
  a copy whose call timed out is closed while a write or an fsync that cannot be
  interrupted may still be running. A Write or Commit that gets its turn after Close fails
  without calling the staged log. A Write still running when Close arrives closes the
  staged log itself before it returns. A Commit that had begun is not undone and may
  publish.
- A copy releases its staged log exactly once, on every path. `Publish` and `Close` are
  the only calls that release one, each on every return, and Commit always runs `Publish`
  on a live copy, so a handle poisoned by a failed write still gives up its temporary and
  its pin.
- Storage failures map as the session store's read path maps them: `state.path`,
  `state.unsafe` and `state.location` are `Unsafe`, `state.invalid` is `Invalid`,
  `state.conflict` is `Refused` and anything else is `Incomplete`. A session that is not
  64 lowercase hex digits is `Invalid` before any storage call, and one that was never
  registered is `Refused`. The domain turns every Begin failure into `Incomplete` and
  retries, which T-40 settled.
- A temporary can be left behind, and nothing sweeps it. A controller killed in the middle
  of a copy leaves its temporary, and so does one that merely exits while a timed-out
  Begin, Write or Commit is still running. Each such attempt can leave up to the log's
  size, about 40 MiB a session, and each retry on a slow disk can leave another. Nothing
  reads them, and deleting from protected state is `p3-session-state-retention`'s
  decision.
- Nothing binds the adapter until slice-27, which binds `RefusalLog`, over the same
  `approval/disk` `Registry` the session registry uses, to run and recovery. slice-28
  keeps the `seq` check and its typed gap, status's record count, display escaping, the
  proxy-failure scenarios and whatever reads the stored file, since a session directory's
  `Read` refuses that name and anything over 1 MiB. Scrubbing staged credentials from the
  copy is `p1-platform-log-redaction`'s, and the lost tail stays with
  `p2-proxy-log-lost-tail-sink`.

[arc42](../architecture/arc42.md) is updated in the same change, under the charter's drift
rule: Context and scope, the building-block rows for `internal/engine/approval` and for the
session networks and proxy, the work-item table, the Egress run's opening, and in
Cross-cutting concepts both Refusal log and Bounded operations, which gains the exception
above. Sibling specs, the pilot contract, decision 0017 and the registry are not edited.
