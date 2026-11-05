package features

import (
	"strings"
	"testing"
)

// Check the real scenario tags too: a missing or misplaced platform tag must
// neither launch a PTY on Windows nor exclude portable invocation refusals.
func TestRunPlatformSelection(t *testing.T) {
	for _, platform := range []featureEnvironment{
		{os: "linux", arch: "amd64"},
		{os: "linux", arch: "arm64"},
		{os: "windows", arch: "amd64"},
		{os: "darwin", arch: "arm64"},
	} {
		for _, tc := range []struct {
			pattern              string
			portable, pty, local int
		}{
			{"^@slice-7$", 24, 3, 11},
			{"^@ID-RUN-04$", 0, 3, 0},
			{"^@ID-RUN-03$|^@ID-RUN-04$", 9, 3, 0},
			{"^@ID-RUN-06$|^@ID-RUN-07$", 0, 0, 11},
		} {
			t.Run(platform.os+"/"+platform.arch+"/"+tc.pattern, func(t *testing.T) {
				want, excluded := tc.portable, tc.pty+tc.local
				if platform.os == "linux" {
					want += tc.pty
					excluded -= tc.pty
				}
				local := platform.os == "linux" && platform.arch == "amd64"
				if local {
					want += tc.local
					excluded -= tc.local
				}
				selected, err := selectScenarios(tc.pattern, ".", platform)
				if err != nil {
					t.Fatal(err)
				}
				if selected.count != want || len(selected.excluded) != excluded {
					t.Fatalf("selected %d, excluded %v; want %d selected and %d excluded", selected.count, selected.excluded, want, excluded)
				}
				if (selected.skip != "") != (want == 0) {
					t.Errorf("skip %q for %d eligible scenarios", selected.skip, want)
				}
				for _, reason := range selected.excluded {
					ptyExcluded := platform.os != "linux" && strings.HasPrefix(reason, "@ID-RUN-04 ") && strings.Contains(reason, "@linux requires Linux")
					localExcluded := !local && (strings.HasPrefix(reason, "@ID-RUN-06 ") || strings.HasPrefix(reason, "@ID-RUN-07 ")) && strings.Contains(reason, "other platform")
					if !ptyExcluded && !localExcluded {
						t.Errorf("unexpected exclusion: %s", reason)
					}
				}
			})
		}
	}
}
