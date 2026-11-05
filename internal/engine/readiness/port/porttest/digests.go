package porttest

import (
	"strings"

	"github.com/donvargax/itos-container/internal/engine/readiness/port"
)

// Digests supplies canonical identities as fixture data, without parsing or I/O.
type Digests map[string]string

func (d Digests) Canonical(value string) string { return d[value] }

// DigestContract runs unchanged against the fake and the reference adapter.
func DigestContract(t Reporter, create func(map[string]string) port.Digests) {
	t.Helper()
	suffix := "@sha256:" + strings.Repeat("a", 64)
	values := map[string]string{
		"worker" + suffix:                                    "docker.io/library/worker" + suffix,
		"worker:release" + suffix:                            "docker.io/library/worker" + suffix,
		"docker.io/worker" + suffix:                          "docker.io/library/worker" + suffix,
		"index.docker.io/worker" + suffix:                    "docker.io/library/worker" + suffix,
		"docker.io/library/worker" + suffix:                  "docker.io/library/worker" + suffix,
		"team/worker:release" + suffix:                       "docker.io/team/worker" + suffix,
		"registry.example:5000/team/worker:release" + suffix: "registry.example:5000/team/worker" + suffix,
		"registry.example:5001/team/worker" + suffix:         "registry.example:5001/team/worker" + suffix,
		"other.test/worker" + suffix:                         "other.test/worker" + suffix,
		"worker@sha256:" + strings.Repeat("b", 64):           "docker.io/library/worker@sha256:" + strings.Repeat("b", 64),
		"": "", "worker": "", "worker:release": "", "bad name" + suffix: "",
		"sha256:" + strings.Repeat("a", 64): "", "worker@sha256:bad": "",
		"worker@sha256:" + strings.Repeat("A", 64):  "",
		"worker@sha512:" + strings.Repeat("a", 128): "",
	}
	digests := create(values)
	for input, want := range values {
		for range 2 {
			if got := digests.Canonical(input); got != want {
				t.Errorf("canonical %q = %q, want %q", input, got, want)
			}
		}
	}
}
