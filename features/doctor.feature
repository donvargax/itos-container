@phase-1
Feature: Inspect checkout and cached worker readiness without executing checkout code
  Ordinary doctor reports observations about the checkout, the selected local engine
  and the requested cached worker image. Success is an inspection result, never host
  approval or proof that a running sandbox is isolated. Policy parsing belongs to
  slice-1, manifest and inherited devcontainer metadata validation to slice-2, and
  approval to slice-4. This feature grants no runtime permission or image acquisition.

  # Implementation boundary and dependency decision for slice-3:
  # Create internal/doctor/{command,domain,port,...} like internal/count. Reusable
  # inspection belongs in an engine package with its own ports/adapters and fakes,
  # not in count or another command's domain. OS/architecture are input values.
  # Use the maintained Docker Go SDK github.com/moby/moby/client v0.6.2 and its
  # github.com/moby/moby/api v1.56.1 types, pinned when implementation adds them.
  # It provides version negotiation and typed inspection without Docker subprocesses
  # or a hand-written Engine client. Configure the validated endpoint explicitly;
  # do not use FromEnv, Docker contexts, credential helpers or registry clients.
  # Preserve Apache-2.0 notices. Sources inspected 2026-11-04:
  # https://docs.docker.com/reference/api/engine/sdk/
  # https://github.com/moby/moby/releases/tag/client/v0.6.2
  # https://raw.githubusercontent.com/moby/moby/client/v0.6.2/client/go.mod
  # Filesystem inspection uses Go's standard library; do not invoke Git, Docker,
  # Dev Containers CLI, hooks, generators, builds or repository-configured helpers.

  # Fixture/step contract (new unless explicitly identified below):
  # "a doctor checkout" creates an ordinary, disposable Git checkout with embedded
  # .git, an unborn HEAD, refs/objects directories, a regular config, and the valid
  # offline policy and restricted manifest defined by slice-1 and slice-2. It writes
  # those files directly; it does not run the product to prepare or approve them.
  # The manifest requests worker.test/agent@sha256:<64 lowercase a characters>.
  # These aliases are fixture data, not real release digests or acquisition advice.
  # Image ID is sha256:<64 lowercase b characters>; a different digest uses c's.
  # The fixture must use the owning policy/manifest specs, never invent permissive
  # test-only configuration or production switches to bypass their validators.
  #
  # "a local Engine API fixture" starts a stateful HTTP fake on a temporary Unix
  # socket OUTSIDE the checkout, with its own request journal. It has no connection
  # to an actual engine. It supplies successful ping/version/info/image inspection,
  # product Docker Engine - Community, Linux amd64, daemon ID engine-A, API 1.56
  # with minimum 1.44, the requested RepoDigest, and the image ID above. Image
  # Config.User matches the non-root identity in the owning policy/manifest fixture;
  # Config has no devcontainer.metadata or implicit volumes.
  # It sets DOCKER_HOST for the child only. All caller DOCKER_*, proxy settings and
  # credential locations are cleared or redirected to disposable fixture state.
  # It records every request, including attempted mutations, redirects and failures;
  # forbidden requests must fail assertions even if the fake refuses them itself.
  # Local scenarios run on Linux amd64; other hosts exercise the unsupported-host
  # case. No fake result establishes actual engine or isolation support.
  #
  # Existing steps reused: "itos-container runs with ...", exit-code assertions,
  # stdout/stderr containment, JSON problem-name and complete-problem assertions.
  # All doctor fixtures, multi-run/controlled-race actions, JSON identity assertions,
  # filesystem snapshots and API-journal assertions below need new definitions.
  # Every Given only writes fixtures, sets child environment, or configures the
  # fake's responses. Then steps read outputs/journals/snapshots, never set them.
  # Snapshot fixture trees, permissions and control-state locations after all
  # Given steps and before invocation; include newly created files in comparisons.
  # Use scratch HOME/XDG state, never the operator's approval or Docker files.
  # Multi-run actions retain every process result even on nonzero exits. A race
  # action waits for either the expected fake request OR process exit: an absent
  # doctor must reach its Then with usage output, not fail/hang in synchronization.
  # Endpoint traps use allocated loopback ports, temporary sockets and PATH marker
  # programs (including ssh); example endpoint addresses are fixture aliases.
  # Never resolve external hosts, connect to real engines or use actual credentials.
  #
  # Black-box red-first handoff: commit those steps in test: while tags stay @wip.
  # Remove @wip only for the evidence run; every case must fail at its FIRST Then
  # below, since today doctor is unknown and reports usage/exit 2. This is an
  # expected failure location, not claimed red evidence from this spec-only commit.
  # Real-engine cases are CI-only and need explicit harness/CI selection before
  # becoming live; merely adding @real-engine must never expose a local socket.

  # Output decisions: --json and inherited ITOS_JSON=1 produce exactly one schema-1
  # object, ok and problems (empty on success), plus an inspection object. It holds
  # checkout {root, device, inode, git_device, git_inode}, engine {endpoint, id,
  # api_version, os, architecture}, image {reference, id, os, architecture}, and
  # ready, which describes ONLY these inspection checks. Device/inode values are
  # decimal strings, not lossy JSON numbers. Never emit run_ready or approved=true.
  # Facts not inspected are omitted, never invented. Plain output names the same
  # inspected subjects and says approval and runtime isolation are not established.
  # Problems have dotted lowercase rule, exit, message, fix and a flat subject field
  # (path for checkout, field for selection, image for image); never raw responses.
  # Top-level ok is false and inspection.ready is false on an inspection problem.
  # Below, "a doctor problem R with exit N" checks that full envelope, a nonempty
  # message/fix, the problem's exit, and the process exit N, not just a substring.
  # All readiness/identity assertions also check schema 1, ok true and no problems;
  # a named-problem outcome checks the same failure contract as the problem step.

  @wip @slice-3 @ID-DOCTOR-01 @local-engine-fixture
  Scenario: Inspect a standalone checkout and the exact cached worker image
    Given a doctor checkout
    And a local Engine API fixture
    When itos-container runs with "doctor --json"
    Then its JSON inspection identifies the checkout, selected engine and requested cached image
    And it exits with code 0
    And its JSON inspection is ready without asserting approval or runtime isolation
    And the Engine API journal contains only ping, version, info and the requested image inspection
    And the checkout and host control state are unchanged

  @wip @slice-3 @ID-DOCTOR-02 @local-engine-fixture
  Scenario: Explain successful inspection without claiming permission to run
    Given a doctor checkout
    And a local Engine API fixture
    When itos-container runs with "doctor"
    Then its standard output says "inspection ready"
    And it exits with code 0
    And its standard output names the checkout, engine identity and requested image digest
    And its standard output says "approval and runtime isolation are not established"
    And its error output says "for scripts, use --json"

  # Effective root is absolute ITOS_ROOT if set, otherwise cwd. It must name the
  # checkout root itself; no upward search and no PWD/GIT_DIR/GIT_WORK_TREE override.
  # ITOS_CONFIG remains core itos configuration, never the sandbox policy.
  @wip @slice-3 @ID-DOCTOR-03 @local-engine-fixture
  Scenario: Honor the extension root and JSON protocol without reading core configuration as policy
    Given a doctor checkout
    And a local Engine API fixture
    And ITOS_ROOT names that checkout while the child cwd is a different empty directory
    And ITOS_CONFIG names a regular core configuration file with unrelated keys
    When itos-container runs with "doctor" and the environment "ITOS_JSON=1"
    Then its JSON inspection identifies the checkout, selected engine and requested cached image
    And it exits with code 0
    And its error output does not say "for scripts, use --json"

  @wip @slice-3 @ID-DOCTOR-25 @local-engine-fixture
  Scenario Outline: Diagnose invalid or inaccessible effective roots without engine access
    Given a doctor checkout
    And a local Engine API fixture
    And ITOS_ROOT is "<root>"
    When itos-container runs with "doctor --json"
    Then its JSON output has a doctor problem "<rule>" with exit <exit>
    And the Engine API journal is empty

    Examples:
      | root                            | rule                        | exit |
      | a relative path                 | doctor.checkout.root        | 2    |
      | an absent absolute path         | doctor.checkout.unavailable | 3    |
      | an unreadable absolute directory | doctor.checkout.unavailable | 3    |

  # Identity is the canonical absolute root plus root and .git device/inode pairs,
  # not a basename, Git origin, branch, HEAD commit or hash of mutable file contents.
  # doctor does not persist or authorize this tuple. Approval/run must re-inspect it.
  @wip @slice-3 @ID-DOCTOR-04 @local-engine-fixture
  Scenario: Keep equal-basename checkouts distinct and an unchanged checkout stable
    Given doctor checkouts "one/project" and "two/project"
    And a local Engine API fixture
    When doctor inspects "one/project", "two/project" and "one/project" again as JSON
    Then the three JSON checkout identities match their filesystem identities
    And the first and second checkout identities differ
    And the first and third checkout identities are equal
    And all three inspections exit with code 0

  # Do not follow replacement paths after inspection starts. Pin the directories
  # used for reading and compare their identity to the selected paths before
  # reporting success. This detects observed changes, not future immutability.
  @wip @slice-3 @ID-DOCTOR-05 @local-engine-fixture
  Scenario Outline: Refuse an observed checkout substitution during inspection
    Given a doctor checkout
    And a local Engine API fixture paused before its image response
    When doctor runs as JSON and the fixture replaces <entry> before releasing the response
    Then its JSON output has a doctor problem "doctor.checkout.changed" with exit 1
    And the replacement tree is unchanged

    Examples:
      | entry         |
      | checkout root |
      | .git          |

  # No extra Git storage outside the sole checkout mount. Inspect directory entries
  # without following .git indirection. A gitfile covers linked worktrees, submodule
  # worktrees and separate git dirs; none becomes supported by naming a local engine.
  # A normal .git needs regular HEAD/config and embedded objects/refs directories.
  @wip @slice-3 @ID-DOCTOR-06 @local-engine-fixture
  Scenario Outline: Refuse unsupported checkout layouts before contacting an engine
    Given a doctor checkout changed to the layout "<layout>"
    And a local Engine API fixture
    When itos-container runs with "doctor --json"
    Then its JSON output has a doctor problem "doctor.checkout.layout" with exit 1
    And the Engine API journal is empty
    And the checkout and host control state are unchanged

    Examples:
      | layout                                  |
      | ordinary directory without .git         |
      | bare repository                         |
      | linked worktree with a .git file         |
      | submodule worktree with a .git file      |
      | .git directory with a commondir file     |
      | .git/objects/info/alternates file        |
      | .git/objects/info/http-alternates file   |
      | nested working directory below the root |
      | missing .git/HEAD                       |

  # Reject symlinks in the selected root's components or required .git metadata;
  # never canonicalize an untrusted alias into permission to inspect its target.
  # Ordinary symlinks in source files are not traversed or banned by doctor.
  @wip @slice-3 @ID-DOCTOR-07 @local-engine-fixture
  Scenario Outline: Refuse symlinked inspection roots and Git metadata
    Given a doctor checkout with "<entry>" replaced by a symlink to an outside fixture
    And a local Engine API fixture
    When itos-container runs with "doctor --json"
    Then its JSON output has a doctor problem "doctor.checkout.path" with exit 1
    And the Engine API journal is empty
    And the outside fixture is unchanged

    Examples:
      | entry                 |
      | ITOS_ROOT             |
      | an ITOS_ROOT ancestor |
      | .git                  |
      | .git/HEAD             |
      | .git/config           |
      | .git/objects          |
      | .git/refs             |

  # Sentinels are fixture programs that would create a marker if invoked; writing
  # them is setup, observing their absence alone is not the positive assertion.
  @wip @slice-3 @ID-DOCTOR-08 @local-engine-fixture
  Scenario: Inspect hostile checkout data without running host helpers or modifying state
    Given a doctor checkout
    And a local Engine API fixture
    And executable marker fixtures for Git hooks, fsmonitor, credential helpers, builds and PATH tools
    And existing host approval and session records containing synthetic fixture data
    When itos-container runs with "doctor --json"
    Then its JSON inspection identifies the checkout, selected engine and requested cached image
    And it exits with code 0
    And no executable marker was created
    And the checkout and host control state are unchanged
    And the Engine API journal contains only ping, version, info and the requested image inspection

  # Engine selection: unset/empty DOCKER_HOST means unix:///var/run/docker.sock;
  # otherwise require unix:///absolute/path with no authority, userinfo, query or
  # fragment. Resolve host-owned socket aliases for the reported endpoint. Reject
  # a socket inside the checkout, including an alias into it. Trust the host user
  # and selected engine as the pilot does; a Unix URI is not remote-host attestation.
  # No contexts, SSH transport, TCP (even loopback), TLS material or registry login.
  @wip @slice-3 @ID-DOCTOR-09 @local-engine-fixture
  Scenario Outline: Reject remote or malformed engine selection without dialing it
    Given a doctor checkout
    And a connection-recording endpoint fixture selected as "<selection>"
    When itos-container runs with "doctor --json"
    Then its JSON output has a doctor problem "<rule>" with exit <exit>
    And the endpoint fixture received no connection
    And no executable marker was created

    Examples:
      | selection                         | rule                     | exit |
      | tcp://127.0.0.1:2375              | doctor.engine.remote     | 1    |
      | ssh://engine.test                 | doctor.engine.remote     | 1    |
      | https://engine.test:2376          | doctor.engine.remote     | 1    |
      | npipe:////./pipe/docker_engine    | doctor.engine.remote     | 1    |
      | unix://relative.sock              | doctor.engine.selection  | 2    |
      | unix:///tmp/engine.sock?mode=test  | doctor.engine.selection  | 2    |
      | unix:///tmp/engine.sock#fragment   | doctor.engine.selection  | 2    |
      | checkout-owned socket             | doctor.engine.untrusted  | 1    |
      | outside alias to checkout socket  | doctor.engine.untrusted  | 1    |

  # A named context is refused even alongside DOCKER_HOST, so inspection cannot
  # silently inspect a different daemon than the host user's Docker selection.
  # DOCKER_API_VERSION is also refused: the supported SDK negotiates the API.
  @wip @slice-3 @ID-DOCTOR-10 @local-engine-fixture
  Scenario Outline: Refuse alternate engine configuration without loading host credentials
    Given a doctor checkout
    And a local Engine API fixture
    And marker fixtures at Docker context and credential-helper locations
    When itos-container runs with "doctor --json" and the environment "<setting>"
    Then its JSON output has a doctor problem "doctor.engine.selection" with exit 2
    And the Engine API journal is empty
    And no executable marker was created

    Examples:
      | setting                     |
      | DOCKER_CONTEXT=remote       |
      | DOCKER_CONTEXT=default      |
      | DOCKER_TLS_VERIFY=1         |
      | DOCKER_CERT_PATH=/unused    |
      | DOCKER_API_VERSION=1.24     |

  @wip @slice-3 @ID-DOCTOR-11 @local-engine-fixture
  Scenario Outline: Distinguish an unavailable local socket from a policy refusal
    Given a doctor checkout
    And the selected local socket is "<state>"
    When itos-container runs with "doctor --json"
    Then its JSON output has a doctor problem "doctor.engine.unavailable" with exit 3
    And its JSON output gives each problem a rule, a message and a fix
    And no engine mutation or image acquisition was attempted

    Examples:
      | state                     |
      | absent                    |
      | a regular file            |
      | a socket without listener |
      | permission denied         |

  # Supported inspection target: host Linux amd64, native Docker Engine Community
  # or Enterprise, server OS linux and architecture amd64 (x86_64 from /info is
  # normalized to amd64). Desktop, Rancher, Podman and unknown products are refused.
  # Require a nonempty /info ID and mutually compatible SDK/server API ranges.
  # SDK negotiation must not silently bypass an incompatible server minimum.
  @wip @slice-3 @ID-DOCTOR-12 @local-engine-fixture
  Scenario Outline: Refuse engine observations that cannot establish a supported identity
    Given a doctor checkout
    And a local Engine API fixture with "<observation>"
    When itos-container runs with "doctor --json"
    Then its JSON output has a doctor problem "<rule>" with exit 1
    And no cached image inspection or engine mutation was attempted

    Examples:
      | observation                              | rule                       |
      | empty daemon ID                          | doctor.engine.identity     |
      | server maximum below SDK minimum         | doctor.engine.api          |
      | server minimum above SDK maximum         | doctor.engine.api          |
      | Docker Desktop product                   | doctor.engine.unsupported  |
      | Rancher Desktop product                  | doctor.engine.unsupported  |
      | Podman product                           | doctor.engine.unsupported  |
      | unknown product                          | doctor.engine.unsupported  |
      | server OS windows                        | doctor.engine.platform     |
      | server architecture arm64                | doctor.engine.platform     |
      | version and info disagree on platform    | doctor.engine.platform     |

  # Bound the whole engine inspection to 5 seconds, including connect, version,
  # info and image reads; no automatic retries. Local timing assertions allow up
  # to 7 seconds to observe process exit. These are inspection limits only, not
  # startup/teardown limits for the separately specified owned-run lifecycle.
  @wip @slice-3 @ID-DOCTOR-13 @local-engine-fixture
  Scenario Outline: Bound incomplete engine inspection and keep operational failures typed
    Given a doctor checkout
    And a local Engine API fixture that "<failure>"
    When itos-container runs with "doctor --json"
    Then its JSON output has a doctor problem "<rule>" with exit 75
    And the command finishes within 7 seconds
    And no engine mutation or image acquisition was attempted
    And no redirect destination received a connection

    Examples:
      | failure                                | rule                     |
      | never completes ping                   | doctor.engine.timeout    |
      | never completes info                   | doctor.engine.timeout    |
      | never completes the image response     | doctor.engine.timeout    |
      | returns HTTP 500 from info             | doctor.engine.inspect    |
      | returns truncated JSON from info       | doctor.engine.inspect    |
      | returns a redirect to a trap endpoint  | doctor.engine.inspect    |

  # Endpoint alone is insufficient: reinstalling/replacing a daemon at the same
  # socket must be visible to the later approval binding. No host state is written.
  @wip @slice-3 @ID-DOCTOR-14 @local-engine-fixture
  Scenario: Report a changed daemon identity at the same local endpoint
    Given a doctor checkout
    And a local Engine API fixture
    When doctor inspects as JSON before and after the fixture daemon ID changes to "engine-B"
    Then both JSON inspections report their observed endpoint and daemon ID
    And the engine endpoints are equal and the daemon IDs differ
    And both inspections exit with code 0
    And the checkout and host control state are unchanged

  # Re-inspect daemon ID and local socket identity before reporting a successful
  # snapshot. Do not combine one daemon's info with another daemon's image result.
  # An observed replacement is a refusal; run/approval still must revalidate later.
  @wip @slice-3 @ID-DOCTOR-26 @local-engine-fixture
  Scenario Outline: Refuse engine substitution observed within one inspection
    Given a doctor checkout
    And a local Engine API fixture paused before its image response
    When doctor runs as JSON and the fixture changes <identity> before releasing the response
    Then its JSON output has a doctor problem "doctor.engine.changed" with exit 1
    And no engine mutation or image acquisition was attempted

    Examples:
      | identity             |
      | daemon ID            |
      | socket device/inode  |

  # Inspect exactly the digest-pinned reference from the effective manifest, not
  # its tag or an image with a similar name. An image ID is a config digest, not
  # the requested repository manifest digest. Require the requested RepoDigest
  # AND a nonempty sha256 image ID; retain both for later approval/run binding.
  # No registry/distribution lookup, pull, build, load, create or tag fallback.
  @wip @slice-3 @ID-DOCTOR-15 @local-engine-fixture
  Scenario: Report a missing cached image without acquiring it
    Given a doctor checkout
    And a local Engine API fixture returning image-not-found for the requested digest
    When itos-container runs with "doctor --json"
    Then its JSON output has a doctor problem "doctor.image.missing" with exit 3
    And the problem identifies the requested image reference
    And its fix asks the host operator to prepare the exact image outside doctor
    And the Engine API journal contains only ping, version, info and the requested image inspection

  @wip @slice-3 @ID-DOCTOR-16 @local-engine-fixture
  Scenario Outline: Reject a cached image that does not attest the requested repository digest
    Given a doctor checkout
    And a local Engine API fixture with image RepoDigests "<digests>"
    When itos-container runs with "doctor --json"
    Then its JSON output has a doctor problem "doctor.image.digest" with exit 1
    And the problem identifies the requested image reference
    And no engine mutation or image acquisition was attempted

    Examples:
      | digests                                              |
      | empty                                                |
      | same repository with a different digest               |
      | same digest under a different repository only         |
      | image config ID instead of the requested RepoDigest   |

  @wip @slice-3 @ID-DOCTOR-17 @local-engine-fixture
  Scenario Outline: Reject cached image platforms incompatible with the offline target
    Given a doctor checkout
    And a local Engine API fixture with image platform "<platform>"
    When itos-container runs with "doctor --json"
    Then its JSON output has a doctor problem "doctor.image.platform" with exit 1
    And the problem identifies the requested image reference
    And no engine mutation or image acquisition was attempted

    Examples:
      | platform       |
      | linux/arm64    |
      | windows/amd64  |
      | missing OS     |
      | missing arch   |

  # Engine-provided Config, including devcontainer.metadata, is passed to the
  # same restricted metadata validator owned by slice-2, without merging or
  # executing hidden settings. This item only owns inspection transport/content
  # identity failures; it does not invent a second metadata permission profile.
  @wip @slice-3 @ID-DOCTOR-18 @local-engine-fixture
  Scenario Outline: Do not report readiness from an unusable cached image response
    Given a doctor checkout
    And a local Engine API fixture with image response "<response>"
    When itos-container runs with "doctor --json"
    Then its JSON output has a doctor problem "doctor.image.inspect" with exit 75
    And no engine mutation or image acquisition was attempted

    Examples:
      | response                 |
      | malformed JSON           |
      | missing image ID         |
      | invalid image ID         |
      | missing Config           |
      | HTTP 500                 |

  # Stop after unsafe checkout/selection/engine prerequisites. Once an image is
  # available, collect independent digest and platform refusals in that order.
  # Each problem keeps its own exit; multiple inspection problems use the highest
  # numeric exit, consistent with the inherited wrapper convention. No cascaded
  # "missing image" when the engine could not be contacted.
  @wip @slice-3 @ID-DOCTOR-19 @local-engine-fixture
  Scenario: Report both independent cached image defects deterministically
    Given a doctor checkout
    And a local Engine API fixture with the wrong image digest and platform linux/arm64
    When itos-container runs with "doctor --json"
    Then its JSON output lists doctor.image.digest and doctor.image.platform in that order with exit 1 each
    And it exits with code 1
    And its JSON inspection is not ready
    And its JSON output gives each problem a rule, a message and a fix

  # Synthetic canaries are not credentials. Never copy raw engine response bodies,
  # proxy URLs, image environment, labels, config commands or helper data into
  # diagnostics. Select only the specified identity fields for success output.
  @wip @slice-3 @ID-DOCTOR-20 @local-engine-fixture
  Scenario Outline: Keep untrusted engine content out of output and persisted diagnostics
    Given a doctor checkout
    And a local Engine API fixture with synthetic canary text in "<location>"
    When itos-container runs with "doctor --json"
    Then its JSON inspection outcome is "<outcome>" with exit <exit>
    And neither output stream nor any changed file contains the synthetic canary
    And the checkout and host control state are unchanged

    Examples:
      | location                              | outcome              | exit |
      | image environment and unrelated labels | ready               | 0    |
      | engine info proxy and warning fields   | ready               | 0    |
      | HTTP 500 error body                    | doctor.engine.inspect | 75 |
      | malformed image response body          | doctor.image.inspect | 75 |

  # User-selected endpoint text can itself contain credential-looking data. A
  # refusal names the field and transport, never echoes userinfo/query/fragment.
  @wip @slice-3 @ID-DOCTOR-21 @local-engine-fixture
  Scenario: Refuse a credential-bearing remote endpoint without echoing its content
    Given a doctor checkout
    And DOCKER_HOST names a remote HTTPS endpoint with synthetic canaries in userinfo and query
    When itos-container runs with "doctor --json"
    Then its JSON output has a doctor problem "doctor.engine.remote" with exit 1
    And the problem identifies the field "DOCKER_HOST"
    And neither output stream nor any changed file contains the synthetic canary
    And no endpoint connection was attempted

  # Run this against a binary actually built for a non-Linux or non-amd64 host.
  # Do not add a production environment variable to fake the host platform. The
  # domain's platform cases also use value inputs and fakes in its unit tests.
  @wip @slice-3 @ID-DOCTOR-22 @unsupported-host
  Scenario: Refuse an unsupported host before opening an engine connection
    Given a doctor checkout on an unsupported host
    And a connection-recording endpoint fixture
    When itos-container runs with "doctor --json"
    Then its JSON output has a doctor problem "doctor.host.unsupported" with exit 1
    And no endpoint connection was attempted
    And the checkout and host control state are unchanged

  # CI alone provisions the disposable native Docker target and an operator-cached,
  # digest-pinned Linux amd64 worker fixture from reviewed test material. Record
  # host, server version/API, image reference/ID and expected/actual results in CI.
  # The test runner's read-only request recorder is separate from product output;
  # it must see attempted forbidden operations, not merely compare resource counts.
  # Setup failure is a failed required CI case, not a skip. Neither scenario runs
  # a worker or establishes the later run slice's isolation properties.
  @wip @slice-3 @ID-DOCTOR-23 @real-engine
  Scenario: Inspect an operator-cached image on native Docker without changing resources
    Given a CI-only doctor checkout and a preloaded native Docker worker fixture
    And a read-only engine request recorder and a snapshot of engine resources
    When itos-container runs with "doctor --json"
    Then its JSON inspection identities agree with the CI engine and cached image observations
    And it exits with code 0
    And the engine recorder observed only inspection requests
    And engine resources, checkout files and host control state are unchanged

  @wip @slice-3 @ID-DOCTOR-24 @real-engine
  Scenario: Missing real cached content never triggers image acquisition
    Given a CI-only doctor checkout requesting a digest absent from native Docker
    And a read-only engine request recorder and a snapshot of engine resources
    When itos-container runs with "doctor --json"
    Then its JSON output has a doctor problem "doctor.image.missing" with exit 3
    And the engine recorder observed only inspection requests
    And engine resources, checkout files and host control state are unchanged
