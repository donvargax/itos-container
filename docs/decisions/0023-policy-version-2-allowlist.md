# Slice-25: policy version 2 requests an exact host-name allowlist

Recorded for slice-25 on 2026-11-09. This decision overrides one text of
[arc42](../architecture/arc42.md): the building-block row for `internal/engine/policy`,
which called its schema "Strict offline". The row now says the engine validates policy
version 1 (offline) and version 2 (offline or an exact host-name allowlist), and names
this record. arc42's "Proxy egress ... deferred" text stays as it is: this slice builds
no proxy and no network, and T-38's record owns that deferral.

The charter's second milestone makes the egress allowlist part of the approved settings,
so it starts in `itos-container.yaml`. Policy is a request: nothing here approves it,
starts a proxy or opens a route. Slice-26 binds it in approval, T-39 and T-40 build the
proxy and its network, and slice-27 runs through them.

## Version 2, not a new key in version 1

[CONFIG.md](../CONFIG.md) rule 7 gives a new key a new version, and a retained version
keeps its written meaning. Version 2 is therefore version 1's closed object with a wider
`network`: `mode` is `offline` (still the default) or `allowlist`, and `allow` is a list
of host names, required and nonempty in allowlist mode and refused in offline mode.
Version 1 is unchanged in every command, so offline approvals keep their identity. The
published [version 2 schema](../schemas/itos-container-v2.schema.json) sits beside it.

## Names are canonicalised, then checked as full DNS names

Each entry passes, in order: a limit of 1024 bytes before any mapping; `idna.Lookup`,
which the adapter also makes refuse a final `xn--` because the library reads it as an
empty label and returns what a written trailing dot gives; removal of one trailing dot;
at least two labels of 1 to 63 bytes and at most 253 bytes in all; and a final label that
is neither all digits nor `0x` plus hexadecimal digits. The
first limit exists because punycode encoding is quadratic: a 64 KiB name of distinct
characters took the library four to six seconds on 2026-11-09, and refusing it first
takes milliseconds. Literal addresses in any spelling, wildcards, suffix shortcuts,
ports, URLs, empty names and repeats are refused at a located path without echoing the
value. The check is lexical; what a listed name may resolve to belongs to the proxy.

The effective list is sorted by the byte order of the canonical spelling and holds each
name once, so slice-26 can bind it as a set.

## Ordinary commands refuse version 2 until approval can bind it

Canonical settings v1 hardcode the profile `offline-v1` and network `offline`. If approve
accepted a version 2 file it would record offline settings for a request that asks for
network, and an offline approval would appear to match it. Ordinary doctor, approve and
run therefore refuse a valid version 2 file with `policy.version` at `/version`, before
the manifest, the engine or host state, in `internal/engine/readiness/domain/inspect.go`
after the shared policy inspection. Their fix names `doctor --policy-only`, which with
`--manifest-only` is the only reader of version 2 for now. An invalid version 2 file is
refused for its own problems. Slice-26 lifts the refusal for doctor and approve once
approval can bind version 2, and must keep run from starting a version 2 policy until
slice-27 can.

## Fix texts depend on the version read

Each issue records the supported version the file declared (zero when none was read), so
a version 2 file read by `doctor --policy-only` never gets a fix naming version 1, a
version 1 file never gets one that offers `allowlist`, and a file with no supported
version gets fixes naming both. The one exception is the refusal in the section above:
the fix of `policy.version` for a valid version 2 file, which ordinary doctor, approve and
run cannot act on yet, says to use policy version 1 or to read what the file requests with
`doctor --policy-only`.

## The library

`golang.org/x/net/idna` at `golang.org/x/net v0.60.0` (BSD-3-Clause, maintained by the Go
team; `net/http` uses the same Lookup profile for host names), checked on 2026-11-09.
`golang.org/x/text v0.42.0` joins `go.mod` as an indirect dependency. v0.59.0 is not used:
govulncheck lists five HTTP/2 advisories against that module version, fixed in v0.60.0. It
still lists two against `x/net/html` (GO-2026-6866, GO-2026-6860), a package nothing here
imports, so no symbol is reached. Their fix, v0.62.0, would also move x/sys, x/term,
x/sync, x/telemetry and x/tools, and the idna package is byte-identical in all three.
Its notices are in THIRD_PARTY_NOTICES.md.

The library is outside the domain allowlist, so canonicalising sits behind a policy port
with an adapter at `internal/engine/policy/idna`, an I/O-free porttest fake and a shared
contract run against both; `cmd` assembles it. The charter's rule that the proxy has no
dependencies (q-51) binds the proxy, not this adapter: canonical names are plain ASCII, so
the proxy can compare them without any IDNA of its own.
