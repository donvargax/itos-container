package features

import (
	"slices"
	"testing"
)

const platformFixture = `Feature: platform selection
  Background:
    Given a memory background

  @ID-MEMORY-20
  Scenario: portable
    Then record "portable"

  @linux @ID-MEMORY-21
  Scenario Outline: Linux terminal
    Then record "<row>"
    Examples:
      | row |
      | terminal-a |
      | terminal-b |

  @linux @wip @ID-MEMORY-22
  Scenario: future terminal
    Then record "wip"

  @linux @engine @ID-MEMORY-23
  Scenario: engine terminal
    Then record "engine"
`

func TestPlatformSelection(t *testing.T) {
	dir := writeFeatureFixture(t, platformFixture)
	for _, platform := range []string{"linux", "windows", "darwin"} {
		for _, tc := range []struct {
			pattern  string
			portable bool
		}{
			{"", true},
			{"^@ID-MEMORY-2[01]$|^@wip$|^@engine$", true},
			{"^@ID-MEMORY-21$", false},
			{"^@linux$", false},
		} {
			t.Run(platform+"/"+tc.pattern, func(t *testing.T) {
				var want []string
				if tc.portable {
					want = append(want, "portable")
				}
				if platform == "linux" {
					want = append(want, "terminal-a", "terminal-b")
				}
				selected, err := selectScenarios(tc.pattern, dir, featureEnvironment{os: platform})
				if err != nil {
					t.Fatal(err)
				}
				seen := executeMemory(t, dir, selected, nil)
				if !slices.Equal(seen.steps, want) || selected.count != len(want) {
					t.Errorf("executed %v, selected %d; want %v", seen.steps, selected.count, want)
				}
				if (selected.skip != "") != (len(want) == 0) {
					t.Errorf("skip %q for %d eligible scenarios", selected.skip, len(want))
				}
				if seen.initialized != len(want) || seen.before != len(want) || seen.after != len(want) || seen.background != len(want) {
					t.Errorf("excluded scenario ran hooks or background: %+v", seen)
				}
			})
		}
	}
}
