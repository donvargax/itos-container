//go:build !linux

package features

import "github.com/cucumber/godog"

// The selector excludes the Linux PTY scenarios before initializing their steps.
func (*world) runTerminalSteps(*godog.ScenarioContext) {}
