package features

import (
	"strings"
	"testing"
)

// Check the real scenario tags too: a missing or misplaced platform tag must
// neither launch a PTY on Windows nor exclude portable invocation refusals.
func TestRunPlatformSelection(t *testing.T) {
	for _, platform := range []string{"linux", "windows", "darwin"} {
		for _, tc := range []struct {
			pattern       string
			portable, pty int
		}{
			{"^@slice-7$", 35, 3},
			{"^@ID-RUN-04$", 0, 3},
			{"^@ID-RUN-03$|^@ID-RUN-04$", 9, 3},
		} {
			t.Run(platform+"/"+tc.pattern, func(t *testing.T) {
				want, excluded := tc.portable, tc.pty
				if platform == "linux" {
					want += tc.pty
					excluded = 0
				}
				selected, err := selectScenarios(tc.pattern, ".", featureEnvironment{os: platform})
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
					if !strings.Contains(reason, "@ID-RUN-04") || !strings.Contains(reason, "@linux requires Linux") {
						t.Errorf("unexpected exclusion: %s", reason)
					}
				}
			})
		}
	}
}
