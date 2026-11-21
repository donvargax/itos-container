//go:build !linux

package main

import "testing"

// Off Linux the init only refuses, so no measurement or probe is needed.
func measureABI() int                                                 { return 0 }
func checkStatic(*testing.T, string)                                  {}
func runWorkload(*testing.T)                                          {}
func enforcement(*testing.T, string, string, func([]string) []string) {}
