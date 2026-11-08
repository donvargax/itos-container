package features

import (
	"fmt"
	"slices"
	"strings"

	"github.com/cucumber/godog"
)

// Keep the engine inventory stable across each scenario's before/after audit.
// The quiet worker is already running when escalation fixtures start, and stays
// running until their assertions and cleanup finish. Arbitrary concurrent suites
// would instead mistake each other's container transitions for product damage.
func runFeatureScenarios(selection scenarioSelection, options godog.Options, initialize func(*godog.ScenarioContext, func())) int {
	plain := func(sc *godog.ScenarioContext) { initialize(sc, nil) }
	const escalation = "@ID-LIFECYCLE-09"
	if !selection.engine || selection.red || selection.skip != "" ||
		!slices.Contains(selection.ids, escalation) || !slices.Contains(selection.ids, "@ID-LIFECYCLE-13") {
		return runScenarios(selection, options, plain)
	}
	nested := selection
	nested.filter = intersectScenarioFilter(selection.filter, escalation)
	nestedOptions := options
	nestedOptions.Tags = nested.filter
	suite := godog.TestSuite{Options: &nestedOptions}
	features, err := suite.RetrieveFeatures()
	if err != nil {
		if options.Output != nil {
			_, _ = fmt.Fprintf(options.Output, "cannot schedule engine observations: %v\n", err)
		}
		return 1
	}
	nested.count = 0
	for _, feature := range features {
		nested.count += len(feature.Pickles)
	}
	if nested.count == 0 || nested.count >= selection.count {
		return 1
	}
	remaining := selection
	remaining.filter = intersectScenarioFilter(selection.filter, "~"+escalation)
	remaining.count -= nested.count
	called, nestedCode := false, 0
	duringQuiet := func() {
		if !called {
			called = true
			nestedCode = runScenarios(nested, options, plain)
		}
	}
	code := runScenarios(remaining, options, func(sc *godog.ScenarioContext) { initialize(sc, duringQuiet) })
	// A failed quiet startup must not drop the deferred scenarios. Run them
	// serially after the primary suite in that case, retaining both failures.
	duringQuiet()
	if nestedCode != 0 {
		return nestedCode
	}
	return code
}

func intersectScenarioFilter(filter, tag string) string {
	branches := strings.Split(filter, ",")
	for i := range branches {
		branches[i] += "&&" + tag
	}
	return strings.Join(branches, ",")
}
